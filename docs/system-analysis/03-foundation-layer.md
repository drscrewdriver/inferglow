# 03 · 基础层详解

基础层由 12 个独立 Go module 组成，零内部循环依赖，为上层提供基础设施零件。总代码量约 25,000 LOC，每个模块均可独立编译、测试和复用。

---

## 一、model — LLM Provider 统一抽象

**模块路径：** `github.com/inferglow/model`

**依赖：** 无（仅 stdlib + yaml.v3）

**代码量：** ~8000 LOC

### 核心接口

```go
// 核心接口
type ModelRequester interface {
    RequestModel(ctx context.Context, req *ModelRequest) (*ModelResponse, error)
}
type StreamRequester interface {
    ModelRequester
    RequestStream(ctx context.Context, req *ModelRequest) (<-chan StreamChunk, error)
}
```

### 核心类型

```go
// 请求/响应模型
type ModelRequest struct {
    Model       string
    Messages    []ChatMessage
    Tools       []ToolDefinition
    Output      *OutputSchema
    MaxTokens   int
    Temperature float64
    // ...
}
type ModelResponse struct {
    Content      string
    ToolCalls    []ToolCall
    Usage        *UsageInfo
    // ...
}
type StreamChunk struct {
    Delta        string
    ToolCalls    []ToolCall
    Reasoning    string
    ContentBlock *ContentBlock
    // ...
}
```

### Provider 实现

| Provider | 协议 | 特点 |
|----------|------|------|
| `OpenAICompatibleProvider` | `/chat/completions` SSE | 兼容 OpenAI/DeepSeek/vLLM |
| `AnthropicCompatibleProvider` | `/messages` SSE | 兼容 Anthropic/SenseNova |
| `OllamaProvider` | `/api/chat` SSE | 本地模型 |
| `OpenAIResponsesProvider` | `/responses` | 新版 API |
| `FailoverModelRequester` | 组合模式 | 自动故障转移 |
| `ModelPool` | 连接池 | 多 Provider 轮转 |

### Schema 四层校验

模型层实现了一个四层递进校验架构，确保 LLM 输出严格符合预期 Schema：

```
L1 硬约束（XGrammar token 级）→ ~100% 准确
    ↓
L2 API 约束（云端 structured output）→ ~99% 准确
    ↓
L3 Prompt 兜底（system prompt 注入）→ ~80% 准确
    ↓
L4 后置校验（JSON 结构 + 重试）→ 检测层
```

- **L1 — XGrammar**：在 token 生成阶段通过 Grammar 约束强制结构，最严格但仅部分 Provider 支持
- **L2 — API 约束**：利用 Provider 原生 structured output / JSON mode 能力
- **L3 — Prompt 兜底**：在 system prompt 中注入 Schema 描述，指导模型按格式输出
- **L4 — 后置校验**：对最终输出进行 JSON 结构校验和路径校验，失败时触发重试

### 流式归一化

`LeadingThinkNormalizer` 是一个三态状态机，用于分离 ` 思考中...` 推理内容与最终回复：

```go
// 状态转换
type normalizerState int
const (
    stateNormal   normalizerState = iota  // 普通文本
    stateInThink                          // 处理中
    stateThinkDone                        // 已完成
)
```

该状态机在流式场景下实时处理 `StreamChunk`，将推理内容与回复内容分开输出，同时通过 `UsageInfo.PromptTokensDetails["cached_tokens"]` 回传 Prefix Cache 命中信息，为上层上下文管理提供预算参考。

---

## 二、schema — Contract-First Schema 引擎

**模块路径：** `github.com/inferglow/schema`

**依赖：** 无（仅 yaml.v3）

**代码量：** ~2800 LOC

### 核心类型

```go
type OutputSchema struct {
    Name        string
    Description string
    Properties  map[string]*FieldDef
    Required    []string
    // ...
}
type FieldDef struct {
    Name        string
    Type        DataType
    Description string
    Required    bool
    // ...
}
type ContractEngine struct {
    // 编译期+运行时双重校验
}
```

### 核心功能

| 功能 | 机制 | 用途 |
|------|------|------|
| 泛型推导 | `DefineOutput[T any]()` | 从 Go struct 自动生成 JSON Schema |
| JSON Schema 转换 | `BuildJSONSchemaFromOutput()` | 转换为 Provider 的 `response_format` |
| 路径校验 | `ValidatePath()` | L4 后置校验：检查输出路径匹配 |
| JSON 提取 | `ExtractJSON()` | 从 LLM 原始响应中提取结构化 JSON |
| Blueprint 序列化 | `Blueprint` | 流式输出 Schema 持久化 |

`ContractEngine` 是 schema 模块的核心编排器，负责：

1. **编译期校验**：通过 `DefineOutput[T]()` 在编译期捕获类型错误
2. **运行时校验**：对 LLM 输出进行 JSON Schema 结构校验
3. **路径定位**：当校验失败时，通过 `ValidatePath` 精确指出不符合的字段路径

---

## 三、session — 对话记忆管理

**模块路径：** `github.com/inferglow/session`

**依赖：** 无（安全特性通过 `MessageHook` 接口注入）

**代码量：** ~1800 LOC

### 双列表架构

```go
type Session struct {
    FullContext    []ChatMessage  // 完整历史（永不裁剪）
    ContextWindow  []ChatMessage  // 当前窗口（可能被裁剪）
    // ...
}
type ChatMessage struct {
    Role    string
    Content []ContentBlock
    // ...
}
type ContentBlock struct {
    Type    string        // "text" | "image_url" | "tool_use" | "tool_result"
    Text    string
    Image   *ImageBlock
    ToolUse *ToolUseBlock
    // ...
}
```

`FullContext` 保留完整对话历史，`ContextWindow` 是当前发送给 LLM 的窗口视图。这种双列表设计使得上下文裁剪不会丢失原始数据，支持历史回溯和审计。

### 4 种上下文窗口管理策略

| 策略 | 行为 | 适用场景 |
|------|------|---------|
| `SimpleCutResizeHandler` | 从前面丢弃，保留最近的 N 条 | 简单聊天 |
| `SummaryFirstResizeHandler` | 保留首条（系统提示）+ 末尾 2 条 + 中间摘要 | 需要保留系统提示 |
| `TokenAwareResizeHandler` | 按 token 估算精确裁剪 | 精确控制 token 预算 |
| `SmartCompressResizeHandler` | 智能压缩：保留关键信息 | 复杂对话 |

### Memory 持久化接口

```go
type Memory interface {
    Load(ctx context.Context) ([]ChatMessage, error)
    Save(ctx context.Context, msgs []ChatMessage) error
    Clear(ctx context.Context) error
}
```

| 实现 | 说明 |
|------|------|
| `SummaryMemory` | Token 阈值触发自动摘要 |
| `TokenBufferMemory` | Token 预算裁剪历史 |
| `InMemoryStore` | 内存存储（server 模块使用） |

---

## 四、sandbox — 沙箱执行框架

**模块路径：** `github.com/inferglow/sandbox`

**依赖：** 无（完全独立）

**代码量：** ~6300 LOC

### 8 种沙箱后端

| 后端 | Provider | 适用平台 | 隔离级别 |
|------|---------|---------|---------|
| Docker | `DockerProvider` | Linux | 容器级 |
| gVisor | `GVisorProvider` | Linux | 系统调用级 |
| 本地 | `LocalProvider` | 任意 | 无隔离 |
| TrustedLocal | `TrustedLocalProvider` | 任意 | 命令白名单 |
| Seatbelt | `SeatbeltProvider` | macOS | 系统调用级 |
| E2B | `E2BProvider` | 云 | 远程沙箱 |
| RestrictedToken | 内置 | Windows | 进程级 |
| AppContainer | 内置 | Windows | 容器级 |
| Windows Sandbox (WSB) | 内置 | Windows | 虚拟机级 |

### 核心接口

```go
type Provider interface {
    Name() string
    CreateHandle(ctx context.Context, policy ExecutionPolicy) (Handle, error)
}
type Handle interface {
    Execute(ctx context.Context, command Command) (ExecutionResult, error)
    Status() HandleStatus
    Close() error
}
type ExecutionPolicy struct {
    Mode         SandboxMode
    AllowedPaths []string
    Timeout      time.Duration
    Network      bool
    // ...
}
```

沙箱框架采用 Provider-Handle 模式：

- **Provider**：工厂接口，负责根据 `ExecutionPolicy` 创建沙箱实例
- **Handle**：沙箱实例句柄，封装命令执行、状态查询和资源清理
- **ExecutionPolicy**：执行策略，定义隔离级别、路径白名单、超时和网络访问等

8 种后端覆盖了从无隔离（本地）到虚拟机级隔离（WSB）的完整谱系，通过 `ExecutionPolicy` 统一配置接口。

---

## 五、context — 上下文管理引擎

**模块路径：** `github.com/inferglow/context`

**依赖：** 无（完全独立）

**代码量：** ~6300 LOC

### 三区压缩架构

```go
type HybridManager struct {
    HotZone    []ChatMessage      // 热区：最近 N 条，不压缩
    WarmZone   []ChatMessage      // 温区：压缩后保留的关键信息
    ColdZone   []ArchivedMessage  // 冷区：摘要归档，历史沉淀
    // ...
}
```

三区架构将对话历史分为三个温度带，每个区域采用不同的压缩策略：

| 区域 | 行为 | 策略 |
|------|------|------|
| **HotZone（热区）** | 最近 N 条消息，不压缩 | 直接保留全部细节 |
| **WarmZone（温区）** | 压缩后保留的关键信息 | 提取关键实体、决策、结论 |
| **ColdZone（冷区）** | 摘要归档 | 生成高层摘要，丢弃原始细节 |

### 核心功能

| 功能 | 说明 |
|------|------|
| **Sweet-spot 自适应阈值** | 根据上下文总长度动态调整热区/温区/冷区的划分比例 |
| **Prefix Cache 预算** | `CacheBudgetUpdater` 回调，为 Provider 的 Prefix Cache 预留 token 预算 |
| **宪法区（Zone 0.5）** | 不可变系统提示区域，位于 HotZone 之前，不受压缩影响 |
| **三问重组** | 按任务相关性（What/Why/How）对消息进行重新组织 |
| **衰减预热** | 失效预热机制，当某条消息被多次引用时提升其温度级别 |

`HybridManager` 是 context 模块的核心，它通过分析当前对话状态和 token 预算，自动决定哪些消息需要保留细节、哪些可以压缩、哪些应该归档，实现上下文窗口的最优利用。

---

## 六、audit — 链表式审计链

**模块路径：** `github.com/inferglow/audit`

**依赖：** 无

**代码量：** ~1100 LOC

### 不可篡改审计

```go
type AuditChain struct {
    entries []AuditEntry
    // ...
}
type AuditEntry struct {
    ID        string
    PrevHash  string
    Data      string
    Timestamp time.Time
    Signature string
    // ...
}
```

审计链采用区块链式数据结构：

- **SHA-256 哈希指针**：每个 `AuditEntry` 包含前一个条目的哈希值（`PrevHash`），形成不可篡改的链表
- **HMAC 签名**：可选签名验证，使用密钥对每个条目进行签名，防止伪造
- **查询能力**：支持按时间范围、ID 和事件类型查询审计历史

审计链的不可篡改性保证了 Agent 执行过程中的每一个决策、每一次 LLM 调用和每一次工具执行都可追溯、可验证。

---

## 七、approval — HITL 审批

**模块路径：** `github.com/inferglow/approval`

**依赖：** 无

**代码量：** ~700 LOC

### 核心类型

```go
type Manager struct {
    policies map[string]AccessPolicy
    // ...
}
type ApprovalRequest struct {
    ID       string
    Action   string
    Input    map[string]any
    Status   ApprovalStatus
    // ...
}
type AccessPolicy struct {
    AllowList []string
    DenyList  []string
    // ...
}
```

### 4 种审批处理器

| 处理器 | 行为 | 适用场景 |
|--------|------|---------|
| `AutoAllowHandler` | 自动允许所有请求 | 开发调试 |
| `AutoApproveHandler` | 自动批准（记录日志） | 可审计但无需人工介入 |
| `FailClosedHandler` | 默认拒绝所有请求 | 生产环境高安全要求 |
| `InputTimeoutFailHandler` | 超时未响应则拒绝 | 需要人工审批但有超时兜底 |

`Manager` 通过 `AccessPolicy` 定义操作的白名单和黑名单，结合审批处理器实现灵活的安全门控。审批结果会通过 `AuditChain` 记录，形成完整的审批链路。

---

## 八、rag — RAG 管道

**模块路径：** `github.com/inferglow/rag`

**依赖：** 无

**代码量：** ~1500 LOC

### 管道架构

```go
type Pipeline struct {
    Loader    Loader
    Splitter  Splitter
    Embedding EmbeddingRegistry
    Store     DocumentStore
    // ...
}
```

RAG 管道采用经典的 Loader → Splitter → Embedding → Store → Retriever 五阶段架构：

### 6 种 Loader

| Loader | 说明 |
|--------|------|
| `CSVLoader` | CSV 文件加载 |
| `JSONLoader` | JSON 文件加载 |
| `HTMLLoader` | HTML 页面加载 |
| `MarkdownLoader` | Markdown 文件加载 |
| `TextLoader` | 纯文本加载 |
| `LineLoader` | 逐行读取 |

### 3 种 Splitter

| Splitter | 分割策略 |
|---------|---------|
| `RecursiveCharacterSplitter` | 递归字符分割，保持语义完整性 |
| `MarkdownSplitter` | 按 Markdown 标题层级分割 |
| `ParagraphSplitter` | 按段落分割 |

### 3 种 Retriever

| Retriever | 检索策略 |
|----------|---------|
| `BM25Retriever` | 基于 BM25 算法的关键词检索 |
| `RecencyRetriever` | 基于时间近因的检索 |
| `FusionRetriever` | 多路召回融合，组合多种检索结果 |

`Embedding` 通过 Registry 模式注册不同提供商的 Embedding 模型，支持灵活的扩展。

---

## 九、rerank — 重排序

**模块路径：** `github.com/inferglow/rerank`

**依赖：** 无

**代码量：** ~500 LOC

### 核心接口

```go
type Reranker interface {
    Rerank(ctx context.Context, query string, docs []Document) ([]Document, error)
}
```

### 3 种后端

| 后端 | 说明 |
|------|------|
| Cohere | 调用 Cohere Rerank API 进行语义重排序 |
| LLM-based | 基于 LLM 对文档进行相关性打分和重排序 |
| Fallback | 降级策略，当主后端不可用时自动切换 |

`Reranker` 接口接受原始查询和待排序文档列表，返回按相关性降序排列的文档。这种设计使得 RAG 检索结果可以经过二次排序优化，提升最终输入给 LLM 的上下文质量。

---

## 十、observability — OpenTelemetry 集成

**模块路径：** `github.com/inferglow/observability`

**依赖：** 无

**代码量：** ~700 LOC

### 6 种 SpanKind

```go
const (
    SpanAgentRun    SpanKind = "agent.run"     // Agent 运行生命周期
    SpanLLMCall     SpanKind = "llm.call"      // LLM 调用
    SpanToolCall    SpanKind = "tool.call"     // 工具调用
    SpanFlowExecute SpanKind = "flow.execute"  // Flow 执行
    SpanStepExecute SpanKind = "step.execute"  // Step 执行
    SpanInternal    SpanKind = "internal"      // 内部操作
)
```

| SpanKind | 覆盖范围 | 说明 |
|----------|---------|------|
| `SpanAgentRun` | Agent 运行 | 一次完整的 Agent 执行 |
| `SpanLLMCall` | LLM 调用 | 单次 LLM 请求-响应周期 |
| `SpanToolCall` | 工具调用 | 单个工具的调用执行 |
| `SpanFlowExecute` | Flow 执行 | 整个 Flow 的编排执行 |
| `SpanStepExecute` | Step 执行 | Flow 中单个 Step 的执行 |
| `SpanInternal` | 内部操作 | 缓存、压缩等内部操作 |

observability 模块将 OpenTelemetry 的 Span 概念与 Inferglow 的领域模型对齐，通过预定义的 6 种 SpanKind 覆盖全栈可观测性需求。`CallbacksTracer` 桥接器将 `AgentCallbacks` 自动映射到 OTel Span，实现零侵入式可观测性集成。

---

## 十一、workspace — 工作区

**模块路径：** `github.com/inferglow/workspace`

**依赖：** 无

**代码量：** ~1200 LOC

### 核心类型

```go
type Workspace struct {
    config Config
    // ...
}
```

### 安全防护机制

| 功能 | 说明 |
|------|------|
| **SafePath 三重防护** | 路径穿越防护：检查路径合法性、防止符号链接逃逸、防止绝对路径注入 |
| **ReadOnly 模式** | 只读保护：工作区可配置为只读，防止写操作 |
| **文件大小限制** | 防止大文件：单文件大小上限可配置，防止恶意大文件 |
| **文件数量限制** | 防止过多文件：工作区文件总数上限可配置，防止 inode 耗尽 |

`Workspace` 为代码执行和文件操作提供安全的沙箱化文件系统视图。所有文件操作都经过 SafePath 路径校验，确保操作不会逃逸到工作区之外。

---

## 十二、resource — 资源管理

**模块路径：** `github.com/inferglow/resource`

**依赖：** 无

**代码量：** ~750 LOC

### 核心类型

```go
type Provider interface {
    Name() string
    CreateHandle(ctx context.Context, requirement Requirement) (Handle, error)
}
type Manager struct {
    providers map[string]Provider
    // ...
}
```

`Manager` 采用注册模式管理多种资源 Provider：

- **Provider 接口**：每种资源类型实现一个 Provider，负责创建资源句柄
- **Manager**：管理所有注册的 Provider，支持按名称查找和按需求匹配
- **Requirement**：资源需求描述，定义所需资源的规格和约束

resource 模块提供统一的资源生命周期管理，包括资源的注册、分配、回收和监控，为上层模块提供计算资源、存储资源等基础资源的抽象管理。

---

## 基础层总结

| 模块 | 路径 | LOC | 核心职责 | 关键设计模式 |
|------|------|-----|---------|------------|
| model | `github.com/inferglow/model` | ~8000 | LLM Provider 统一抽象 | 适配器模式、组合模式 |
| schema | `github.com/inferglow/schema` | ~2800 | Contract-First Schema 引擎 | 泛型推导、契约优先 |
| session | `github.com/inferglow/session` | ~1800 | 对话记忆管理 | 双列表架构、策略模式 |
| sandbox | `github.com/inferglow/sandbox` | ~6300 | 沙箱执行框架 | Provider-Handle 模式 |
| context | `github.com/inferglow/context` | ~6300 | 上下文管理引擎 | 三区压缩架构 |
| audit | `github.com/inferglow/audit` | ~1100 | 链表式审计链 | 区块链式哈希指针 |
| approval | `github.com/inferglow/approval` | ~700 | HITL 审批 | 策略模式、门控模式 |
| rag | `github.com/inferglow/rag` | ~1500 | RAG 管道 | 管道架构、Registry 模式 |
| rerank | `github.com/inferglow/rerank` | ~500 | 重排序 | 策略模式、降级模式 |
| observability | `github.com/inferglow/observability` | ~700 | OpenTelemetry 集成 | Span 领域映射 |
| workspace | `github.com/inferglow/workspace` | ~1200 | 工作区 | SafePath 三重防护 |
| resource | `github.com/inferglow/resource` | ~750 | 资源管理 | 注册模式、工厂模式 |

12 个基础模块之间零内部循环依赖，每个模块均可独立编译、测试和复用。这种极致的模块化设计确保了 Inferglow 框架的灵活性和可扩展性，上层模块可以按需组合这些基础零件，构建复杂的 AI Agent 应用。