# 10 · 设计模式与架构决策

> 基于源码分析 + Graphify 知识图谱（8017 节点，17577 边，414 社区）的全面架构文档
> 生成日期：2026-07-31

---

## 一、设计模式清单

| 模式 | 使用位置 | 说明 |
|------|---------|------|
| **接口注入** | security → session/orchestrator | 可选安全特性，不注入即零开销 |
| **Build Tag** | action → sandbox | `//go:build with_sandbox` 可选编译 |
| **策略模式** | session.ResizeHandler | 4 种裁剪策略可互换 |
| **工厂模式** | model.ProviderFactory | 从配置创建 Provider |
| **组合模式** | model.FailoverModelRequester | 组合多个 ModelRequester |
| **观察者模式** | flow.SignalNet | 事件驱动信号网络 |
| **责任链模式** | orchestrator/middleware | 中间件链 |
| **模板方法** | flow.Step → StepFunc | 步骤执行框架 |
| **适配器模式** | security/agenthook | PIIMasker 适配 |
| **Builder 模式** | flow.NewFlow().AddStep().To().Build() | 链式构建 |
| **Registry 模式** | action.ActionRegistry | 动作注册发现 |
| **哨兵错误** | flow.ErrPauseRequested | 暂停信号 |

---

### 1. 接口注入（Interface Injection）

**使用位置：** `security/sessionhook` → `session`，`security/agenthook` → `orchestrator/agent`

**代码示例：**

```go
// session/session.go — 轻量接口定义
type MessageHook interface {
    BeforeAddMessage(ctx context.Context, role, content string) error
    AfterAddMessage(ctx context.Context, role, content string)
}

// security/sessionhook/hook.go — 实现
var _ session.MessageHook = (*SecurityHook)(nil)

// 使用方无感知：不注入即零开销
sess := session.NewSessionWithOptions("id", 4000,
    session.WithSecurityHook(secHook), // 可选注入
)
```

**为什么选择接口注入：** 相比编译时选择（Build Tag），接口注入让安全特性在运行时动态启用，且对 `session` 和 `orchestrator` 模块完全透明——这两个模块不依赖 `security` 包，零耦合。

---

### 2. Build Tag（编译标签）

**使用位置：** `action/executor_sandbox.go`

**代码示例：**

```go
//go:build with_sandbox

package action

func NewSandboxExecutor(config SandboxExecutorConfig) *SandboxExecutor {
    // 完整实现
}
```

```go
//go:build !with_sandbox

package action

func NewSandboxExecutor(config SandboxExecutorConfig) *SandboxExecutor {
    return &SandboxExecutor{} // 调用 Execute 返回错误
}
```

**为什么选择 Build Tag：** 沙箱功能涉及大量操作系统级依赖（Docker、gVisor、Seatbelt 等），编译为独立二进制时体积差异显著。通过 `//go:build with_sandbox` 标签，默认构建不包含沙箱，体积更小、编译更快；需要沙箱时显式启用。对比接口注入，Build Tag 适用于**编译时**可选功能，而接口注入适用于**运行时**可选功能。

---

### 3. 策略模式（Strategy Pattern）

**使用位置：** `session.ResizeHandler`

**代码示例：**

```go
// session/session.go
type ResizeHandler func(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error)

// 策略 1: SimpleCutResizeHandler — 从前面丢弃，保留最近的
func SimpleCutResizeHandler(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error) {
    // trim from front until fits
}

// 策略 2: SummaryFirstResizeHandler — 保留首条 + 末尾 2 条 + 中间摘要
func SummaryFirstResizeHandler(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error) {
    // keep first, summarize middle, keep last two
}

// 策略 3: TokenAwareResizeHandlerWithMax — 按 token 估算裁剪
func TokenAwareResizeHandlerWithMax(maxLength int) ResizeHandler {
    return func(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error) {
        // token-aware trimming
    }
}
```

**为什么选择策略模式：** 上下文窗口裁剪策略因应用场景不同而差异巨大——简单聊天用 `SimpleCut`，保留系统提示用 `SummaryFirst`，精确控制预算用 `TokenAware`。策略模式将算法封装为函数类型，允许调用方在 `Session` 构造时通过 `WithResizeHandler` 注入，无需修改 `Session` 核心逻辑。对比继承，Go 的函数类型签名更轻量；对比配置参数枚举，策略模式更易扩展新策略。

---

### 4. 工厂模式（Factory Pattern）

**使用位置：** `model.ProviderFactory`

**代码示例：**

```go
// model/provider_factory.go
func NewOpenAIProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
    cfg, err := LoadProviderConfig(cp, "openai")
    if err != nil {
        return nil, fmt.Errorf("load openai provider config: %w", err)
    }
    return &OpenAICompatibleProvider{
        BaseURL:    cfg.BaseURL,
        APIKey:     cfg.APIKey,
        Model:      cfg.Model,
        HTTPClient: cfg.HTTPClient,
    }, nil
}

func NewAnthropicProviderFromConfig(cp ConfigProvider) (*AnthropicCompatibleProvider, error) {
    // ...
}

func NewDeepSeekProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
    // ...
}
```

**为什么选择工厂模式：** 每个 Provider 的构造逻辑各不相同（不同 API 协议、不同认证方式、不同角色映射），工厂函数封装了这些差异。调用方只需传入统一的 `ConfigProvider` 接口，无需了解 Provider 内部构造细节。对比直接 `new` 构造，工厂模式支持从配置文件/环境变量统一创建；对比 `Provider` 接口自身承担构造责任，工厂模式遵循单一职责原则。

---

### 5. 组合模式（Composite Pattern）

**使用位置：** `model.FailoverModelRequester`

**代码示例：**

```go
// model/failover.go
type FailoverModelRequester struct {
    providers []modelProviderEntry // ordered by priority
    mu        sync.RWMutex
    config    FailoverConfig
    lastIndex int
}

func NewFailoverModelRequester(providers []ModelRequester, config FailoverConfig) *FailoverModelRequester {
    entries := make([]modelProviderEntry, len(providers))
    for i, p := range providers {
        entries[i] = modelProviderEntry{
            requester: p,
            name:      p.Name(),
            healthy:   true,
        }
    }
    return &FailoverModelRequester{
        providers: entries,
        config:    normalizeFailoverConfig(config),
        lastIndex: -1,
    }
}

// 组合后的对象同样实现 ModelRequester 接口
func (f *FailoverModelRequester) RequestModel(ctx context.Context, req *ModelRequest) (*ModelResponse, error) {
    // 按优先级依次尝试，失败自动切换到下一个
}
```

**为什么选择组合模式：** `FailoverModelRequester` 将多个 `ModelRequester` 组合为一个整体，对外暴露相同的接口。调用方可以像使用单个 Provider 一样使用 Failover 组合，完全透明。对比在调用方实现故障转移逻辑，组合模式将"重试+切换"封装在单一对象中，职责清晰。对比装饰器模式，组合模式侧重于"多个实例的编排"，而非"职责的叠加"。

---

### 6. 观察者模式（Observer Pattern）

**使用位置：** `flow.SignalNet`

**代码示例：**

```go
// flow/signal.go
type SignalNet struct {
    staticHandlers  map[string]map[string]Handler  // triggerEvent -> handlerName -> Handler
    dynamicBindings map[string]*DynamicBinding
    // ...
}

func NewSignalNet() *SignalNet {
    return &SignalNet{
        staticHandlers: make(map[string]map[string]Handler),
        // ...
    }
}

// 注册观察者（Handler）
func (sn *SignalNet) RegisterStaticHandler(triggerEvent, name string, handler Handler) {
    // ...
}

// 发布信号
func (sn *SignalNet) Emit(signal Signal) {
    // 通知所有匹配的 Handler
}
```

**为什么选择观察者模式：** TriggerFlow 的事件驱动模型天然符合观察者模式——`SignalNet` 作为事件总线，`Handler` 作为观察者。信号触发时，所有注册的 Handler 按顺序执行。对比 channel 直接通信，SignalNet 提供了一对多的订阅/发布能力；对比回调注册表，SignalNet 支持动态绑定和静态注册两种模式，并内置了信号去重和 TTL 清理机制。

---

### 7. 责任链模式（Chain of Responsibility）

**使用位置：** `orchestrator/middleware`

**代码示例：**

```go
// orchestrator/middleware/middleware.go
type Handler func(ctx context.Context, input *Input) (*Output, error)

type Middleware func(next Handler) Handler

// 使用示例
func LoggingMiddleware(next Handler) Handler {
    return func(ctx context.Context, input *Input) (*Output, error) {
        log.Printf("request: %s", input.SessionID)
        out, err := next(ctx, input)
        log.Printf("response: %v", out)
        return out, err
    }
}

// orchestrator/agent/middleware.go
func WithMiddleware(mw ...middleware.Middleware) RunOption {
    return func(c *runConfig) {
        c.middlewares = append(c.middlewares, mw...)
    }
}
```

**为什么选择责任链模式：** 中间件是 Agent 执行路径的横切关注点（日志、追踪、限流、权限检查等），责任链模式将这些关注点与核心业务逻辑解耦。每个中间件只关心自己的职责，可以自由组合。对比嵌入 if-else 或装饰器嵌套，责任链模式提供类型安全的组合方式，且与 `net/http` 的 `Handler` 模式一致，零学习成本。中间件顺序即执行顺序，第一个注入的中间件是最外层包装。

---

### 8. 模板方法模式（Template Method）

**使用位置：** `flow.Step → StepFunc`

**代码示例：**

```go
// flow/step.go
type StepFunc func(ctx context.Context, input any) (any, error)

type Step struct {
    Name   string
    Func   StepFunc
    Schema *schema.OutputSchema
}

// flow/engine.go — 模板方法
func (f *Flow) Execute(ctx context.Context, input any) *Execution {
    // 1. 初始化执行状态
    // 2. 遍历 steps
    for _, step := range f.steps {
        // 3. Schema 校验（如配置）
        if step.Schema != nil {
            // 校验
        }
        // 4. 执行 StepFunc
        result, err := step.Func(ctx, currentInput)
        // 5. 错误处理（含哨兵错误检查）
        if err != nil && errors.Is(err, ErrPauseRequested) {
            exec.State.Status = StatusPaused
            return exec
        }
        // 6. 结果传递到下一步
        currentInput = result
    }
    // 7. 返回最终结果
}
```

**为什么选择模板方法模式：** `Flow.Execute` 定义了"步骤执行的骨架"（初始化→遍历→校验→执行→错误处理→传递），而每个 `Step` 的 `StepFunc` 填充"具体步骤的行为"。对比完全的策略模式，模板方法确保了所有步骤共享相同的执行生命周期（校验、错误处理、上下文传递），避免每个 StepFunc 重复实现这些横切逻辑。对比回调，模板方法让 Flow 拥有对执行流程的完全控制权。

---

### 9. 适配器模式（Adapter Pattern）

**使用位置：** `security/agenthook`

**代码示例：**

```go
// security/agenthook/pii_adapter.go
var _ agent.PIIMasker = (*PIIMasker)(nil)

// PIIMasker 适配 *pii.Masker 到 agent.PIIMasker 接口
type PIIMasker struct {
    m *pii.Masker
}

func NewPIIMasker(m *pii.Masker) *PIIMasker {
    return &PIIMasker{m: m}
}

func (p *PIIMasker) MaskInput(text string) string {
    if p == nil || p.m == nil {
        return text
    }
    return p.m.MaskInput(text)
}
```

**为什么选择适配器模式：** `pii.Masker` 在 `security/pii` 包中定义，提供完善的 PII 脱敏能力；但 `orchestrator/agent` 只需要一个简单的 `MaskInput(text) string` 接口。适配器 `PIIMasker` 在 `security/agenthook` 中桥接两者，使 `orchestrator` 无需依赖 `security/pii` 的具体类型。对比直接修改 `pii.Masker` 的接口，适配器模式不破坏原有类型；对比在 `orchestrator` 中重复实现，适配器模式实现了复用。

---

### 10. Builder 模式（Builder Pattern）

**使用位置：** `flow.NewFlow().AddStep().To().Build()`

**代码示例：**

```go
// flow/flow.go
type FlowBuilder struct {
    flow     *Flow
    lastStep *Step
}

func NewFlow() *FlowBuilder {
    return &FlowBuilder{
        flow: &Flow{
            steps: make(map[string]*Step),
        },
    }
}

func (fb *FlowBuilder) AddStep(step *Step) *FlowBuilder {
    fb.flow.steps[step.Name] = step
    if fb.flow.startStep == nil {
        fb.flow.startStep = step
    }
    fb.lastStep = step
    return fb
}

func (fb *FlowBuilder) To(step *Step) *FlowBuilder {
    if fb.lastStep != nil {
        fb.flow.edges = append(fb.flow.edges, Edge{
            From: fb.lastStep.Name,
            To:   step.Name,
        })
    }
    fb.flow.steps[step.Name] = step
    fb.lastStep = step
    return fb
}

func (fb *FlowBuilder) Build() *Flow {
    return fb.flow
}

// 使用示例
flow := NewFlow().
    AddStep(step1).
    To(step2).
    If(cond, trueStep, falseStep).
    WithOptions(WithAutoCheckpoint(store)).
    Build()
```

**为什么选择 Builder 模式：** Flow 的构建需要顺序添加 Step、连接边、设置分支和选项，参数繁多。Builder 模式通过链式 API 让构建过程清晰可读，且构建期（Builder）与运行期（Flow）分离——Builder 可加写锁保护并发，Build 后返回的 Flow 只有读操作。对比构造函数参数列表，Builder 模式避免了"构造器爆炸"；对比 JSON 配置，Builder 模式提供编译期类型安全。

---

### 11. Registry 模式（Registry Pattern）

**使用位置：** `action.ActionRegistry`

**代码示例：**

```go
// action/action.go
type ActionRegistry struct {
    mu      sync.RWMutex
    actions map[string]*Action
}

func NewRegistry() *ActionRegistry {
    return &ActionRegistry{
        actions: make(map[string]*Action),
    }
}

func (r *ActionRegistry) Register(a *Action) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    if _, ok := r.actions[a.Name]; ok {
        return ErrActionAlreadyRegistered
    }
    r.actions[a.Name] = a
    return nil
}

func (r *ActionRegistry) Execute(ctx context.Context, name string, input map[string]any) (*ActionResult, error) {
    r.mu.RLock()
    a, ok := r.actions[name]
    r.mu.RUnlock()
    if !ok {
        return nil, ErrActionNotFound
    }
    return a.Executor.Execute(ctx, input)
}
```

**为什么选择 Registry 模式：** Action 需要全局注册、按名查找和执行。Registry 模式提供了集中式的管理容器，支持在运行时动态注册/注销 Action。对比在编译期硬编码所有 Action 的 switch-case，Registry 模式让 Action 可独立添加、测试和发布。`sync.RWMutex` 保护并发安全，读操作（Execute/Get/List）使用读锁，写操作（Register/Unregister）使用写锁，性能友好。

---

### 12. 哨兵错误（Sentinel Error）

**使用位置：** `flow.ErrPauseRequested`

**代码示例：**

```go
// flow/flow_context.go
var ErrPauseRequested = errors.New("flow: pause requested by step")

// 使用场景
func (fc *flowContextImpl) RequestPause(reason string) error {
    // 记录暂停点
    return ErrPauseRequested
}

// flow/engine.go — 捕获处理
if err != nil && errors.Is(err, ErrPauseRequested) {
    exec.State.Status = StatusPaused
    // 不追加到 Errors，仅记录暂停
    return exec
}
```

**为什么选择哨兵错误：** Flow 的暂停是 "正常流程控制" 的一部分，而非异常。使用哨兵错误 `ErrPauseRequested` 传递暂停信号，让暂停路径与普通错误路径在引擎层面统一处理（都是 `error` 返回值），但在语义上区分（`errors.Is` 检查）。对比使用单独的 `pauseCh` channel，哨兵错误让暂停信号在步骤函数返回值中自然传递，无需额外参数。对比布尔返回值，哨兵错误可被 `fmt.Errorf("...: %w", ErrPauseRequested)` 包装，兼容 Go 的错误链模式。

---

## 二、关键架构决策

| 决策 | 选择 | 替代方案 | 理由 |
|------|------|---------|------|
| 模块拆分粒度 | 23 个独立 Go module | 大单体 | 独立发布、测试、复用 |
| 依赖方向 | 严格单向（基础层→中间层→编排层→应用层） | 双向依赖 | 避免循环依赖 |
| 安全架构 | 接口注入 + Build Tag | 编译时选择 | 零开销不使用时 |
| FlowContext | 接口定义在 `flow` 包，实现在 `orchestrator` | 直接依赖 | 避免 `flow` 依赖 `orchestrator` |
| 持久化 | 文件系统（`FileCheckpointStore`） | 数据库 | 零配置，简单部署 |
| 传输协议 | MCP JSON-RPC 2.0 | 自定义协议 | 标准协议，可互操作 |
| 序列化 | JSON/YAML | Protobuf | 可读性优先 |
| 流式处理 | channel-based | callback-based | goroutine 天然适配 |

### 决策详解

#### 1. 模块拆分粒度：23 个独立 Go module

**选择理由：** 每个模块拥有独立的 `go.mod`，可以独立发布、版本化、测试和复用。例如 `model` 模块仅依赖标准库 + `yaml.v3`，可被其他项目直接 import 而无需引入整个框架。Graphify 验证确认零循环依赖。

**代价：** 模块间调用需要经过 `go.mod replace` 或发布后的版本引用，开发期间需要在根 `go.mod` 中配置 `replace` 指令。

#### 2. 依赖方向：严格单向

```
应用层 → 编排层 → 中间层 → 基础层
```

**选择理由：** 双向依赖会导致循环依赖，Go 编译器会直接拒绝编译。严格单向依赖确保每层只依赖下层，上层变更不影响下层——这是 Graphify 检测确认的"无 import cycle"的根本保证。

#### 3. 安全架构：接口注入 + Build Tag

**选择理由：** 安全特性（PII 脱敏、注入检测、沙箱）是"可选"而非"核心"功能。接口注入让运行时安全特性零开销（不注入即不执行），Build Tag 让编译时沙箱特性零开销（不构建即不包含）。两者结合覆盖了完整的安全可选范围。

#### 4. FlowContext：接口定义在 flow，实现在 orchestrator

```go
// flow 包定义接口
type FlowContext interface {
    ExecuteAction(ctx context.Context, name string, params map[string]any) (any, error)
    GenerateModel(ctx context.Context, system, userMessage string) (string, error)
    // ...
}

// orchestrator 包实现
type flowContextImpl struct {
    engine    *Engine
    session   *SessionExtension
    // ...
}
var _ flow.FlowContext = (*flowContextImpl)(nil)
```

**选择理由：** 如果 `flow.Context` 直接定义在 `orchestrator`，则 `flow` 包需要依赖 `orchestrator`，造成循环依赖（`orchestrator → flow → orchestrator`）。接口定义在 `flow`、实现在 `orchestrator` 的经典模式避免了这一循环，同时也让 `flow` 包可独立测试。

#### 5. 持久化：文件系统

**选择理由：** 项目当前阶段优先"零配置、简单部署"，文件系统方案无需额外依赖（数据库服务、容器等）。`FileCheckpointStore` 基于 JSON 序列化，可直接用文本编辑器查看和修改。未来可演进为 `PostgresCheckpointStore` 等数据库实现。

#### 6. 传输协议：MCP JSON-RPC 2.0

**选择理由：** 选择 MCP（Model Context Protocol）标准协议而非自定义协议，确保与主流 AI 工具链的互操作性。JSON-RPC 2.0 是成熟、简洁的 RPC 协议，Go 标准库的 `json` 包即可完成序列化/反序列化。

#### 7. 序列化：JSON/YAML

**选择理由：** 可读性优先——JSON 和 YAML 是人类可读的文本格式，适合调试、日志和配置。Protobuf 虽然更紧凑、更高效，但引入编译期代码生成、Schema 管理复杂度超出了当前阶段的收益。

#### 8. 流式处理：channel-based

**代码示例：**

```go
// channel-based 流式处理
type StreamRequester interface {
    ModelRequester
    RequestStream(ctx context.Context, req *ModelRequest) (<-chan StreamChunk, error)
}
```

**选择理由：** Go 的 goroutine + channel 天然适配流式处理——生产者 goroutine 发送到 channel，消费者 goroutine 从 channel 接收，无需手动管理回调注册和生命周期。对比 callback-based 方案，channel 提供了更清晰的"生产者-消费者"边界，且 `select` 语句让超时/取消处理更优雅。

---

## 三、Go 语言适配对照

| Python 特性 | Go 适配方案 |
|------------|------------|
| `ContextVar` | `context.Context` + 值传递 |
| `Pydantic TypeAdapter` | Go 泛型 + 反射 + JSON Schema |
| 装饰器 (`@agent.tool_func`) | Go func + 显式注册 |
| `async/await` | goroutine + channel |
| `TypedDict` | Go struct |
| `Protocol` (typing) | Go interface |
| `asyncio.Event/Lock` | Go channel + `sync.Mutex` |
| `pip module` | Go module + `go.mod replace` |

### 对照详解

#### 1. ContextVar → `context.Context` + 值传递

**Python 的 `ContextVar`** 提供隐式上下文传递，在同一个协程内自动可见。

**Go 的适配方案：** 使用 `context.Context` 显式传递，辅以 `context.WithValue` 和自定义 getter 函数：

```go
// 定义 key
type contextKey string

const flowContextKey contextKey = "flow_context"

// 写入
ctx = context.WithValue(ctx, flowContextKey, fc)

// 读取
func FlowContextFrom(ctx context.Context) FlowContext {
    v, _ := ctx.Value(flowContextKey).(FlowContext)
    return v
}
```

**为什么：** Go 没有隐式上下文。`context.Context` 是 Go 社区的标准做法，getter 函数封装了类型断言细节，调用方无需关心内部存储结构。

#### 2. Pydantic TypeAdapter → Go 泛型 + 反射 + JSON Schema

**Python 的 `Pydantic TypeAdapter`** 从 Python 类型注解自动推导 JSON Schema 并校验。

**Go 的适配方案：** 使用 Go 泛型定义 `DefineOutput[T any]()`，结合反射提取结构体字段标签，生成 JSON Schema：

```go
type OutputSchema struct {
    Name        string
    Description string
    Properties  map[string]*FieldDef
    Required    []string
}

// 泛型推导
func DefineOutput[T any]() *OutputSchema {
    var t T
    // 反射提取字段信息
    // 生成 JSON Schema
}
```

**为什么：** Go 没有 Python 那样丰富的运行时类型信息。泛型 + 反射的组合在编译期（泛型约束）和运行时（反射提取）之间取得了平衡，提供了接近 Pydantic 的开发体验。

#### 3. 装饰器 → Go func + 显式注册

**Python 的 `@agent.tool_func`** 通过装饰器语法糖在函数定义时自动注册。

**Go 的适配方案：** 调用 `Register` 函数将 Go 函数显式注册为 Action：

```go
registry := action.NewRegistry()
registry.Register(&action.Action{
    Name: "web_search",
    Executor: action.NewLocalFunctionExecutor(WebSearch),
})
```

**为什么：** Go 没有装饰器语法。显式注册虽然代码量稍多，但意图清晰——注册发生在 `init()` 或 `main()` 中，无"魔法"，易于调试和测试。

#### 4. async/await → goroutine + channel

**Python 的 `async/await`** 提供协作式异步编程模型。

**Go 的适配方案：** goroutine 启动并发任务，channel 传递结果：

```go
resultCh := make(chan string, 1)
go func() {
    result, err := doWork(ctx)
    if err != nil {
        resultCh <- ""
    }
    resultCh <- result
}()
select {
case result := <-resultCh:
    return result
case <-ctx.Done():
    return "", ctx.Err()
}
```

**为什么：** goroutine 比 Python 的协程更轻量（栈初始仅 2KB），channel 提供了类型安全的通信机制。`select` 语句天然支持超时和取消，无需 `asyncio.wait_for` 等额外工具。

#### 5. TypedDict → Go struct

**Python 的 `TypedDict`** 提供带类型注解的字典。

**Go 的适配方案：** Go struct 是类型安全的字段集合：

```go
type ChatMessage struct {
    Role    string
    Content []ContentBlock
}
```

**为什么：** Go struct 是静态类型语言的最自然选择，编译期确保字段访问的正确性，无运行时类型错误。

#### 6. Protocol → Go interface

**Python 的 `Protocol`** 定义结构化子类型（structural subtyping）。

**Go 的适配方案：** Go interface 提供隐式实现（duck typing）：

```go
type ModelRequester interface {
    RequestModel(ctx context.Context, req *ModelRequest) (*ModelResponse, error)
}
```

**为什么：** Go interface 天然支持隐式实现——无需 `implements` 关键字，类型只要实现了接口方法即满足接口。这与 Python Protocol 的"结构兼容"理念一致，但 Go 在编译期检查。

#### 7. asyncio.Event/Lock → Go channel + sync.Mutex

**Python 的 `asyncio.Event`** 提供协程间的事件通知。

**Go 的适配方案：** channel 提供同步/通知，`sync.Mutex` 提供互斥访问：

```go
// 事件通知
done := make(chan struct{})
// 等待
<-done
// 通知
close(done)
```

**为什么：** Go 的 channel 同时具备事件通知和数据传递能力，`sync.Mutex` 处理临界区保护。Go 的并发原语比 Python 的 asyncio 原语更底层但也更灵活，且无事件循环开销。

#### 8. pip module → Go module + go.mod replace

**Python 的 `pip install`** 从 PyPI 安装包。

**Go 的适配方案：** Go module 通过 `go.mod` 管理依赖，开发期间使用 `replace` 指令本地调试：

```
module github.com/inferglow/orchestrator

require (
    github.com/inferglow/flow v0.1.0
    github.com/inferglow/model v0.1.0
)

replace (
    github.com/inferglow/flow => ../flow
    github.com/inferglow/model => ../model
)
```

**为什么：** Go module 的 `replace` 指令允许在开发阶段使用本地路径，无需每次修改后发布到远程仓库。23 个独立 module 的开发调试正是依赖这一机制。

---

## 小结

本章从三个维度审视了 Inferglow 的架构设计：

1. **设计模式清单**（12 种模式）：覆盖了从模块组装（接口注入、Builder）、运行机制（策略、工厂、组合、观察者、责任链、模板方法）、跨模块适配（适配器、Registry）到错误处理（哨兵错误）的完整设计空间。

2. **关键架构决策**（8 项）：从模块拆分粒度、依赖方向、安全架构、接口定义、持久化、传输协议、序列化到流式处理，每一项决策都基于"当前阶段最优"的原则，在灵活性、性能和简洁性之间取得了平衡。

3. **Go 语言适配对照**（8 项）：展示了如何将 Python Agently 的设计理念映射到 Go 的静态类型系统和并发模型上。核心思路是"尊重 Go 的惯用法，而非强行模仿 Python 模式"。

这些设计模式和架构决策共同构成了 Inferglow 的"架构 DNA"——模块化、可扩展、零开销抽象、类型安全，同时也是 Graphify 知识图谱中 8017 个节点、17577 条边背后的设计逻辑。

---

*本文档由 Inferglow 源码分析 + Graphify 知识图谱（8017 节点，17577 边）联合生成*