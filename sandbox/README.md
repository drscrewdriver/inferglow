# sandbox - 沙箱执行框架

**模块路径**: `github.com/inferglow/sandbox`

## 概述

sandbox 模块提供隔离的代码执行环境，支持多种沙箱后端（Docker、gVisor、本地），通过 Provider 插件机制可扩展任意自定义沙箱类型。

## 设计定位

- **被谁依赖**: 上层业务逻辑（agently 主模块的 Agent 类通过 PolicyApproval 门控）
- **依赖谁**: 无第三方库 — **完全独立**
- **对标 Python**: `agently/types/data/policy_approval.go` + `utils/PythonSandbox.py`
- **独立可用性**: ✅ 完全独立，有独立 CLI 示例

## 核心类型

### Provider — 沙箱提供者接口

```go
type Provider interface {
    Name() string
    Kind() string
    InspectAvailability() (*AvailabilityResult, error)
    CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error)
}
```

### Handle — 沙箱句柄

```go
type Handle interface {
    Start(ctx context.Context) error
    Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error)
    Stop(ctx context.Context) error
    Status() HandleStatus
}
```

### Command — 执行命令

```go
type Command struct {
    Argv    []string // 命令及参数
    Env     []string // 环境变量（"KEY=VALUE" 列表）
    Workdir string   // 工作目录
    Stdin   io.Reader // 标准输入
}
```

### ExecutionResult — 执行结果

```go
type ExecutionResult struct {
    ExitCode   int
    Stdout     string
    Stderr     string
    Duration   time.Duration
}
```

## 沙箱模式

```go
type SandboxMode string

const (
    ModeTrustedLocal SandboxMode = "trusted_local"  // 可信本地（进程级）
    ModeLocal        SandboxMode = "local"           // 本地（进程级）
    ModeDocker       SandboxMode = "docker"          // 容器级
    ModeGVisor       SandboxMode = "gvisor"          // 用户态内核级
    ModeAuto         SandboxMode = "auto"            // 自动选择（降级链）
)
```

**ModeAuto 降级链**: gvisor → docker → local（Windows 上 local 内部为 AppContainer → RestrictedToken；Windows Sandbox 因企业版/弹窗/通信限制被排除）

## 执行策略

### ExecutionPolicy — 完整安全策略

```go
type ExecutionPolicy struct {
    SandboxMode      SandboxMode
    ResourceLimit    ResourceLimit    // CPU/内存/磁盘/进程数
    NetworkAccess    NetworkPolicy    // 网络访问控制
    FilesystemAccess FilesystemPolicy // 文件系统限制
    AllowedCommands  []string         // 命令白名单
    Timeout          time.Duration    // 超时
    IsolationLevel   IsolationLevel   // 隔离级别
}
```

### ResourceLimit — 资源限制

```go
type ResourceLimit struct {
    CPUShares   int64 // CPU 份额
    MemoryBytes int64 // 内存限制（字节）
    DiskBytes   int64 // 磁盘限制（字节）
    NPROC       int   // 最大进程数
}
```

### NetworkPolicy — 网络策略

```go
type NetworkPolicy struct {
    AllowInternet bool
    AllowedPorts  []int
    AllowedHosts  []string
    Level         NetworkAccessLevel // none | egress_only | full
}
```

### FilesystemPolicy — 文件系统策略

```go
type FilesystemPolicy struct {
    ReadOnlyRoot bool
    Mounts       []MountEntry
    AllowedPaths []string
    DeniedPaths  []string
}
```

## 内置后端实现

| 文件 | 后端 | 说明 |
|------|------|------|
| docker.go | DockerProvider | Docker 容器沙箱 |
| gvisor.go | GVisorProvider | gVisor 用户态内核沙箱 |
| local_sandbox.go | LocalSandboxProvider | 本地进程级沙箱（自动选择平台最优后端） |
| trusted_local.go | TrustedLocalProvider | 可信本地执行（无隔离） |
| seatbelt.go | SeatbeltProvider | macOS Seatbelt 沙箱 |
| windows_runtime.go | WindowsRuntimeProvider | Windows 三后端：RestrictedToken（进程级）/ AppContainer（应用级，默认强隔离）/ WindowsSandbox（VM 级，需企业版且无法无头运行，默认不选） |

## Manager — 管理器

```go
// Provider 注册中心 + 模式选择
mgr, _ := NewProviderBuilder().Build()

// 列出所有注册提供者
for _, name := range mgr.List() { ... }

// 检查提供者可用性
p, _ := mgr.Get("docker")
avail, _ := p.InspectAvailability()

// 自动选择沙箱
selected, _ := mgr.SelectSandbox(ModeAuto)

// 创建沙箱句柄
handle, _ := mgr.CreateHandle(ModeTrustedLocal, nil, &policy)
handle.Start(ctx)
result, _ := handle.Execute(ctx, &Command{Argv: []string{"echo", "hello"}})
handle.Stop(ctx)
```

## OS 检测

```go
osName := DetectOS()        // "linux" | "darwin" | "windows"
providers := AvailableProvidersOnOS(osName)  // 列出支持的提供者
```

## 核心接口一览

```
Provider              → 沙箱提供者接口
Handle                → 沙箱句柄
Command               → 执行命令
ExecutionResult       → 执行结果
AvailabilityResult    → 可用性检查结果
Manager               → 管理器（Provider 注册表）
ProviderBuilder       → 自动注册内置提供者
SandboxMode           → 沙箱模式枚举
ExecutionPolicy       → 执行策略
ResourceLimit         → 资源限制
NetworkPolicy         → 网络策略
FilesystemPolicy      → 文件系统策略
IsolationLevel        → 隔离级别
```

## 与上层的关系

```
agently 主模块 (Agent 类)
  ├── Action.Spec.SandboxRequired = true
  ├── PolicyApprovalManager.Check() → 触发沙箱创建
  ├── ExecutionResourceProvider.Ensure() → 创建沙箱资源
  ├── sandbox.Mgr.CreateHandle(mode) → 获取 Handle
  ├── Handle.Execute(ctx, cmd) → 执行命令
  └── ExecutionResult → 返回输出/错误
```

## CLI 示例

项目包含独立可运行的 CLI 示例：

```bash
cd sandbox/cmd/sandbox
go run main.go
```

执行步骤：
1. 检测当前 OS
2. 列出支持的提供者
3. 创建 Manager 并注册 4 个内置提供者
4. 检查每个提供者的可用性
5. SelectSandbox(ModeAuto) 选择最佳沙箱
6. 创建 TrustedLocal 句柄并执行 `echo hello`
