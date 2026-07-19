# Agently 三个分支修复的小问题 → Inferglow 等效性交叉检查

## 来源

- Agently 仓库：`Agently/`（remote: github.com/drscrewdriver/Agently.git）
- Inferglow 仓库：`inferglow/`（remote: gitlab.drscrewdriver.cn:joshua/inferglow.git）
- 检查日期：2026-07-29
- 分析师：Reasonix

## 检查方法

```
Agently Git 历史分析分支：
  feature/bubblewrap-provider  ← "bwarp"
  feature/landlock-provider    ← "landlock"
  adapt/seatbelt-provider      ← "seatbelt"

对每个分支的 fix commit 提取：
  1) 问题的技术本质
  2) Agently 的修复方式
  3) Inferglow 对应代码是否有等效问题
```

---

## 一、Bubblewrap（bwarp）— `feature/bubblewrap-provider`

### 分支 commit 全景

```
b53af797 feat: Bubblewrap (Linux) ExecutionResourceProvider    ← 初始实现
0b07eaf2 docs: 添加 Bubblewrap provider 双语说明文档
a8a382f2 test: 添加 Bubblewrap 隔离验证测试
c6381d9f fix: 修复 bwrap 测试检测函数和命令路径              ← **唯一 fix commit**
2840dcb9 docs: 测试文件说明改为双语（中/英）
007ed581 docs: 补充 Bubblewrap 限制能力详细说明
```

### 修复的问题（c6381d9f）

| # | 问题 | 细节 | Agently 修复 |
|---|------|------|-------------|
| 1 | `check_bwrap_works()` 找不到 `echo` | 最小容器/CI 环境没有 /usr bind → bwrap 内 `echo` 命令不存在 | 检测函数添加 `/usr` 只读 bind |
| 2 | `run_bwrap()` 也缺基础路径 | 测试 helper 同样缺少 /usr、/bin 符号链接 | 添加 `--ro-bind /usr /usr` + `--symlink usr/lib64 /lib64` + `--symlink usr/bin /bin` |
| 3 | tmpfs 测试重复绑定 | 默认 `run_bwrap` 已含 `--tmpfs /tmp`，测试用例又额外传入 | 移除测试用例中多余的 `extra_args=["--tmpfs", "/tmp"]` |

### Inferglow 等效性检查

**Inferglow 文件：** `sandbox/bubblewrap.go`（541 行）

| 问题 | Inferglow 状态 | 详情 |
|------|---------------|------|
| (1) 默认缺基础路径 | 🟢 **测试已配置，但实现无默认** | `TestBubblewrapIntegrationEcho` 和 `TestBubblewrapIntegrationReadOnly` 显式传入了 `bind_ro: ["/usr:/usr", "/bin:/bin", "/lib:/lib", "/lib64:/lib64"]`。但 `buildArgv()`（bubblewrap.go:361-416）只对 `/proc` 和 `/dev` 提供默认挂载，**不对 /usr/bin 等做默认注入**。这不是 bug，用户需自行配置 bind_ro。 |
| (2) 测试重复绑定 | 🟢 **无此问题** | inferglow 测试无此模式 |
| (3) symlink 挂载 | 🟢 **Go 测试未涉及** | inferglow 测试使用直白路径，无 symlink 场景 |

**结论：** Bubblewrap 部分问题均属**测试层**，inferglow 测试已正确配置。`buildArgv()` 无默认 /usr 注入是其设计选择，不是 bug。

---

## 二、Landlock — `feature/landlock-provider`

### 分支 commit 全景

```
193682a7 feat: Landlock (Linux) ExecutionResourceProvider       ← 初始实现（placeholder 版本，隔离从未实际生效）
fd6ff23c docs: 添加 Landlock provider 双语说明文档
815ea463 test: 添加 Landlock 隔离验证测试
08d2149c fix: Landlock 实际隔离生效 - 添加 PR_SET_NO_NEW_PRIVS  ← **P0 修复：让 Landlock 真正工作**
6a9c1c0c fix: Landlock Provider 6项缺陷修复                  ← **P0~P2 综合修复**
32f82e9f fix: 补齐 spec 要求 - /proc/self/exe + 测试名对齐    ← **Spec 对齐**
b8bdcf07 feat: Landlock probe capabilities 增强
```

### 修复的问题列表

#### 🔴 P0-1：`PR_SET_NO_NEW_PRIVS` 缺失（08d2149c）

- **本质：** `landlock_restrict_self()` 前必须调用 `prctl(PR_SET_NO_NEW_PRIVS, 1)`，否则内核返回 EPERM。
- **Agently 初始版本：** `_apply_landlock` 是 placeholder，只写了 `# TODO` 注释，从未真正调用 Landlock。
- **修复：** 在 `preexec_fn` 中先 prctl 再 syscall，退出码 125/126/127 表示 Landlock 设置失败。

**Inferglow 等效：🔴 等效问题存在**

```go
// sandbox/landlock.go:176-187
func landlockRestrictSelf(rulesetFD int) error {
    _, _, e := unix.Syscall(
        unix.SYS_LANDLOCK_RESTRICT_SELF,
        uintptr(rulesetFD),
        0,
        0,
    )
    if e != 0 {
        return e
    }
    return nil
}
```

**在 `landlockRestrictSelf()` 之前没有调用 `unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)`。** 在标准 Linux 内核（5.13+）上，Landlock 要求 `no_new_privs` 属性已设置，否则 `restrict_self` 返回 EPERM。这意味着 Inferglow 的 Landlock **隔离功能实际上可能从未生效**。

**影响范围：** `sandbox/landlock.go` 中 `LandlockHandle.Execute()` 的代码路径（line 473-561）。首次 Execute 调用 `landlockRestrictSelf(rulesetFD)`（line 506），但该调用将在没有 `no_new_privs` 的情况下静默失败，`Execute()` 返回错误但隔离未生效。

---

#### 🔴 P0-2：`_apply_landlock` 静默失败（6a9c1c0c）

- **本质：** ctypes 失败时只 `return`，父进程以为沙箱已生效，实际上未受限。
- **Agently 修复：** 用 `os._exit(125/126/127)` 显式退出，父进程检测 exit code 并在 stderr 输出诊断信息。

**Inferglow 等效：🟢 无此问题**

```go
// sandbox/landlock.go:506-508
if err := landlockRestrictSelf(rulesetFD); err != nil {
    return nil, fmt.Errorf("landlock: restrict_self: %w", err)
}
```

Inferglow 的 `landlockRestrictSelf()` 返回 `error`，`Execute()` 检查并传播错误。但是——**由于 P0-1 的 PR_SET_NO_NEW_PRIVS 缺失，这里的错误实际上是 EPERM，用户看到的是 "landlock: restrict_self: permission denied" 而不是隔离未生效，但问题性质不同。**

---

#### 🔴 P0-3：默认无法执行命令（缺少 `_BASE_READ_DIRS`）（6a9c1c0c）

- **本质：** Landlock 应用后，默认不读 `/usr`、`/lib`、`/lib64`、`/dev/null`、`/dev/urandom` 等路径 → `python3`、`ls` 等基本命令执行失败。
- **Agently 修复：** 自动注入 `_BASE_READ_DIRS`（含 /usr, /lib, /lib64, /dev/null, /dev/urandom, /etc/ld.so.cache 等），自动注入 cwd 为只读路径。

**Inferglow 等效：🔴 等效问题存在**

```go
// sandbox/landlock.go:195-207
type LandlockConfig struct {
    AllowedReadDirs  []string
    AllowedWriteDirs []string
    AllowedReadFiles []string
    AllowedWriteFiles []string
    HandledAccessFS LandlockAccessFS
    ABIVersion     int
}
```

**没有 `_BASE_READ_DIRS` 等价物。** Inferglow 的 `LandlockConfig` 完全依赖用户配置 `read_dirs`/`write_dirs`。如果用户不传入 `/usr`、`/lib`、`/lib64`、`/dev/null`、`/dev/urandom` 等路径，Landlock 应用后基本命令将无法执行。

---

#### ⚠️ P1：路径逃逸校验（6a9c1c0c）

- **本质：** `__init__` 中未对路径做 `Path.resolve()`，`../` 和 symlink 可绕过白名单。
- **Agently 修复：** `self.allowed_read_dirs = [str(Path(p).resolve()) for p in ...]`

**Inferglow 等效：🔴 等效问题存在**

```go
// sandbox/landlock.go:146-151
func landlockAddPathRule(rulesetFD int, path string, allowed LandlockAccessFS) error {
    fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
    if err != nil {
        return fmt.Errorf("landlock: open %q: %w", path, err)
    }
    // ...
}
```

**没有调用 `filepath.EvalSymlinks(path)` 来解析 symlink。** 用户配置 `/tmp/real_dir` 而 `/tmp/real_dir` 到 `/etc` 有 symlink → Landlock 实际限制 `/tmp/real_dir` 而非 `/etc`，攻击者可通过 symlink 绕过文件系统限制。

---

#### ⚠️ P1：`/tmp` 无可选写入 + 自动 temp dir 清理（6a9c1c0c）

- **本质：** 用户需要 `/tmp` 写入能力时需手动创建目录，且无自动清理。
- **Agently 修复：** `async_ensure` 自动 `tempfile.mkdtemp()` 创建可写目录，`async_close` 时自动清理。

**Inferglow 等效：🔴 等效问题存在**

Inferglow 的 `LandlockHandle` 没有自动 temp dir 创建和清理机制。用户如果需要在沙箱内写入临时文件，需自行配置 `write_dirs` 并管理生命周期。

---

#### 💡 P2：`async_health_check` 缺少 ABI 版本检测（6a9c1c0c）

**Inferglow 等效：🟢 已正确实现**

```go
// sandbox/landlock.go:276-297
func (p *LandlockProvider) probe() {
    ver := landlockCreateRulesetProbe()  // ABI 探测
    // ... 验证可用性
    fd, err := landlockCreateRuleset(supported)
    // ... 实际创建 ruleset 验证
}
```

Inferglow 的 `probe()` 同时做了 ABI 版本探测 + 实际创建 ruleset 验证。

---

#### 💡 Spec：`/proc/self/exe` 缺失 + `/proc` 不做 resolve（32f82e9f）

- **Agently 修复：** `_BASE_READ_DIRS` 添加 `/proc/self/exe`；`_add()` 对 `/proc/` 路径跳过 `Path.resolve()`。

**Inferglow 等效：🟡 间接影响**

由于 Inferglow 没有 `_BASE_READ_DIRS` 概念，`/proc/self/exe` 不在任何自动注入列表里。但 `landlockAddPathRule()` 对所有路径一视同仁调用 `unix.Open()`，`/proc/self/exe` 在 Linux 上 open(O_PATH) 通常能工作。

---

## 三、Seatbelt — `adapt/seatbelt-provider`

### 分支 commit 全景

```
c8ca4a69 feat: Seatbelt (macOS) ExecutionResourceProvider   ← 初始实现
87c9bab5 refactor: 条件导入 Seatbelt provider，仅 macOS 加载
dfa91b7b refactor: SBPL profile 重构，借鉴 OpenHanako 设计
ce9518df docs: 添加 Seatbelt 双语说明文档 (cn/en)
fca0c8f2 fix: 修复 Seatbelt provider 兼容性问题           ← **接口/架构修复**
aa0a0814 fix: 修复 Seatbelt provider 3 个致命 BUG          ← **BUG 修复**
6bed059b ignore
12cf4ba8 chore: 恢复 .gitignore 到 main 状态，添加 seatbelt TDD 测试
```

### 修复的问题列表

#### 🔴 BUG-1：`sandbox-exec -f -` 不支持 stdin（aa0a0814）

- **本质：** macOS 的 `sandbox-exec -f -` 不支持从 stdin 读取 profile → profile 静默被忽略，沙箱未生效。
- **Agently 修复：** 改用临时文件 `tempfile.NamedTemporaryFile` + `-f tmp.name`。

**Inferglow 等效：🟢 已正确实现**

```go
// sandbox/seatbelt_policy.go:234-251
func writeSBPLProfile(profile string) (string, func(), error) {
    tmpFile, err := os.CreateTemp("", "seatbelt-*.sbpl")
    // ...
}
```

```go
// sandbox/seatbelt.go:105-110
func (h *SeatbeltHandle) Start(ctx context.Context) error {
    policyPath, cleanup, err := writeSBPLProfile(h.profile)
    h.policyFile = policyPath
    // ...
}
```

```go
// sandbox/seatbelt.go:134
args := []string{"-f", policyFile}
```

从初始实现就使用了临时文件方式，未经过 stdin 传参的阶段。

---

#### 🔴 BUG-2：SBPL 语法 `(allow mach(*))` 无效（aa0a0814）

- **本质：** Seatbelt Profile Language 中 `mach(*)` 是非法语法，应为 `mach*`。
- **Agently 修复：** 改为 `(allow mach*)`。

**Inferglow 等效：🟢 已用正确语法**

```go
// sandbox/seatbelt_policy.go:79
sb.WriteString("(allow mach-lookup)\n")
```

使用 `mach-lookup` 而非 `mach*`，语法正确。

---

#### 🔴 BUG-3：SBPL 语法 `(deny network)` 无效（aa0a0814）

- **本质：** SBPL 中 `(deny network)` 无法实际拒绝网络；正确语法是 `(deny network-outbound)`。
- **Agently 修复：** 改为 `(deny network-outbound)`。

**Inferglow 等效：🟢 已用正确语法**

```go
// sandbox/seatbelt_policy.go:168
sb.WriteString("(deny network-outbound)\n")
```

从初始实现就使用了正确的 `network-outbound` 语法。

---

#### ⚠️ 兼容性修复：移除不存在的基类继承（fca0c8f2）

- **本质：** 旧版 Seatbelt provider 继承了 `ExecutionResourceProvider` 基类（4.1.4.2 中已被移除），导致运行时导入错误。
- **Agently 修复：** 移除基类继承改用 duck-typing，`TYPE_CHECKING` 导入替代运行时导入。接口方法从 `create_handle` 改为 `async_ensure`/`async_health_check`/`async_release`。

**Inferglow 等效：🟢 不适用（Go 语言无关）**

Inferglow 使用 Go interface 静态类型检查：

```go
// sandbox/provider.go
var _ Provider = (*SeatbeltProvider)(nil)
var _ Handle = (*SeatbeltHandle)(nil)
```

编译期即可发现接口不匹配，不存在 Python duck-typing 的兼容性问题。Inferglow 使用 `CreateHandle`/`Start`/`Execute`/`Stop` 生命周期模型。

---

## 四、总结：等效问题清单

### P0（致命 — 安全/功能完全失效）

| # | 问题 | 位置 | 风险 | 修复成本 |
|---|------|------|------|---------|
| **L-P0-1** | **Landlock: 缺少 `PR_SET_NO_NEW_PRIVS`** | `sandbox/landlock.go:176`，`landlockRestrictSelf()` 前 | Landlock 隔离**从未实际生效**（内核返回 EPERM） | 低：添加一行 `unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)` |
| **L-P0-2** | **Landlock: 无基础路径自动注入** | `sandbox/landlock.go:195-207`，`LandlockConfig` | 默认配置下基本命令无法执行 | 中：定义 `_BASE_READ_DIRS` 列表，`Start()` 中自动注入 |

### P1（高危 — 安全削弱）

| # | 问题 | 位置 | 风险 | 修复成本 |
|---|------|------|------|---------|
| **L-P1-1** | **Landlock: 路径逃逸** | `sandbox/landlock.go:148`，`landlockAddPathRule()` | symlink/`../` 可绕过白名单 | 低：`filepath.EvalSymlinks()` 后再 open |
| **L-P1-2** | **Landlock: 无自动 temp dir** | `sandbox/landlock.go`，`LandlockHandle` | 用户需手动管理临时目录生命周期 | 中：`Start()` 自动创建 temp dir，`Stop()` 清理 |

### P2（注意点）

| # | 问题 | 位置 | 说明 |
|---|------|------|------|
| **B-Note** | **Bubblewrap: 无默认 /usr/bin 挂载** | `sandbox/bubblewrap.go:361-416` | 设计选择，不是 bug，但文档应提醒用户配置 bind_ro |

### 各 Provider 等效问题汇总

| Provider | P0 (致命) | P1 (高危) | P2 (注意) | 总评 |
|----------|-----------|-----------|-----------|------|
| **Bubblewrap** | 0 | 0 | 0（1 个注意点） | 🟢 **良好** — 从初始实现就避免了 Agently 的测试层问题 |
| **Landlock** | **2 🔴** | **2 🟡** | 0 | 🟠 **需修复** — PR_SET_NO_NEW_PRIVS 是功能完全失效级 bug |
| **Seatbelt** | 0 | 0 | 0 | 🟢 **良好** — 从初始实现就避免了 Agently 的 3 个 BUG |

---

## 五、推荐修复优先级

```
优先级 1 (立即修复):
  L-P0-1: Landlock PR_SET_NO_NEW_PRIVS 缺失
          文件: sandbox/landlock.go
          修复: landlockRestrictSelf() 调用前添加 unix.Prctl()

优先级 2 (尽快修复):
  L-P1-1: Landlock 路径逃逸 (symlink resolve)
          文件: sandbox/landlock.go
          修复: landlockAddPathRule() 入口添加 filepath.EvalSymlinks()

优先级 3 (改进):
  L-P0-2: Landlock 基础路径自动注入
  L-P1-2: Landlock 自动 temp dir

优先级 4 (文档):
  B-Note: Bubblewrap 注意 /usr 默认挂载配置
```
