# TypeScript 自建 Seatbelt 适配器方案 — Spec

> 编写时间：2026-08-15
> 状态：规划（仅文档，不实施代码）
> 总纲：[Seatbelt Loader 迁移规划](seatbelt-loader-spec.md) · 关联：[Python 适配器方案](seatbelt-python-adapter-spec.md)

---

## 1. 可行性判定

Node.js **纯 JS 无法自做 loader**，依据（总纲矩阵）：

- **无内建 FFI**：Node 标准库不提供 dylib 调用（需第三方 koffi / node-ffi-napi）
- **无 execve**：Node 无进程映像替换原语；`child_process` 是 fork+spawn，无法在目标进程 exec 前注入 sandbox_init
- **运行时窗口大**：V8 编译缓存、模块加载、事件循环在沙箱后 exec 前有大量隐式行为

但 **N-API 原生 addon 完全可行**，且是 dsh 生态既有模式——`@deepseek-ai/node-addon-landlock-run` 已经用 N-API C 扩展直调内核 LSM（Landlock），seatbelt 适配器可完全对齐该包结构。

## 2. 方案对比

### 方案 A：N-API addon（推荐，对齐 landlock addon 先例）

包结构建议（对齐 `node-addon-landlock-run`）：

```
node-addon-seatbelt-run/
├── src/index.ts          # TS 入口：resolve 路径 + 导出 run()
├── src/native.c          # N-API C 扩展
├── binding.gyp           # node-gyp 构建（或 napi-rs）
└── tests/                # node:test + 真实 macOS e2e
```

C 侧核心：

```c
// 同步函数：sandbox_init + execvp 直接替换 Node 进程映像
napi_value Run(napi_env env, napi_callback_info info) {
  // 1. 解析 profilePath + argv（从 JS 参数）
  // 2. sandbox_init(profile, 0, &err) → 失败: napi_throw_error + 退出 125
  // 3. execvp(argv[0], argv)  // 成功则 Node 进程消失，沙箱跨 exec 保留
}
```

- **execvp 前 Node runtime 已启动**——与 landlock addon 相同的接受度（landlock 的 launcher 也是 Node 进程内应用 LSM 后 exec）；风险点：Node runtime 在 sandbox 应用后、execvp 前若有异步写会失败，但同步路径 + 立即 execvp 窗口极小
- 替代实现：napi-rs（Rust 写 addon）——获得 Rust 无 runtime 窗口优势，但引入 Rust 工具链；Node 生态默认 node-gyp

### 方案 B：koffi FFI（可行但引入第三方依赖）

```ts
import { lib } from 'koffi'
const libsandbox = lib('/usr/lib/libsandbox.1.dylib')
libsandbox.func('int sandbox_init(const char *profile, uint64_t flags, char **errorbuf)')
// + libc execv 调用替换进程
```

- 可行；但 koffi 是第三方 FFI 运行时，与 dsh"原生能力走 addon"的既有决策（landlock addon）不一致
- execv 需额外 libc 绑定，错误处理/生命周期管理在 TS 侧更脆弱

### 方案 C：包装原生 loader 二进制（最简，无编译链）

```
child_process.spawn(loaderPath, [profileFile, ...cmd])
```

- loader 为 Go/Rust 编译的独立二进制（总纲：Rust 最优）；TS 侧零 FFI、零窗口风险
- 缺点：分发需附带二进制；但 dsh 的 windows-acl runner 已经是"node 启动外部 runner 脚本"模式，架构上无违和

## 3. dsh 集成点（seatbelt rung 替换）

现状（`packages/sandbox/sandbox-local/src/index.ts`）：

- L340 `runnerArgv`：`case 'seatbelt': return [this.seatbeltExec(), ...seatbeltProfileArgs(policy)]`
- L546-549 `seatbeltExec()`：返回 `'sandbox-exec'`
- L85-91 `defaultProbeSeatbelt`：功能探测（真实 profile + `-- true`）

替换方案（以方案 C 为例，改动最小）：

```ts
case 'seatbelt': return [this.seatbeltLoader(), seatbeltProfileFile(policy), ...]
```

- `seatbeltLoader()`：解析内置 loader 路径（打包资源或 PATH）
- `defaultProbeSeatbelt`：改为 loader `--self-test` 探测（profile 应用后退出 0）
- 探测失败 → `unusable` → darwin 链无候选 → fail-closed `SANDBOX_UNAVAILABLE`（语义不变）
- `seatbeltProfileArgs`（SBPL 生成，profiles.ts L51-58）零改动；profile 走文件（写临时文件）规避 ARG_MAX

## 4. 陷阱清单

| 陷阱 | 缓解 |
|------|------|
| addon 内 execvp 前 Node 异步写 | 同步 API + 立即 execvp；文档化接受度（与 landlock 相同） |
| argv 传递编码 | addon 侧 UTF-8 转 C 字符串；argv[0] 保持目标命令名 |
| profile 超长 | 走临时文件传递 |
| 退出码约定 | loader/addon 失败统一 125（对齐 landlock launcher）；denial 由沙箱内核产生（operation not permitted） |
| addon 构建链 | node-gyp 需要 macOS clang（自带）；CI 需 macOS runner |
| 与现有 landlock addon 的包风格一致性 | 复制 node-addon-landlock-run 的目录/构建/测试骨架 |

## 5. 测试与验收

- `node:test` 单元：argv 组装、profile 文件写入、探测判定
- 真实 macOS e2e（对齐 landlock.e2e.ts 模式）：
  - deny-write：`run(denyProfile, ['/bin/sh', '-c', 'echo x > /tmp/pwned'])` → 非零且文件未创建
  - workspace-write：`echo x > <sandboxDir>/f` → 成功
- dsh 现有 `local.spec.ts` 的 seatbelt 用例迁移到新 runner
- 不可用降级：loader 缺失 → `SandboxUnavailableError`

## 6. 决策建议

| 场景 | 选择 |
|------|------|
| dsh 生态（已有 landlock addon 先例） | **方案 A**（N-API addon）或 **方案 C**（原生 loader 二进制）——按"是否愿意为每个平台编译 addon"权衡 |
| 最少分发面、跨语言复用 | **方案 C**（Go/Rust loader，inferglow/Python/TS 共用同一二进制） |
| 避免第三方 FFI | 排除方案 B |

## 7. 参考

- 总纲：[Seatbelt Loader 迁移规划](seatbelt-loader-spec.md)（语言可行性矩阵）
- `@deepseek-ai/node-addon-landlock-run`（N-API 直调内核 LSM 的既有模式）
- dsh `packages/sandbox/sandbox-local/src/index.ts`（seatbelt rung：L85-91/L340/L546-549）、`src/profiles.ts`（L51-58）
