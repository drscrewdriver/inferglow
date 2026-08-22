# Python 自建 Seatbelt 适配器方案 — Spec

> 编写时间：2026-08-15
> 状态：规划（仅文档，不实施代码）
> 总纲：[Seatbelt Loader 迁移规划](seatbelt-loader-spec.md) · 关联：[TS 适配器方案](seatbelt-ts-adapter-spec.md)

---

## 1. 可行性判定

Python 自建 seatbelt 适配器（在解释器内实现 loader 模式）**技术上可行**，依据：

- **FFI**：`ctypes.CDLL('/usr/lib/libsandbox.1.dylib')` 可直接调用 `sandbox_init`
- **进程替换**：`os.execv` 存在且语义正确（替换当前进程映像，沙箱跨 exec 保留）
- **运行时窗口可控**：关键技巧是——**解释器完整初始化（含可能的 pyc 写入、site 导入）发生在 `sandbox_init` 之前；沙箱应用后仅执行 `os.execv` 一件事**，不再有任何 Python 代码运行（execv 成功即进程消失）

可行性边界（来自总纲矩阵）：Python 的窗口风险是"沙箱后仍有 Python 代码执行"的场景；通过"初始化前置 + 沙箱后直 exec"的结构可以消除。启动开销（解释器 ~30-50ms/命令）对一次性命令可接受。

## 2. 方案对比

### 方案 A：ctypes 直调 loader（推荐，无原生二进制依赖）

```
python3 -B -S seatbelt_loader.py <profile-file> <cmd> <args...>
```

- `-B`：禁止写入 `__pycache__`（消除沙箱后/前 pyc 惰性写入）
- `-S`：不导入 site（最小化解释器初始化面）
- 流程：
  1. 读取 profile 文件（纯文本，无 import 依赖）
  2. `lib = ctypes.CDLL('/usr/lib/libsandbox.1.dylib')`
  3. `lib.sandbox_init(profile_cstr, 0, ctypes.byref(err_buf))` → 非 0 则打印错误退出 125
  4. `os.execv(cmd, [cmd, *args])`（argv[0] 保持目标命令名）
- 异常路径：execv 失败时**不得触发回溯写文件**——`sys.stderr.write` + `sys.exit(125)`，并确保 `sys.excepthook` 不落地（默认 stderr 输出，不写文件，天然安全；但 `faulthandler` 等会写文件的功能需显式关闭）

### 方案 B：包装原生 loader 二进制（可靠性最高）

```
subprocess.run([loader, profile_file, *cmd])
```

- loader 为 Go/Rust 编译的独立二进制（见总纲语言可行性矩阵：Rust 无 runtime 窗口最优，Go cgo C-main 次之）
- Python 侧零 FFI、零窗口风险，`subprocess.run` 标准用法即可
- **建议默认采用**；方案 A 作为无原生 loader 环境（纯 Python 部署）的备选

### 方案 C：排除

- PyObjC 桥：`sandbox_init` 是 C API 而非 Objective-C API，PyObjC 无帮助
- 第三方 FFI 库（如 cffi）：ctypes 已足够，无需额外依赖

## 3. 适配器 API 设计

```python
# seatbelt_adapter.py
def run(profile: str | Path, argv: list[str], *,
        cwd: str | None = None, env: dict[str, str] | None = None,
        timeout: float | None = None) -> SeatbeltResult
```

- `profile`：SBPL 文本或文件路径（走文件规避 ARG_MAX，与 inferglow `writeSBPLProfile` 语义对齐）
- 返回 `SeatbeltResult(exit_code, stdout, stderr, duration)`——对齐 inferglow `ExecutionResult`（exit/stdout/stderr/duration）
- 内部实现：方案 A 为 `os.execv` 直接替换（调用方即 loader 进程，无返回）；方案 B 为 `subprocess.run` 包装
- 超时：方案 B 由 `subprocess.run(timeout=...)` 提供；方案 A 由调用方 `signal.alarm` 或外层包装

## 4. 陷阱清单（方案 A 必读）

| 陷阱 | 缓解 |
|------|------|
| pyc 惰性写入被沙箱拦截 | `python3 -B`（或 `sys.dont_write_bytecode = True` 首行设置） |
| site 初始化导入第三方包 | `-S`；loader 脚本零 import（仅 ctypes/os/sys） |
| 异常回溯写文件（faulthandler 等） | 显式禁用；错误路径用 `sys.stderr.write` + `sys.exit(125)` |
| `tempfile` 惰性创建 | loader 不使用 tempfile |
| 解释器启动开销 | 一次性命令可接受（~30-50ms）；批量场景用方案 B 复用原生 loader |
| 依赖 python3 环境 | 方案 A 的前提；无解释器环境时强制方案 B |
| `sandbox_init` 参数编码 | profile 为 UTF-8 C 字符串，`ctypes.c_char_p(profile.encode('utf-8'))`，**不要用 UTF-16**（macOS API 非 Windows 语义） |

## 5. 测试与验收

- 真实 macOS 双向断言：
  - deny-write profile：`seatbelt_loader.py <deny> /bin/sh -c 'echo x > /tmp/pwned'` → 退出非零且文件未创建
  - workspace-write profile：`echo x > <sandboxDir>/f` → 成功
- loader 不可用降级：`sandbox_init` 返回非 0 或 dylib 加载失败 → 退出 125 + stderr 明确信息
- 无 python3 环境：方案 A 探测失败，适配器 fallback 到方案 B 或显式报错（fail-closed，不无约束直通）
- 非 macOS 平台：跳过（`platform.system() != 'Darwin'`）

## 6. 决策建议

| 场景 | 选择 |
|------|------|
| 有原生 loader 可分发的部署 | **方案 B**（默认） |
| 纯 Python 部署 / 无编译链 | 方案 A（ctypes loader，技巧已列明） |
| 高吞吐批量命令 | 方案 B（解释器启动开销不放大） |

## 7. 参考

- 总纲：[Seatbelt Loader 迁移规划](seatbelt-loader-spec.md)（语言可行性矩阵）
- inferglow `sandbox/seatbelt_policy.go`（SBPL 生成，Python 侧可移植同一 profile 语义）
- Chromium `sandbox/mac`（libsandbox 私有 API 生产使用参考）
