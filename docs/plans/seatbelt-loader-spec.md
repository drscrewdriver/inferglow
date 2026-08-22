# Seatbelt Loader 迁移规划 — Spec

> 编写时间：2026-08-15
> 状态：规划（仅文档，不实施代码）
> 关联文档：[Python 适配器方案](seatbelt-python-adapter-spec.md) · [TS 适配器方案](seatbelt-ts-adapter-spec.md) · [macOS Containers 储备](macos-containers-spec.md)

---

## 1. 背景

Apple 自 macOS 10.10 起将 `sandbox-exec` CLI 标记为 **deprecated**，未来系统版本可能移除。但底层 `libsandbox.1.dylib` 的私有 API（`sandbox_init` / `sandbox_compile_string` / `sandbox_apply`）仍是 macOS 安全架构核心：

- **App Sandbox**（应用沙箱强制层）
- **sandboxd**（沙箱守护进程）
- **launchd** 的 `(sandbox ...)` 配置

Chromium / Firefox 在生产环境直接调用这套 API（Chromium `sandbox/mac` 模块）。因此结论是：**被弃用的是 CLI 入口，内核机制不会消失**，迁移方向是绕过 CLI 直接调用私有 API。

## 2. Loader 模式原理

`sandbox-exec` 本身是一个极小的 C 程序，其完整逻辑可复刻：

```
sandbox_init(profile)   ← 沙箱化当前进程（不可逆，仅一次）
execvp(cmd, argv)       ← 替换进程映像；沙箱是进程属性，跨 exec 保留
```

核心不变量：**沙箱必须作用于"最终运行目标命令的进程"，且必须先于目标启动应用**。

## 3. 语言可行性矩阵（总纲结论）

Loader 模式对语言有三个硬性要求：FFI 能力、进程替换能力、沙箱后 exec 前的运行时窗口（运行时不得有文件写入等被沙箱拦截的行为）。

| 语言 | FFI | 进程替换 | 沙箱后 exec 前运行时窗口 | 判定 |
|------|-----|---------|------------------------|------|
| **Go** | ✓ `syscall.LazyDLL`；cgo C-main 可让 runtime 完全不启动 | ✓ `syscall.Exec` | 极小（cgo C-main 归零） | **合适（本项目采用）** |
| **Rust** | ✓ `libc` / `libloading` | ✓ `libc::execv` | 无（无 GC runtime） | **最合适（多语言生态 loader 首选）** |
| **Python** | △ `ctypes.CDLL` 可调 | △ `os.execv` 存在 | 大：pyc 惰性写入、site 初始化、异常回溯写文件 | **不适合自做，需适配器技巧**（见 [Python 适配器方案](seatbelt-python-adapter-spec.md)） |
| **TypeScript/Node** | ✗ 无内建 FFI（需 koffi/node-ffi） | ✗ 无 execve（需 FFI 调 libc） | 大：V8 编译缓存、模块系统、事件循环 | **不适合纯 JS 自做，需 N-API addon 或原生二进制**（见 [TS 适配器方案](seatbelt-ts-adapter-spec.md)） |

推论：Python/TS 生态的正确姿势是**调用原生 loader 二进制**或**语言侧适配层**，而非在解释器内自做完整 loader。

## 4. inferglow 实施方案（Go）

### 4.1 现状（改造点）

- `sandbox/seatbelt.go` L42：`exec.LookPath("sandbox-exec")` 探测
- `sandbox/seatbelt.go` L71 / L106：`buildSBPLProfile` + `writeSBPLProfile`（profile 已走文件）
- `sandbox/seatbelt.go` L133-176：`exec.CommandContext("sandbox-exec", ...)` 执行
- `sandbox/seatbelt_policy.go`：SBPL 生成（零改动复用）

### 4.2 方案选型

| 方案 | 说明 | 取舍 |
|------|------|------|
| **cgo C-main loader（推荐）** | `//go:build darwin && cgo` 的独立 main 包，仅运行 C main：`sandbox_init` + `execvp`。Go runtime 完全不启动，零窗口、零 GC 干扰 | 构建需 cgo 工具链（macOS 自带 clang，无额外依赖） |
| 纯 Go LazyProc loader（备选） | `syscall.NewLazyDLL("/usr/lib/libsandbox.1.dylib")` + `procSandboxInit.Call` + `syscall.Exec`，风格对齐 `windows_syscall.go` | 无 cgo；Go runtime 已启动，沙箱后 exec 前有极小窗口（Go runtime 正常不写文件系统，风险可接受） |

### 4.3 实施步骤

1. **新建 `sandbox/seatbelt_loader/` 独立 main 包**
   - C 侧：`sandbox_init(profile, 0, &err)` → 失败打印 `seatbelt-loader: <err>` 退出 **125**（对齐 dsh landlock launcher 失败码约定）；成功 `execvp(argv[1], &argv[1])`
   - 参数约定：`seatbelt-loader <profile-file> <cmd> <args...>`（profile 走文件，规避 ARG_MAX）
2. **`seatbelt.go` 改造**
   - `NewSeatbeltProvider`：探测从 `exec.LookPath("sandbox-exec")` 改为 loader 路径解析 + `--self-test` 功能探测（空 profile 应用后退出 0）
   - `Execute`：`exec.CommandContext(loaderPath, profilePath, args...)`；`writeSBPLProfile` / stdin / stdout / stderr / Dir / Env 传递逻辑不变
   - 保持 fail-closed：loader 缺失/不可用 → `ErrProviderUnavailable`，绝不无约束直通
3. **构建接入**：Makefile 或 `go generate` 增加 `seatbelt-loader` 构建目标（`go build -o bin/seatbelt-loader ./sandbox/seatbelt_loader`）；文档记录 macOS 分发需附带该二进制
4. **测试**（真实 macOS 运行，非 darwin 跳过）：
   - 沙箱生效：deny-write profile 下 `echo x > /tmp/pwned` 退出非零且文件未创建
   - 沙箱内可写：workspace-write profile 下 `echo x > <sandboxDir>/f` 成功
   - 不可用降级：loader 缺失 → `ErrProviderUnavailable`
   - 现有 mock 测试零回归（非 darwin stub 跳过）
5. **文档同步**：README 后端表更新 Seatbelt 说明（"内置 loader，不再依赖 sandbox-exec CLI"）

### 4.4 依赖关系

```
步骤 1（loader 包）独立先行
  └─► 步骤 2（seatbelt.go 改造）依赖步骤 1 的 loader 产物
        └─► 步骤 4（测试）依赖 1-2
步骤 3（构建接入）可与 2 并行
步骤 5（文档）依赖全部
```

## 5. 风险与缓解

| 风险 | 缓解 |
|------|------|
| 私有 API 未来变化 | Apple 自身依赖 seatbelt（App Sandbox 等），消失概率低；`--self-test` 探测 fail-closed 兜底 |
| cgo 构建依赖 | macOS 自带 clang；仅 loader 包用 cgo，主模块保持纯 Go |
| Go runtime 窗口（若走 LazyProc 路径） | 首选 cgo C-main 方案归零；LazyProc 为备选并文档化 |
| profile 超长（ARG_MAX） | 走文件传递（`writeSBPLProfile` 已有） |
| exec 失败（目标不存在） | loader 打印明确错误退出 125 |

## 6. 验收标准

- macOS 上 `go build` 产出 `seatbelt-loader`；`--self-test` 通过
- 沙箱双向断言通过（deny 写失败 / workspace 写成功）
- loader 缺失时 Provider fail-closed（`ErrProviderUnavailable`）
- 非 darwin 平台零回归（stub 跳过）
- 文档与实现一致

## 7. 参考

- `sandbox/seatbelt.go`（L42/L71/L106/L133-176）、`sandbox/seatbelt_policy.go`
- dsh `node-addon-landlock-run`（N-API 直调内核 LSM 的先例，见 [TS 适配器方案](seatbelt-ts-adapter-spec.md)）
- Chromium `sandbox/mac`（libsandbox 私有 API 生产使用）
