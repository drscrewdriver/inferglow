# 05 · session、sandbox 与 audit 模块

## 一、session 模块

### 1.1 职责

`session` 模块（`github.com/inferglow/session`）是**会话级对话记忆管理器**。维护双消息列表（FullContext 永不裁剪 + ContextWindow 可裁剪），支持多模态内容、多种 resize 策略、JSON/YAML 持久化。完全独立，无 inferglow 内部依赖。

### 1.2 核心类型（[session.go](../../session/session.go)）

#### 消息与多模态

```go
// 多模态内容块
type ContentBlock struct {
    Type string         // "text" | "image" | "video" | "audio" | "file"
    Data any            // string (text) / []byte (binary) / URL
    Meta map[string]any
}

// 单条消息
type ChatMessage struct {
    Role      string         // "user" | "assistant" | "system"
    Content   any            // string | []ContentBlock
    Name      string
    Meta      map[string]any
    Timestamp time.Time
}
```

#### Session 主体

```go
type Session struct {
    mu sync.RWMutex

    ID            string
    FullContext   []ChatMessage          // 完整历史，永不裁剪
    ContextWindow []ChatMessage          // 当前窗口，可能被 resize 裁剪
    Memo          map[string]any         // 会话级长期记忆（跨轮次共享）
    MaxLength     int                    // 窗口字节上限
    AutoResize    bool                   // 自动触发裁剪开关

    // resize 策略
    ResizeHandler       ResizeHandler            // 旧路径：单一 handler
    resizeHandlers      map[string]ResizeHandler // 新路径：多策略注册表
    analysisHandlers    []AnalysisHandler        // 分析器：决定触发哪个策略
    defaultResizeName   string                   // 默认策略名

    // 安全钩子
    securityHook MessageHook       // 消息拦截（注入检测）
    masker       MessageMasker     // 内容脱敏（PII）
}
```

#### Resize 接口

```go
// ResizeHandler 在窗口超限时裁剪
type ResizeHandler func(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error)

// AnalysisHandler 决定是否触发 resize 及用哪个策略
type AnalysisHandler func(full []ChatMessage, window []ChatMessage, memo map[string]any) (string, error)
```

### 1.3 四种内置 Resize 策略（[resize.go](../../session/resize.go)）

| 策略 | 函数 | 行为 |
|------|------|------|
| **SimpleCut** | `SimpleCutResizeHandler` | 从前面丢弃，保留最近消息 |
| **SummaryFirst** | `SummaryFirstResizeHandler` | 保留首条 + 末尾 2 条 + 中间摘要 |
| **TokenAware** | `TokenAwareResizeHandler` | 按 token 估算裁剪（`len/4 = 1 token`） |
| **SmartCompress** | `SmartCompressResizeHandler` | 保留首条 + 最近 N 条，中间 tool 结果压缩为标记 |

#### SmartCompress 策略详细说明

`SmartCompressResizeHandler(maxKeepRecent int)` 是 v2 新增的高级压缩策略，专为长对话场景设计：

- **第一条消息**（通常 system prompt）始终保留
- **最近 N 条**消息完整保留
- **中间区域**：`role="tool"` 的消息被压缩为 `[previously executed tool]` 或 `[previously read: /path/to/file]`（从 Meta 提取文件路径）
- **assistant/user 文本消息**（推理链）始终保留

这种策略在大幅减少工具结果占用的同时，保留 assistant 的完整推理链，对 prefix cache 命中率非常友好。

### 1.3.1 多策略注册表（[resize.go](../../session/resize.go)）

v2 起 Session 支持多 resize 策略注册：

```go
// 旧路径（单一 handler，仍兼容）
sess.ResizeHandler = TokenAwareResizeHandler

// 新路径（多策略）
sess.SetResizeStrategies(
    SnipFromHead(4),
    PruneLowValue(64),
    SmartCompressResizeHandler(8),
)
```

多策略路径通过 `AnalysisHandler` 决定使用哪个策略：分析器遍历 `full/window` 计算是否需要 resize，返回策略名；`resizeHandlers` 按名查找并执行。

### 1.3.2 SessionBackend 统一抽象（v2）

v2 新增 `SessionBackend` 接口，统一 `Session` 和 `ThreeZoneSession` 的使用方式：

```go
type SessionBackend interface {
    AddMessage(role string, content any, name string)
    AddMessageWithMeta(role string, content any, name string, meta map[string]any)
    PreparePrompt() []ChatMessage
    SaveJSON(path string) error
}
```

这使得 `SessionExtension` 不再依赖具体类型，可透明切换 `Session` / `ThreeZoneSession`。

同时 `Session` 和 `ThreeZoneSession` 均实现了 `AddMessageWithMeta`：允许在消息中附带 `tool_calls`、`tool_call_id` 等元数据，这些元数据通过 `PreparePrompt()` 完整传递给 LLM。

`ThreeZoneSession.SaveJSON()` 将三区状态序列化到 JSON 文件，格式兼容 `LoadJSON`，支持 crash recovery。

### 1.4 安全钩子接口

```go
// 消息拦截钩子（在 AddMessage 前执行）
type MessageHook interface {
    BeforeAddMessage(role string, content any, name string) error
    // 返回 error 阻止消息写入（如 ErrPromptInjectionBlocked）
}

// 内容脱敏钩子（在拦截通过后执行）
type MessageMasker interface {
    MaskInput(text string) string   // 输入侧脱敏
    MaskOutput(text string) string  // 输出侧脱敏
}
```

> `security/pii.Masker` 实现 `MessageMasker` 接口；`security/prompt_injection.Detector` 可包装为 `MessageHook`。这是**接口依赖**，`session` 包不 import `security`。

### 1.5 关键调用链

#### 链 A：AddMessage 完整流程

```
session.AddMessageChecked(role, content, name)
    │
    ├──[1] securityHook.BeforeAddMessage(role, content, name)
    │       │
    │       ├── nil → 跳过（向后兼容）
    │       ├── 返回 error → 拒绝写入，返回 err
    │       │   (如 ErrPromptInjectionBlocked)
    │       └── 返回 nil → 继续
    │
    ├──[2] masker.MaskInput(content)  (仅 string 内容)
    │       │
    │       ├── nil → 不脱敏
    │       └── pii.Masker 按 ApplyOn 配置决定是否实际脱敏
    │
    ├──[3] 构造 ChatMessage{Role, Content, Name, Timestamp: now}
    │
    ├──[4] 追加到 FullContext 和 ContextWindow
    │
    └──[5] if AutoResize: applyResizeLocked()
            │
            ├── 新路径 (analysisHandlers 非空):
            │     注入 Memo["max_length"]
            │     遍历 analysisHandlers:
            │       analyzer(full, window, memo) → strategyName
            │       resizeHandlers[strategyName](full, window) → resized
            │       (失败回退到 defaultResizeName)
            │
            └── 旧路径 (ResizeHandler 非空):
                  计算总字节数
                  if totalBytes > MaxLength:
                    ResizeHandler(full, window) → resized
                  ContextWindow = resized
```

#### 链 B：PreparePrompt（供 LLM 使用）

```
session.PreparePrompt() → []ChatMessage
    │
    ├── 加读锁
    ├── 复制 ContextWindow
    └── 对每条消息:
          if Content is []ContentBlock:
            拼接所有 text block 为字符串
            (image/video/audio/file → "[type referenced]")
          else:
            保持原样
        → 返回纯文本化的消息列表（供 model.ModelRequest.ChatHistory）
```

### 1.6 其他文件

| 文件 | 内容 |
|------|------|
| [three_zone.go](../../session/three_zone.go) | 三区记忆系统（Zone/MemoryZone） |
| [persistence.go](../../session/persistence.go) | `ToJSON()`/`ToYAML()`/`LoadJSON()`/`LoadYAML()` |
| [security_hook.go](../../session/security_hook.go) | `MessageHook` 接口 + `WithSecurityHook` Option |
| [resize.go](../../session/resize.go) | 三种 resize 策略实现 |

### 1.7 可插拔架构改进（v2）：session/security 解耦

v2 起 `session` 不再直接 import `security`，仅保留 `MessageHook` 接口契约。原 `SecurityHook` 实现（依赖 `prompt_injection.Detector`）已移至 `security/sessionhook` 子包。

**变更前（v1）**：`session/security_hook.go` 同时定义接口与实现，`session` 直接依赖 `security/prompt_injection`。

**变更后（v2）**：

| 位置 | 内容 |
|------|------|
| `session/security_hook.go` | 仅保留 `MessageHook` 接口、`WithSecurityHook` Option、`NewSessionWithOptions` |
| `security/sessionhook/hook.go` | `SecurityHook` 实现 + `NewSecurityHook(cfg)` + `FlagRecord` |
| `security/sessionhook/hook.go` | 编译期断言 `var _ session.MessageHook = (*SecurityHook)(nil)` |

**依赖方向**：`security/sessionhook → session`（单向）。`session` 对 `security` 完全无感知。

**注入方式**：

```go
import (
    "github.com/inferglow/session"
    "github.com/inferglow/security/sessionhook"
    promptinjection "github.com/inferglow/security/prompt_injection"
)

hook := sessionhook.NewSecurityHook(promptinjection.NewDefaultConfig())
sess := session.NewSessionWithOptions("id", 4000, session.WithSecurityHook(hook))
```

不注入时 `securityHook` 为 nil，`AddMessageChecked` 跳过钩子调用，**零开销**。

---

## 二、sandbox 模块

### 2.1 职责

`sandbox` 模块（`github.com/inferglow/sandbox`）提供隔离的代码执行环境，支持 7 种沙箱后端，跨平台（Linux/macOS/Windows）。完全独立，无 inferglow 内部依赖。

### 2.2 核心接口（[provider.go](../../sandbox/provider.go)）

```go
// Provider 是每个沙箱后端必须实现的契约
type Provider interface {
    Name() string
    Kind() string
    InspectAvailability() (*AvailabilityResult, error)
    CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error)
}

// Handle 是长期存活的沙箱实例
type Handle interface {
    Start(ctx context.Context) error
    Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error)
    Stop(ctx context.Context) error
    Status() HandleStatus
}

// 命令与结果
type Command struct {
    Argv    []string
    Env     []string
    Workdir string
    Stdin   io.Reader
}

type ExecutionResult struct {
    ExitCode int
    Stdout   string
    Stderr   string
    Duration time.Duration
}

type HandleStatus string  // "created" | "running" | "stopped" | "error"
```

### 2.3 7 种沙箱后端

| 后端 | 文件 | 平台 | 说明 |
|------|------|------|------|
| **Docker** | [docker.go](../../sandbox/docker.go) | 跨平台 | Docker 容器隔离 |
| **gVisor** | [gvisor.go](../../sandbox/gvisor.go) | Linux | 用户态内核隔离 |
| **Local** | [local_sandbox.go](../../sandbox/local_sandbox.go) | 跨平台 | 本地受限执行 |
| **TrustedLocal** | [trusted_local.go](../../sandbox/trusted_local.go) | 跨平台 | 信任本地执行（无隔离） |
| **Seatbelt** | [seatbelt.go](../../sandbox/seatbelt.go) | macOS | macOS sandbox-exec |
| **WindowsAppContainer** | [windows_appcontainer.go](../../sandbox/windows_appcontainer.go) | Windows | Windows AppContainer |
| **E2B** | [e2b.go](../../sandbox/e2b.go) | 云端 | E2B 远程沙箱 |

### 2.4 Manager（[manager.go](../../sandbox/manager.go)）

```go
type Manager struct {
    mu          sync.RWMutex
    providers   map[string]Provider
    defaultMode SandboxMode       // 默认 ModeAuto
}

func NewManager() *Manager
func (m *Manager) Register(p Provider) error
func (m *Manager) Get(name string) (Provider, error)
func (m *Manager) List() []string
func (m *Manager) SelectSandbox(mode SandboxMode) (Provider, error)
func (m *Manager) CreateHandle(mode SandboxMode, cfg map[string]any, policy *ExecutionPolicy) (Handle, error)
```

`ModeAuto` 会按可用性链尝试后端：Docker → gVisor → TrustedLocal → Local。

### 2.5 审批服务（[approval.go](../../sandbox/approval.go)）

```go
type ApprovalService struct { ... }

// 审批模式
const (
    ApprovalAutoApprove    // 自动批准
    ApprovalFailClosed     // 拒绝
    ApprovalInputTimeoutFail // 超时拒绝
)

type ApprovalRequest struct {
    ProviderName string
    Mode         SandboxMode
    Policy       *ExecutionPolicy
    Requester    string
    Reason       string
}

type ApprovalRecord struct {
    ID     string
    Status ApprovalStatus  // Pending / Approved / Rejected
    ...
}

func (s *ApprovalService) Submit(req *ApprovalRequest) (*ApprovalRecord, error)
```

### 2.6 执行策略（[policy.go](../../sandbox/policy.go)）

```go
type ExecutionPolicy struct {
    Timeout           time.Duration
    NetworkAccess     NetworkAccess
    FilesystemAccess  FilesystemAccess
    ResourceLimit     ResourceLimit
}
```

### 2.7 关键调用链

```
SandboxExecutor.Execute(ctx, input)
    │ (见 04 文档链 B)
    │
    ├── Manager.SelectSandbox(mode)
    │     │
    │     ├── mode == ModeAuto:
    │     │     遍历 [docker, gvisor, trusted_local, local]
    │     │     返回首个 InspectAvailability().Available 的 Provider
    │     │
    │     └── 指定 mode: 直接 Get(name)
    │
    ├── Provider.CreateHandle(cfg, policy) → Handle
    │
    ├── Handle.Start(ctx)
    │     (后端特定: docker run / exec sandbox-exec / ...)
    │
    ├── Handle.Execute(ctx, cmd) → ExecutionResult
    │     (在沙箱内执行命令, 收集 stdout/stderr/exitcode)
    │
    └── Handle.Stop(ctx)
          (后端特定: docker rm / kill subprocess / ...)
```

---

## 三、audit 模块

### 3.1 职责

`audit` 模块（`github.com/inferglow/audit`）实现**链表式不可篡改审计链**。每条 `AuditEntry` 通过 SHA-256 哈希指针链接前一条，形成 append-only 日志。任何历史条目被修改都会导致后续链路哈希断裂。**默认关闭**，开启后零侵入接入 orchestrator。完全独立，无 inferglow 内部依赖。

### 3.2 核心类型（[types.go](../../audit/types.go)）

```go
// 单条审计记录
type AuditEntry struct {
    PrevHash  string            `json:"prev_hash"`   // 前一条的 Hash（链指针）
    Hash      string            `json:"hash"`        // 本条 SHA-256
    ID        string            `json:"id"`          // 唯一标识
    Timestamp time.Time         `json:"timestamp"`
    Source    string            `json:"source"`      // "agent"|"action"|"model"|"flow"
    Action    string            `json:"action"`      // "decision"|"execute"|"request"
    Input     any               `json:"input,omitempty"`
    Output    any               `json:"output,omitempty"`
    Duration  time.Duration     `json:"duration"`
    Error     string            `json:"error,omitempty"`
    Metadata  map[string]string `json:"metadata,omitempty"`
    Signature string            `json:"signature,omitempty"`  // 可选 HMAC
}

// 配置
type AuditConfig struct {
    Enabled        bool
    SignatureKey   []byte    // HMAC-SHA256 密钥，nil 不签名
    StorageBackend string    // "memory" | "json_file"
    StoragePath    string    // json_file 模式的目录
    MaxEntries     int       // 0 = 无限
}

// 轻量接口（调用方依赖此接口而非具体类型）
type AuditHook interface {
    Append(entry *AuditEntry) (string, error)
    IsEnabled() bool
}

// 查询过滤器
type QueryFilter struct {
    Source   string
    Action   string
    From     time.Time
    To       time.Time
    Metadata map[string]string
}

// 存储接口
type Storage interface {
    Save(entry *AuditEntry) error
    LoadAll() ([]*AuditEntry, error)
}
```

### 3.3 AuditChain（[chain.go](../../audit/chain.go)）

```go
type AuditChain struct {
    mu       sync.RWMutex
    cfg      AuditConfig
    entries  []*AuditEntry
    lastHash string              // 链尾哈希
    storage  Storage
    seq      int64               // 原子递增序列号
    clock    func() time.Time    // 可注入（测试用）
}

func NewAuditChain(cfg AuditConfig) (*AuditChain, error)
func (c *AuditChain) IsEnabled() bool
func (c *AuditChain) Append(entry *AuditEntry) (string, error)
func (c *AuditChain) Len() int
func (c *AuditChain) VerifyChain() error
```

> `AuditChain` 编译期实现 `AuditHook` 接口（`var _ AuditHook = (*AuditChain)(nil)`）。`NoOpHook` 是零开销默认实现。

### 3.4 关键调用链

#### 链 A：Append（追加审计条目）

```
auditChain.Append(entry)
    │
    ├──[1] entry == nil → 返回 ("", nil)
    │
    ├──[2] cfg.Enabled == false → 返回 ("", nil)  // 零开销
    │
    ├──[3] 自动填充字段:
    │       now := clock()
    │       if entry.ID == "":  ID = "audit-{now.UnixNano()}-{seq}"
    │       if entry.Timestamp.IsZero():  Timestamp = now.UTC()
    │       if entry.PrevHash == "":  PrevHash = lastHash  // 链接前一条
    │       if entry.Hash == "":  Hash = ComputeHash(entry)  // SHA-256
    │
    ├──[4] if len(SignatureKey) > 0:
    │       SignEntry(entry, key)  // HMAC-SHA256(key, entry.Hash)
    │
    ├──[5] entries = append(entries, entry)
    │       lastHash = entry.Hash
    │
    ├──[6] if MaxEntries > 0 && len > MaxEntries:
    │       丢弃最旧的条目 (软上限，不重写历史)
    │
    └──[7] storage.Save(entry)
            (MemoryStorage: 追加切片; JSONFileStorage: 写 JSONL 文件)
            返回 (entry.Hash, saveErr)
```

#### 链 B：VerifyChain（完整性校验）

```
auditChain.VerifyChain()  (verify.go)
    │
    ├── snapshot := c.snapshot()  // 防御性拷贝
    │
    └── for i, e := range entries:
          │
          ├──[1] prev_hash 连续性:
          │       if i > 0 && e.PrevHash != entries[i-1].Hash:
          │           return VerifyError{Index:i, Reason:"prev_hash mismatch"}
          │
          ├──[2] hash 重计算:
          │       recomputed := ComputeHash(e)
          │       if recomputed != e.Hash:
          │           return VerifyError{Index:i, Reason:"hash mismatch"}
          │
          └──[3] 签名验证 (if SignatureKey 非空):
                  if !VerifySignature(e, key):
                      return VerifyError{Index:i, Reason:"signature mismatch"}

        return nil  // 全部通过
```

> **篡改检测原理**：修改第 N 条的任何字段 → 第 N 条 Hash 不匹配（检查 2）；修改第 N 条的 Hash → 第 N+1 条的 PrevHash 与第 N 条新 Hash 不匹配（检查 1）。两种篡改都会被捕获。

### 3.5 其他文件

| 文件 | 内容 |
|------|------|
| [hash.go](../../audit/hash.go) | `ComputeHash(entry)` — SHA-256(PrevHash + Timestamp + Source + Action + Input + Output) |
| [sign.go](../../audit/sign.go) | `SignEntry` / `VerifySignature` — HMAC-SHA256 |
| [storage.go](../../audit/storage.go) | `MemoryStorage` / `JSONFileStorage` 实现 |
| [query.go](../../audit/query.go) | `Query(filter)` — 按 source/action/时间范围/metadata 查询 |
| [export.go](../../audit/export.go) | `Export(format)` — JSON / CSV / Text 导出 |
| [config.go](../../audit/config.go) | 配置辅助 |
| [noop.go](../../audit/noop.go) | `NoOpHook` 零开销默认实现 |

### 3.6 接入点

审计链通过 `AuditHook` 接口接入 orchestrator，有两个写入点（见 [01 文档 §4.1](./01-architecture-overview.md)）：

```
orchestrator/agent/engine.go:
    executeLoop() 每轮 LLM 决策后:
        auditHook.Append(&AuditEntry{Source:"agent", Action:"decision", ...})

orchestrator/actionruntime/dispatcher.go:
    Execute() 每个 Action 执行后:
        auditHook.Append(&AuditEntry{Source:"action", Action:"execute", ...})
```

`NoOpHook`（默认）的 `IsEnabled()` 返回 false，`Append` 直接返回，**零开销**。
