# 04 · action 与 MCP 模块

## 一、action 模块

### 1.1 职责

`action` 模块（`github.com/inferglow/action`）提供 Action Runtime：将 Go 函数、MCP 工具、沙箱命令统一注册为可发现、可校验、可执行的 Action。核心是 `ActionRegistry`（并发安全目录）+ `ActionExecutor` 接口（三种实现）。

该模块本身**完全独立**（不依赖其他 inferglow 模块），仅 `executor_sandbox.go` 一个文件 import `sandbox`。

### 1.2 核心类型（[action.go](../../action/action.go)）

```go
// Action 是一个命名的、Schema 描述的、绑定 Executor 的工作单元
type Action struct {
    Name        string
    Description string
    Schema      map[string]any      // JSON Schema 格式的参数描述
    Executor    ActionExecutor
    Tags        []string
}

// ActionExecutor 是每个 Action 必须实现的运行时契约
type ActionExecutor interface {
    Execute(ctx context.Context, input map[string]any) (*ActionResult, error)
}

// ActionResult 是 Action 执行的结构化结果
type ActionResult struct {
    OK       bool
    Status   string             // "success" | "error" | "blocked"
    Result   any
    Error    string
    Metadata map[string]any     // executor 特定附加信息
}
```

### 1.3 ActionRegistry（[action.go](../../action/action.go) L80-L166 + [registry.go](../../action/registry.go)）

```go
type ActionRegistry struct {
    mu      sync.RWMutex
    actions map[string]*Action
}

func NewRegistry() *ActionRegistry

// 核心 CRUD
func (r *ActionRegistry) Register(a *Action) error           // 拒绝空名/nil Executor/重复
func (r *ActionRegistry) Get(name string) (*Action, error)
func (r *ActionRegistry) Execute(ctx, name, input) (*ActionResult, error)  // 查找+派发
func (r *ActionRegistry) Unregister(name string) bool
func (r *ActionRegistry) Has(name string) bool
func (r *ActionRegistry) List() []string                     // 排序返回所有名称

// Tag 管理
func (r *ActionRegistry) Tag(names []string, tags []string)
func (r *ActionRegistry) ListActionNames(tags []string) []string   // 按 tag 过滤
func (r *ActionRegistry) GetAction(name string) *Action
func (r *ActionRegistry) GetTags(name string) map[string]bool
```

> **`Execute` 的错误约定**：查找失败返回 `error`；Executor 返回 `error` 时被转换为 `ActionResult{OK:false, Status:"error"}`，**调用者总能拿到结构化结果**。

### 1.4 ActionSpec（[spec.go](../../action/spec.go)）

`ActionSpec` 是完整的 Action 规格，用于安全策略与沙箱配置：

```go
type SideEffectLevel string
const (
    SideEffectNone    SideEffectLevel = "none"
    SideEffectRead    SideEffectLevel = "read"
    SideEffectWrite   SideEffectLevel = "write"
    SideEffectNetwork SideEffectLevel = "network"
    SideEffectExec    SideEffectLevel = "exec"
)

type ActionPolicy struct {
    Timeout        time.Duration
    TimeoutSeconds float64
    Retries        int
    MaxRetries     int
    RetryDelay     time.Duration
    MaxOutputBytes int
    NetworkAccess  string         // "inherit" | "enabled" | "disabled"
    ReadOnly       bool
    PathAllowlist  []string
    PathDenylist   []string
}

type ActionSpec struct {
    ActionID           string
    Name               string
    Description        string
    DefaultPolicy      *ActionPolicy
    SideEffectLevel    SideEffectLevel
    ApprovalRequired   bool         // 是否需要审批
    SandboxRequired    bool         // 是否需要沙箱
    ReplaySafe         bool         // 是否可重放
    ExposeToModel      bool         // 是否暴露给 LLM
    ExecutorType       string
    ExecutionResources map[string]any
    Meta               map[string]any
    // ... 共 14+ 字段
}
```

### 1.5 三种 ActionExecutor 实现

#### A. LocalFunctionExecutor（[local_executor.go](../../action/local_executor.go)）

将普通 Go 函数包装为 ActionExecutor，支持三种签名：

| 签名编号 | 函数形式 |
|:--------:|---------|
| 1 | `func(ctx context.Context, in InputT) (OutputT, error)` |
| 2 | `func(in InputT) (OutputT, error)` |
| 3 | `func(ctx context.Context, in InputT) OutputT` |

```go
// 快速构造：自动推导 JSON Schema
func New(name, description string, fn any) (*Action, error)
```

执行时通过 JSON marshal/unmarshal 将 `map[string]any` 转换为 `InputT`，panic 会被 recover 为 error 型 `ActionResult`。

#### B. SandboxExecutor（[executor_sandbox.go](../../action/executor_sandbox.go)）

桥接 `sandbox.Manager` 到 `ActionExecutor` 契约：

```go
type SandboxExecutorConfig struct {
    Manager         *sandbox.Manager
    ApprovalService *sandbox.ApprovalService
    DefaultMode     sandbox.SandboxMode
}

func NewSandboxExecutor(cfg SandboxExecutorConfig) *SandboxExecutor
```

#### C. MCPExecutor（[executor_mcp.go](../../action/executor_mcp.go)）

代理到 MCP 服务器的 `tools/call` 方法，见第三节。

### 1.6 关键调用链

#### 链 A：注册并执行一个 Local Action

```
// 1. 注册
action, err := action.New("celsius_to_fahrenheit", "温度转换", func(in TempInput) (TempOutput, error) {
    return TempOutput{Fahrenheit: in.Celsius*9/5 + 32}, nil
})
registry.Register(action)

// 2. 执行
result, err := registry.Execute(ctx, "celsius_to_fahrenheit", map[string]any{"celsius": 37})
```

内部流程：
```
registry.Execute(ctx, name, input)
    │
    ├── Get(name) → *Action
    │
    └── a.Executor.Execute(ctx, input)
          │  (LocalFunctionExecutor)
          │
          ├── json.Marshal(input) → []byte
          ├── json.Unmarshal(bytes, &InputT)
          ├── reflect.Call(fn, [ctx, inputT])
          ├── panic recover → error-shaped ActionResult
          └── 返回 *ActionResult
```

#### 链 B：SandboxExecutor 执行流程

```
SandboxExecutor.Execute(ctx, input)
    │
    ├──[1] 解析 input: argv, env, workdir, stdin
    │
    ├──[2] 选择 sandbox 模式:
    │       input["sandbox_required"]==true → ModeDocker
    │       否则 → cfg.DefaultMode (默认 TrustedLocal)
    │       Docker 不可用时降级到 ModeAuto
    │
    ├──[3] buildPolicyFromInput(input)
    │       → sandbox.ExecutionPolicy{Timeout, NetworkAccess, ...}
    │
    ├──[4] 审批门控 (若 ApprovalService 配置 + approval_required):
    │       ApprovalService.Submit(req)
    │       ├── ApprovalPending  → ActionResult{Status:"blocked"}
    │       ├── ApprovalRejected → ActionResult{Status:"blocked"}
    │       └── ApprovalApproved → 继续
    │
    ├──[5] Manager.CreateHandle(mode, nil, policy) → Handle
    │
    ├──[6] handle.Start(ctx)
    │
    ├──[7] handle.Execute(ctx, cmd) → ExecutionResult
    │       (ExitCode, Stdout, Stderr, Duration)
    │
    ├──[8] handle.Stop(ctx)
    │
    └──[9] 映射结果:
            ExitCode==0 → ActionResult{OK:true, Result:{exit_code,stdout,...}}
            ExitCode!=0 → ActionResult{OK:false, Error:"exit code N: stderr"}
```

---

## 二、MCP 协议层（action/mcp/）

### 2.1 职责

`action/mcp/` 子包（`github.com/inferglow/action/mcp`）实现了**最小化的 MCP (Model Context Protocol) 客户端**，基于 JSON-RPC 2.0 over stdio。**不依赖任何第三方 MCP SDK**，仅用 Go 标准库实现。

### 2.2 协议常量（[client.go](../../action/mcp/client.go) L33-L39）

```go
const (
    jsonRPCVersion  = "2.0"
    protocolVersion = "2024-11-05"
    clientName      = "inferglow"
    clientVersion   = "0.1.0"
    errCodeInternal = -32603
)
```

### 2.3 核心类型

#### Transport 接口（[transport.go](../../action/mcp/transport.go)）

```go
type Transport interface {
    Start(ctx context.Context) error           // 建立连接/启动子进程
    Send(ctx context.Context, msg []byte) error // 写一帧 JSON-RPC + \n
    Recv(ctx context.Context) ([]byte, error)   // 读一帧
    Stop(ctx context.Context) error             // 关闭
}
```

两个实现：
- `StdioTransport`（[transport_stdio.go](../../action/mcp/transport_stdio.go)）：子进程 + stdin/stdout 管道
- `HTTPTransport`（[transport_http.go](../../action/mcp/transport_http.go)）：HTTP POST + SSE 流式

#### Client（[client.go](../../action/mcp/client.go)）

```go
type Client struct {
    transport Transport
    reqID     int64              // atomic 递增
    mu        sync.Mutex
    pending   map[int64]chan *jsonRPCResponse  // id → 响应通道
    done      chan struct{}
    closed    atomic.Bool
}

func NewClient(t Transport) *Client     // 启动后台 readLoop goroutine

// MCP 协议方法
func (c *Client) Initialize(ctx context.Context) (*ServerInfo, error)
func (c *Client) ListTools(ctx context.Context) ([]Tool, error)
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) ([]Content, error)
func (c *Client) Close(ctx context.Context) error
```

#### 数据类型（[types.go](../../action/mcp/types.go)）

```go
type Tool struct {
    Name        string
    Description string
    InputSchema map[string]any      // JSON Schema 片段
}

type Content struct {
    Type     string   // "text" | "image" | "resource_link"
    Text     string   // text 内容
    Data     string   // image base64
    MimeType string
    URI      string   // resource_link URI
    Name     string   // resource_link 名称
}

type ServerInfo struct {
    Name         string
    Version      string
    Capabilities map[string]any
}

type MCPServerConfig struct {
    Transport string     // "stdio"
    Command   string
    Args      []string
    Env       []string
}
```

### 2.4 关键调用链

#### 链 A：MCP 完整交互流程

```
// 1. 建立连接
transport := mcp.NewStdioTransport(MCPServerConfig{
    Command: "npx",
    Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
})
client := mcp.NewClient(transport)

// 2. Initialize 握手
serverInfo, err := client.Initialize(ctx)
    │
    ├── sendRequest(ctx, "initialize", {protocolVersion, capabilities, clientInfo})
    │     │
    │     ├── 分配 id = atomic.AddInt64(&reqID, 1)
    │     ├── 注册 pending[id] = channel
    │     ├── transport.Send(ctx, JSON{jsonrpc,id,method,params})
    │     └── 阻塞等待 <-ch 或 ctx.Done()
    │
    ├── readLoop goroutine:
    │     transport.Recv(ctx) → []byte
    │     json.Unmarshal → jsonRPCResponse{ID, Result, Error}
    │     按 ID 路由到 pending[ID] 通道
    │
    └── sendNotification(ctx, "notifications/initialized", {})

// 3. 列出工具
tools, err := client.ListTools(ctx)
    └── sendRequest(ctx, "tools/list", {}) → []Tool

// 4. 调用工具
contents, err := client.CallTool(ctx, "read_file", {"path":"/tmp/test.txt"})
    └── sendRequest(ctx, "tools/call", {name, arguments}) → []Content

// 5. 关闭
client.Close(ctx)
    └── transport.Stop(ctx) (终止子进程)
```

#### 链 B：MCP 工具自动发现并注册为 Action

```
// action.DiscoverMCPTools 一键发现+注册
registered, err := action.DiscoverMCPTools(ctx, client, registry)
    │
    ├── mcp.DiscoverAll(ctx, client)
    │     ├── client.Initialize(ctx)        // 握手
    │     └── client.ListTools(ctx)         // 列出工具
    │
    └── for each tool:
          action.NewMCPAction(client, tool)
            │
            ├── copySchema(tool.InputSchema)  // 深拷贝 JSON Schema
            └── &Action{
                    Name:        tool.Name,
                    Description: tool.Description,
                    Schema:      schema,
                    Executor:    &MCPExecutor{caller: client, toolName: tool.Name},
                }
          │
          └── registry.Register(action)
```

#### 链 C：MCPExecutor 执行

```
MCPExecutor.Execute(ctx, input)
    │
    ├── caller.CallTool(ctx, e.toolName, input)
    │     │  (mcp.Client)
    │     │
    │     ├── sendRequest(ctx, "tools/call", {name:toolName, arguments:input})
    │     └── 返回 []Content
    │
    └── 遍历 Content 数组映射到 ActionResult:
          ├── "text"          → Result = text (首个优先，后续拼接)
          ├── "image"         → Result = base64 (text 为空时)
          └── "resource_link" → Metadata["resource_links"] = [{uri,name}]
```

### 2.5 并发模型

`Client` 通过单个后台 `readLoop` goroutine 多路分解响应：

```
goroutine A: sendRequest(id=1) ──┐
goroutine B: sendRequest(id=2) ──┼──▶ transport (单写入)
goroutine C: sendRequest(id=3) ──┘
                                  │
                                  ▼
                          ┌───────────────┐
                          │  readLoop     │  (单读取 goroutine)
                          │  Recv → parse │
                          │  route by id  │
                          └───┬───┬───┬───┘
                              │   │   │
                              ▼   ▼   ▼
                            ch1 ch2 ch3  (buffer=1, 每个 id 一个)
                              │   │   │
                              ▼   ▼   ▼
                          goroutine A/B/C 收到响应
```

- `reqID` 用 `atomic.AddInt64` 分配，线程安全
- `pending` map 用 `sync.Mutex` 保护
- 响应通道 buffer=1，超时后调用方放弃时不会阻塞 readLoop
- Transport 死亡时 `failAll` 向所有 pending 通道投递错误

### 2.6 其他文件

| 文件 | 内容 |
|------|------|
| [discovery.go](../../action/mcp/discovery.go) | `DiscoverAll` 工具发现 |
| [config.go](../../action/mcp/config.go) | MCP Server 配置解析 |
| [persistence.go](../../action/persistence.go) | Action 持久化 |
