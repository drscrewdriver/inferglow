# 06 · security、observability 与 workspace 模块

## 一、security 模块

`security` 模块（`github.com/inferglow/security`）包含四个子包，提供 PII 脱敏、Prompt 注入防护、速率限制、RBAC 访问控制。完全独立，无 inferglow 内部依赖。通过接口被 `session` 和 `orchestrator` 引用。

### 1.1 PII 脱敏（security/pii/）

#### 核心类型（[mask.go](../../security/pii/mask.go)）

```go
type MaskOn int
const (
    MaskOnInput  MaskOn = 1 << iota  // 输入侧脱敏
    MaskOnOutput                      // 输出侧脱敏
)

type MaskConfig struct {
    Patterns   map[PIIType]*regexp.Regexp  // nil 时用 DefaultPatterns()
    MaskChar   string                       // 默认 "***"
    KeepPrefix int                          // 保留前缀字符数
    ApplyOn    MaskOn                       // 选择脱敏侧
}

type Masker struct { ... }

func NewMasker(cfg MaskConfig) *Masker
func (m *Masker) Mask(text string) string
func (m *Masker) MaskInput(text string) string   // 实现 session.MessageMasker
func (m *Masker) MaskOutput(text string) string  // 实现 session.MessageMasker
```

#### 内置 PII 模式（[patterns.go](../../security/pii/patterns.go)）

| PIIType | 匹配对象 |
|---------|---------|
| `Email` | 电子邮箱 |
| `Phone` | 电话号码 |
| `IDCard` | 身份证号（中国 18 位） |
| `BankAccount` | 银行卡号 |
| `CreditCard` | 信用卡号 |

`DefaultPatterns()` 返回上述 5 种模式的编译后正则。模式按「最具体优先」顺序应用，避免 BankAccount 吞噬 IDCard 匹配。

#### 接入点

```
Agent.Run() 通过 WithPIIMasker(masker) 注入:
    │
    ├── session.SetMessageMasker(masker)
    │     → AddMessageChecked 时调用 MaskInput (输入侧)
    │
    └── 最终响应调用 masker.MaskOutput (输出侧)
```

### 1.2 Prompt 注入防护（security/prompt_injection/）

#### 核心类型（[detector.go](../../security/prompt_injection/detector.go)）

```go
type Severity int
const (
    SeverityLow    Severity = iota  // 软信号
    SeverityMedium                  // 可能的注入
    SeverityHigh                    // 明确注入
)

type Match struct {
    Pattern     string    // 命中的关键词/正则
    MatchedText string    // 匹配的子串
    Position    int       // 字节偏移
    Severity    Severity
}

type DetectionResult struct {
    Detected bool
    Matches  []Match
    Severity Severity      // 所有 Match 中的最大严重级别
}

type Detector struct { ... }

func NewDetector(cfg Config) *Detector
func (d *Detector) Detect(text string) DetectionResult
func (d *Detector) DetectInput(text string) DetectionResult   // 输入侧
func (d *Detector) DetectOutput(text string) DetectionResult  // 输出侧
```

#### 配置（[config.go](../../security/prompt_injection/config.go)）

```go
type Config struct {
    ProtectionLevel string    // "none" | "detect" | "block" | "sanitize"
    CustomPatterns  []Pattern
    // ...
}
```

- **detect**（默认）：仅记录不阻断
- **block**：检测到注入时返回 error 阻断
- **sanitize**：对可疑输入做转义/隔离

#### 接入点

```
作为 session.MessageHook:
    BeforeAddMessage() → DetectInput(content)
        if Detected && ProtectionLevel=="block":
            return ErrPromptInjectionBlocked

作为 Agent OutputSecurityHook:
    CheckOutput(response) → DetectOutput(response)
        if Detected && ProtectionLevel=="block":
            return error (阻止响应返回)
```

### 1.3 速率限制（security/ratelimit/）

#### 令牌桶（[bucket.go](../../security/ratelimit/bucket.go)）

```go
type TokenBucket struct {
    capacity   int           // 桶容量（最大突发）
    tokens     int           // 当前令牌数
    refillRate int           // 每分钟补充数
    lastRefill time.Time
    mu         sync.Mutex
}

func NewTokenBucket(capacity, refillRate int) *TokenBucket
func (b *TokenBucket) Take(count int) bool          // 非阻塞
func (b *TokenBucket) Wait(ctx context.Context) bool // 阻塞等待
```

#### Provider 级限流器（[provider_limiter.go](../../security/ratelimit/provider_limiter.go)）

```go
type ProviderLimiter struct { ... }
// 为每个 Provider 维护独立的令牌桶
```

#### 接入点

```
model.ratelimit_wrap.go 包装 Provider:
    RateLimitedProvider.Request(ctx, req)
        ├── bucket.Wait(ctx)          // 令牌桶等待
        └── inner.Request(ctx, req)   // 实际调用

orchestrator/agent/ratelimit_hook.go:
    RateLimitHook 在 RequestModel 前检查
```

### 1.4 RBAC 访问控制（security/rbac/）

| 文件 | 内容 |
|------|------|
| [context.go](../../security/rbac/context.go) | `RBACContext` 携带角色信息 |
| [matrix.go](../../security/rbac/matrix.go) | `PermissionMatrix` 角色×权限映射 |
| [policy.go](../../security/rbac/policy.go) | `Policy` 接口 + 授权规则 |
| [middleware.go](../../security/rbac/middleware.go) | HTTP/调用中间件，请求前校验权限 |
| [defaults.go](../../security/rbac/defaults.go) | 默认角色与策略 |
| [approval_integration.go](../../security/rbac/approval_integration.go) | 与 `sandbox.ApprovalService` 集成 |

---

## 二、observability 模块

`observability` 模块（`github.com/inferglow/observability`）提供 OpenTelemetry 集成，当前仅 `otel/` 子包。

### 2.1 核心类型（[tracer.go](../../observability/otel/tracer.go)）

```go
// InferGlow 语义 Span 类型
type SpanKind int
const (
    SpanAgentRun    SpanKind = iota  // 顶层 Agent 运行
    SpanLLMCall                       // LLM 调用
    SpanToolCall                      // 工具/Action 调用
    SpanFlowExecute                   // Flow 执行
    SpanPause                         // Flow 暂停
    SpanResume                        // Flow 恢复
)

type Tracer struct {
    tr trace.Tracer   // 包装 OTel tracer
}

func NewTracer(name string, opts ...trace.TracerOption) *Tracer
// 从全局 TracerProvider 解析 tracer
```

### 2.2 Span 类型

| 文件 | Span 类型 | 用途 |
|------|----------|------|
| [agent_span.go](../../observability/otel/agent_span.go) | Agent run span | 追踪 Agent.Run 全周期 |
| [llm_span.go](../../observability/otel/llm_span.go) | LLM call span | 追踪模型请求（含 token 用量） |
| [tool_span.go](../../observability/otel/tool_span.go) | Tool call span | 追踪 Action 执行 |

### 2.3 其他文件

| 文件 | 内容 |
|------|------|
| [attributes.go](../../observability/otel/attributes.go) | 通用 Span 属性定义 |
| [exporters.go](../../observability/otel/exporters.go) | Exporter 配置（Jaeger/OTLP/stdout） |

### 2.4 接入方式

`Tracer` 从全局 `TracerProvider` 解析，因此调用方通过 `InstallNewProvider` 配置全局 Provider 即可被所有 Tracer 感知。Span 在 orchestrator 关键路径手动创建。

---

## 三、workspace 模块

`workspace` 模块（`github.com/inferglow/workspace`）提供 Agent 工作空间：安全的文件读写、目录操作、路径管理。以 `rootDir` 为沙箱边界，通过 `filepath.Clean` + `filepath.Join` + 前缀校验三重防护阻止路径穿越。完全独立。

### 3.1 核心类型（[workspace.go](../../workspace/workspace.go)）

```go
type Config struct {
    RootDir        string   // 工作空间根目录（绝对路径）
    MaxFileSize    int64    // 单文件上限，默认 10MB
    MaxFileCount   int      // 文件数上限，默认 10000
    MaxFileNameLen int      // 文件名长度上限，默认 255
    ReadOnly       bool     // 只读模式
}

type Workspace struct {
    cfg Config
    mu  sync.RWMutex
}

func New(cfg Config) (*Workspace, error)
```

### 3.2 安全防护

| 错误 | 触发条件 |
|------|---------|
| `ErrPathOutsideRoot` | 路径解析后超出 rootDir |
| `ErrFileTooLarge` | 文件超过 MaxFileSize |
| `ErrTooManyFiles` | 文件数超过 MaxFileCount |
| `ErrFileNameTooLong` | 文件名超过 MaxFileNameLen |
| `ErrReadOnly` | 只读模式下尝试写入 |

### 3.3 血缘追踪（独立可选组件）（[workspace_lineage.go](../../workspace/workspace_lineage.go)）

**血缘追踪（LineageStore）是独立可选组件，与 workspace.go 解耦，需调用方显式集成，不自动嵌入文件操作。**

`workspace_lineage.go` 实现文件血缘追踪：记录文件的创建来源、修改链、依赖关系，支持通过父级资源推断子资源上下文。

### 3.4 关键调用链

```
workspace.WriteFile(path, content)
    │
    ├──[1] resolvePath(path)
    │       filepath.Clean + filepath.Join(rootDir, path)
    │       前缀校验: 确保结果在 rootDir 内
    │       (否则返回 ErrPathOutsideRoot)
    │
    ├──[2] 检查 ReadOnly → ErrReadOnly
    │
    ├──[3] 检查文件大小 → ErrFileTooLarge
    │
    ├──[4] 检查文件数 → ErrTooManyFiles
    │
    ├──[5] os.WriteFile(resolved, content, 0644)
    │
    └──[6] 记录血缘 (workspace_lineage)
```
