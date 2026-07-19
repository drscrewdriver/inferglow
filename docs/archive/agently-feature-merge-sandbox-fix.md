# Agently Feature 分支合并与沙箱接入修复 Spec

## 1. 合并策略：Rebase + 手工适配

**选择 rebase 而非 merge**：两个 feature 分支基于旧版接口协议，机械 merge 会引入契约违规代码。rebase 允许逐 commit 重写以对齐 4.1.4.4 接口。

**执行顺序**：先 gvisor（改动小），再 seatbelt（独立 provider）。

```bash
# Step 1: gvisor 适配
git checkout -b adapt/gvisor-docker-runtime origin/feature/gvisor-docker-runtime
git rebase --onto main 63a3026e adapt/gvisor-docker-runtime
# Step 2: 合并 gvisor
git checkout main && git merge --no-ff adapt/gvisor-docker-runtime

# Step 3: seatbelt 适配
git checkout -b adapt/seatbelt-provider origin/feature/seatbelt-provider
git rebase --onto main 63a3026e adapt/seatbelt-provider
# Step 4: 合并 seatbelt
git checkout main && git merge --no-ff adapt/seatbelt-provider
```

## 1.5 契约硬约束：所有分支必须遵守

基于 4.1.4.2 版本的架构契约，以下规则**不可违反**：

### 1.5.1 统一扩展点

**所有隔离实现必须接入 `ExecutionResourceProvider` 扩展点，统一使用 `code_execution` 资源 kind。**

```
✅ 正确：
   ExecutionResourceProvider (kind="code_execution")
   ├── DockerExecutionResourceProvider (supported_kinds=("docker", "code_execution"))
   │   └── config.runtime="runsc" → gVisor
   ├── TrustedLocalExecutionResourceProvider (supported_kinds=("code_execution",))
   ├── SeatbeltExecutionResourceProvider (kind="code_execution")
   └── BubblewrapExecutionResourceProvider (kind="code_execution")

❌ 错误：
   ExecutionResourceProvider
   ├── DockerExecutionResourceProvider (kind="docker")
   ├── GvisorExecutionResourceProvider (kind="gvisor")  ← 平行架构
   ├── SeatbeltExecutionResourceProvider (kind="seatbelt")  ← 平行架构
   └── BubblewrapExecutionResourceProvider (kind="bubblewrap")  ← 平行架构
```

### 1.5.2 禁止修改 `_normalize_code_sandbox`

**`_normalize_code_sandbox()` 的枚举值不可扩展。**

- 当前枚举：`"auto"` / `"docker"` / `"trusted_local"`
- 这是 Action 层的沙箱模式选择，决定"要不要隔离、哪种级别"
- 所有隔离实现（gVisor/Seatbelt/Bubblewrap）都应通过 `code_execution.providers` 有序候选接入
- 新增枚举会导致平行 ActionExecutor 栈，违反契约

```
✅ 正确：通过有序候选选择隔离实现
   settings["code_execution.providers"] = [
       "seatbelt",  # macOS 优先
       {"provider_id": "docker", "config": {"runtime": "runsc"}},  # gVisor fallback
       "docker",  # runc fallback
   ]

❌ 错误：新增 Action 层枚举
   _normalize_code_sandbox(value) -> Literal["auto", "docker", "trusted_local", "seatbelt", "gvisor"]
```

### 1.5.3 外部 Provider 接口完整性

**所有 `ExecutionResourceProvider` 必须实现以下接口：**

| 接口方法 | 必须实现 | 用途 |
|----------|---------|------|
| `provider_id` | ✅ | 稳定标识 |
| `supported_kinds` | ✅ | 声明支持的资源 kind |
| `async_probe` | ✅ | 报告可用性与隔离能力轴 |
| `async_ensure` | ✅ | 创建/获取资源 handle |
| `async_health_check` | ✅ | 健康检查 |
| `async_release` | ✅ | 释放资源 |
| `async_execute_code` | ✅ | 代码执行（`code_execution` kind 必须） |

**执行规范：**
- argv-only：不经 shell，直接 argv 执行
- 有界输出：`max_output_bytes` 限制
- 超时/取消终止：`asyncio.wait_for` + 进程终止
- 终态清理：`async_release` 清理所有临时资源

### 1.5.4 职责划分不可混淆

| 职责 | 承担者 | 禁止 |
|------|--------|------|
| 文件 + grant | `TaskWorkspace` | Provider 不可自建文件管理 |
| bundle + argv plan | 语言 adapter (`CodeRuntimeAdapter`) | Provider 不可自建语言适配 |
| live lifecycle | `ExecutionResource` | ActionRuntime 不可直接管理资源生命周期 |
| schema / dispatch | `ActionRuntime` | Provider 不可自建调度逻辑 |

---

## 2. 冲突解决清单

### 2.1 `ActionResourceRegistrar.py`（高冲突）

- **保留 main 全部代码**（`_normalize_code_execution_providers()`、argv-only 等）
- **不修改 `_normalize_code_sandbox()`**：保持 `"auto"` / `"docker"` / `"trusted_local"` 三值
- **原因**：`_normalize_code_sandbox` 是 Action 层的沙箱模式选择，决定"要不要隔离、哪种级别"。所有隔离实现（gVisor/Seatbelt/Bubblewrap）都应通过 `code_execution.providers` 有序候选接入，而非新增 Action 层枚举。新增枚举会导致平行架构，违反契约。
- gvisor 的 `runtime` 配置通过 provider config 透传，不进入此文件

### 2.2 `DockerExecutionResourceProvider.py`（高冲突）

- **以 main 1119 行版本为基础**
- 添加 `SUPPORTED_RUNTIMES = ("runc", "runsc")`
- `__init__` 添加 `runtime: str = "runc"` 参数
- `_container_base_args()` 注入 `--runtime runsc`
- 添加 `inspect_gvisor_availability()` 静态方法
- `create_resource()` / `async_ensure()` 透传 runtime
- `async_probe()` 动态报告 container_runtime

### 2.3 `ExecutionResourceProvider/__init__.py`（中冲突）

- 保留 main 全部 import
- 末尾追加 seatbelt 条件导入：`if platform.system() == "Darwin": from .Seatbelt...`

## 3. gVisor 适配：Docker 配置变体

**设计决策**：gVisor 作为 `DockerExecutionResourceProvider` 的**配置变体**（非独立 provider），因为 runsc 通过 Docker `--runtime` flag 调用，复用全部 Docker 生命周期。

**配置方式**：
```python
settings["code_execution.providers"] = [
    {"provider_id": "docker", "config": {"runtime": "runsc"}},
    "docker",  # fallback
]
```

**Probe 隔离能力轴**（`runtime == "runsc"` 时）：
- `process_contained: True`、`host_filesystem_restricted: True`
- `privilege_escalation_blocked: True`、`syscalls_restricted: True`
- `mechanism: "gvisor"`、`safety_class: "isolated"`

## 4. Seatbelt 适配：独立 Provider

**对齐基类**：继承 `BuiltinExecutionResourceProvider`，`kind = "code_execution"`（非 `"seatbelt"`）

**核心变更**：
- 添加 `async_probe` 方法（当前缺失）
- `provider_id` 返回 `"seatbelt"`
- `async_execute_code(bundle, manifest, grant)` 实现（当前只有 `run_python_code`）

**Probe 隔离能力轴**：
- `process_contained: True`、`host_filesystem_restricted: True`
- `privilege_escalation_blocked: True`、`syscalls_restricted: True`
- `mechanism: "seatbelt"`、`network_mode: "disabled"`

## 5. 测试验证

- `test_seatbelt_bugs.py` — SBPL profile 正确性
- `test_docker_gvisor_runtime.py`（新建）— `--runtime runsc` 注入
- `test_seatbelt_provider_interface.py`（新建）— 接口合规
- 集成测试：`register_python_sandbox_action(sandbox="seatbelt")` 生成正确 provider_candidates
- 平台测试：Linux 上 seatbelt 不导入、macOS 上正常

## 6. 回滚方案

- **分支级**：`git revert -m 1 <merge-commit>`
- **功能级**：移除 settings 中的 `"seatbelt"` / `runtime: "runsc"` 配置即可禁用，默认行为不受影响

---

## 7. 扩展评估：Bubblewrap / Landlock 新增 Provider

> 参考实现：inferglow Go sandbox 模块（`sandbox/bubblewrap.go` 553 行、`sandbox/landlock.go` 608 行）
> 当前环境：`bwrap 0.9.0` 已安装，内核 `7.0.0-28-generic`（Landlock 要求 5.13+）

### 7.1 inferglow 参考架构

inferglow 的 `LocalSandboxProvider` 持有**后端链**，按 OS 自动选择：

```
Linux:   [Bubblewrap, Landlock]  // bwrap 优先（完整命名空间隔离），landlock fallback（仅文件系统）
Darwin:  [Seatbelt]
Windows: [WindowsRuntime]
```

Agently 的 `code_execution.providers` 有序候选机制本质上是同一思路。

### 7.2 对比矩阵

| 维度 | Bubblewrap | Landlock |
|------|-----------|----------|
| **隔离机制** | 用户命名空间（`bwrap` CLI 封装） | 内核 LSM syscall（raw syscall 包装） |
| **隔离范围** | 完整：user/pid/uts/ipc/cgroup/net 命名空间 + 文件系统 + 网络 | 仅文件系统（读/写/执行/删除/创建 权限位） |
| **外部依赖** | `bwrap` 二进制（`apt install bubblewrap`） | 零外部依赖，纯内核 syscall |
| **不可逆性** | 否，每次 `bwrap` 是独立子进程 | **是**，`landlock_restrict_self` 一旦应用不可撤销 |
| **适用模型** | 可反复创建/销毁 Handle | 一次性约束，需 fork 子进程模式 |
| **inferglow 代码量** | ~550 行 Go | ~600 行 Go（含 raw syscall 包装） |

### 7.3 移植到 Agently Python 的开销

**Bubblewrap Provider**（开销：**低，~300 行 Python**）：
- 核心逻辑：`asyncio.create_subprocess_exec("bwrap", *argv)` 构造 argv 前缀
- 无需 ctypes/FFI，与 Docker provider 模式一致（"构造 argv → 启动子进程"）
- inferglow 的 `buildArgv()` 可直接翻译：`--unshare-all`、`--ro-bind`、`--bind`、`--tmpfs`、`--die-with-parent` 等
- 配置解析：`bind_ro`/`bind_rw`/`tmpfs`/`unshare_all`/`share_net`/`clearenv`/`timeout`
- 生命周期：每次 Execute 启动新 bwrap 子进程，天然支持反复创建

**Landlock Provider**（开销：**中，~400 行 Python**）：
- 需要 `ctypes` 调用 3 个 raw syscall（syscall 号 444/445/446 on x86_64/arm64）
- 或引入 `landlock` Python 包（生态不成熟，不推荐）
- **不可逆性是最大挑战**：Agently 是长生命周期服务进程，`restrict_self` 永久限制当前进程
- 解决方案：`os.fork()` + `exec` 在子进程中应用 Landlock，父进程不受影响
- ABI 版本协商（v1/v2/v3）需处理不同内核版本的访问位差异

### 7.4 可行性结论

| Provider | 可行性 | 理由 |
|----------|--------|------|
| **Bubblewrap** | **高** | 外部 CLI 封装，与 Docker provider 模式一致，无不可逆问题，`bwrap` 已是 Ubuntu 标配 |
| **Landlock** | **中低** | 不可逆性与 Agently 长生命周期模型冲突；fork 子进程模式增加复杂度；ctypes syscall 包装维护成本高 |

### 7.5 建议优先级

```
1. gVisor (runtime=runsc)     — 已有分支，保留为 Docker 配置变体
2. Seatbelt                   — 已有分支，补齐 async_probe + kind 对齐
3. Bubblewrap                 — 新增分支，参考 inferglow Go 实现，~300 行 Python
4. Landlock                   — 暂缓，待 Agently 引入 fork-then-exec 子进程模型后再评估
```

### 7.6 Bubblewrap Provider 适配 Spec（预留）

**设计**：独立 `ExecutionResourceProvider`，`kind = "code_execution"`，`provider_id = "bubblewrap"`

**配置方式**：
```python
settings["code_execution.providers"] = [
    "bubblewrap",           # Linux 本地隔离优先
    {"provider_id": "docker", "config": {"runtime": "runsc"}},  # gVisor fallback
    "docker",               # runc fallback
]
```

**Probe 隔离能力轴**：
- `process_contained: True`（user/pid/net 命名空间隔离）
- `host_filesystem_restricted: True`（bwrap 白名单 bind mount）
- `privilege_escalation_blocked: True`（user namespace 内 root 非宿主 root）
- `syscalls_restricted: False`（bwrap 不做 syscall 过滤，与 gVisor/Seatbelt 的区别）
- `mechanism: "bubblewrap"`、`safety_class: "contained"`

**关键实现点**：
- `async_ensure()` → 探测 `bwrap` 在 PATH 中，解析 requirement config
- `async_execute_code()` → 构建 bwrap argv 前缀 + 追加 user command argv
- `async_release()` → 清理临时 bind source 目录
- 条件导入：`if platform.system() == "Linux"` 才加载

---

## 8. `runtime=runsc` 定位确认

**保留为 Docker provider 的配置变体**，理由：
- `docker run --runtime=runsc` 是 Docker 官方 OCI runtime 切换方式
- 复用 Docker 全部生命周期管理
- 不需要独立 provider

**调整**：probe 中当 `runtime == "runsc"` 时，`mechanism` 报告 `"gvisor"` 而非 `"container"`，让上层调度能区分隔离能力等级。

---

## 9. 契约合规性总览

| 契约要求 | Bubblewrap | Landlock | gVisor (runsc) | Seatbelt |
|----------|-----------|----------|----------------|----------|
| 统一扩展点 `code_execution` kind | ✅ | ✅ | ✅ Docker config 变体 | ✅ 需修复 |
| 禁止平行架构 | ✅ | ✅ | ✅ 复用 Docker 生命周期 | ✅ |
| TaskWorkspace 持有文件+grant | ✅ bundle→bind mount | ✅ bundle→path_beneath | ✅ 已有 | ✅ 已有 |
| 语言 adapter 负责 bundle+argv | ✅ bwrap argv 前缀 | ✅ fork+exec argv | ✅ 已有 | ✅ 已有 |
| ExecutionResource 负责 live lifecycle | ✅ Start/Execute/Stop | ⚠️ 一次性约束 | ✅ 已有 | ✅ 已有 |
| ActionRuntime 负责 schema/dispatch | ✅ 不涉及 | ✅ 不涉及 | ✅ 不涉及 | ✅ 不涉及 |
| `async_probe` 布尔能力轴 | ✅ | ✅ | ✅ 扩展 Docker probe | ✅ 需添加 |
| argv-only 执行 | ✅ | ✅ | ✅ 已有 | ✅ 已有 |
| 有界输出+超时/取消 | ✅ `context.WithTimeout` 等价 | ✅ | ✅ 已有 | ✅ 已有 |
| 终态清理 | ✅ Stop 清理临时目录 | ⚠️ fd 关闭但限制不可撤销 | ✅ 已有 | ✅ 已有 |
