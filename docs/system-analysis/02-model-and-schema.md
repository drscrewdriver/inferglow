# 02 · model 与 schema 模块

## 一、model 模块

### 1.1 职责

`model` 模块（`github.com/inferglow/model`）是整个框架的最底层，提供 LLM Provider 的统一抽象。它屏蔽 OpenAI / Anthropic / Ollama 三类供应商的 API 差异，向上层暴露统一的 `ModelRequester` 接口。该模块是**零内部依赖**（仅 stdlib + `yaml.v3`）。

### 1.2 核心类型

#### 请求/响应类型（[model.go](../../model/model.go)）

```go
// 统一模型请求
type ModelRequest struct {
    System        string             // system prompt
    Developer     string             // developer 消息（OpenAI o1 系列）
    Instruct      string             // 额外指令
    Input         string             // 当前用户输入
    OutputFormat  string
    ChatHistory   []ChatMessage      // 对话历史（来自 session）
    Info          map[string]any
    Tools         []ToolDefinition   // 工具定义（来自 ActionRegistry）
    Actions       []ActionResult     // 上一轮 Action 执行结果
    Examples      []Example
    Attachment    []Attachment
    Output        *OutputSchema      // 结构化输出约束
    EnsureAll     bool
    Options       map[string]any     // provider 特定选项（如 force_json）
    Model         string
    Temperature   float64
    TemperatureSet bool              // 是否显式设置 Temperature
    ToolChoice    any               // "auto"/"none"/"required" 或结构化对象
}

// 非流式响应
type ModelResponse struct {
    Content            string
    Reasoning          string        // 推理内容（o1/DeepSeek-R1）
    Tools              []ToolCall
    Usage              UsageInfo
    Meta               map[string]any
    ReasoningTruncated bool
    ReasoningTokens    int
}

// 流式块
type StreamChunk struct {
    Delta     string              // 增量文本
    Reasoning string              // 增量推理
    Tools     []ToolCall
    IsDone    bool
    Usage     *UsageInfo          // 最后一块携带
    Meta      map[string]any
}

// 发给 Provider 的中间数据
type RequestData struct {
    Model       string
    Messages    []ChatMessage
    Tools       []ToolDefinition
    Temperature float64
    MaxTokens   int
    Options     map[string]any
    ToolChoice  any
    Output      *OutputSchema      // 启用 JSON mode
}
```

#### 核心接口（[model.go](../../model/model.go) L129-L138）

```go
type ModelRequester interface {
    Name() string
    GenerateRequestData(ctx context.Context, req *ModelRequest) (*RequestData, error)
    RequestModel(ctx context.Context, data *RequestData) (<-chan *StreamChunk, error)
    BroadcastResponse(ctx context.Context, stream <-chan *StreamChunk) (<-chan *ResultEvent, error)
}
```

`ModelRequester` 是上层（`orchestrator/agent.Engine`）唯一直接依赖的 model 接口。四种 Provider 实现该接口：
- `OpenAICompatibleProvider`（[openai.go](../../model/openai.go)）— 支持 OpenAI 及兼容 API（DeepSeek、Qwen、Moonshot 等）
- `AnthropicCompatibleProvider`（[anthropic.go](../../model/anthropic.go)）— Claude 系列
- `OllamaProvider`（[ollama.go](../../model/ollama.go)）— 本地 Ollama
- `OpenAIResponsesProvider`（[openai_responses.go](../../model/openai_responses.go)）— OpenAI Responses API（`/responses` 端点，`response.output_text.delta` 事件）

#### Provider 注册表（[provider.go](../../model/provider.go)）

```go
type ModelRequesterRegistry struct {
    mu        sync.RWMutex
    providers map[string]ModelRequester
}

func NewModelRequesterRegistry() *ModelRequesterRegistry
func (r *ModelRequesterRegistry) Register(provider ModelRequester) error
func (r *ModelRequesterRegistry) Get(name string) (ModelRequester, error)
func (r *ModelRequesterRegistry) List() []string
```

### 1.3 关键调用链

#### 链 A：发起一次流式模型请求

```
engine.executeLoop()
    │
    ├─[1] modelReq.GenerateRequestData(ctx, req)
    │       │  将 ModelRequest 转换为 RequestData
    │       │  - 拼接 System/Developer/Instruct 到 messages
    │       │  - 注入 Tools 到 request body
    │       │  - 处理 ToolChoice / Output(JSON mode)
    │       │  - 当 force_json:true + Output.Properties 非空 → response_format: json_schema (L1/L2)
    │       │  - 否则 → response_format: json_object (降级)
    │       └─▶ *RequestData
    │
    ├─[2] modelReq.RequestModel(timeoutCtx, data)
    │       │  HTTP POST /chat/completions (stream=true)
    │       │  启动 goroutine 读取 SSE 流
    │       └─▶ <-chan *StreamChunk
    │
    └─[3] for chunk := range stream {
            content.WriteString(chunk.Delta)   // 增量拼接
          }
```

#### 链 B：ModelPool 故障转移

`ModelPool`（[pool.go](../../model/pool.go)）管理多个 Provider，支持故障转移：

```
ModelPool.Request(ctx, req)
    │
    ├── 按 strategy 选择 primary provider
    │     (fixed / random / round_robin / least_used)
    │
    ├── primary.Request(ctx, req)
    │     │
    │     ├── 成功 → 返回 Response
    │     │
    │     └── 401/403/429 → 触发 failover
    │           │
    │           ├── failover.go: 切换 API Key 或切换 Provider
    │           └── 重试下一个 provider
    │
    └── 全部失败 → 返回最后一个错误
```

#### 链 C：速率限制包装

`ratelimit_wrap.go` 提供装饰器，将 `security/ratelimit` 的令牌桶套在 Provider 外层：

```
RateLimitedProvider.Request(ctx, req)
    │
    ├── bucket.Wait(ctx)          // 令牌桶等待
    │     (来自 security/ratelimit)
    │
    └── inner.Request(ctx, req)   // 实际 Provider 调用
```

### 1.4 扩展点

| 扩展点 | 实现方式 | 用途 |
|--------|---------|------|
| `ModelRequester` 接口 | 实现四个方法 | 接入新的 LLM 供应商 |
| `Provider` 配置 | `Config` 结构体 | 自定义 BaseURL/APIKey/Model |
| `ModelPool` 策略 | `PoolConfig.Strategy` | 切换负载均衡策略 |

### 1.5 其他重要文件

| 文件 | 内容 |
|------|------|
| [config.go](../../model/config.go) | `Config` 结构体、YAML 加载、环境变量替换（`${ENV.VAR}`）、`ProviderConfig`（含 `FullURL`/`ContentMap` 字段）、`DEFAULT_SETTINGS`（含 `openai_responses` 默认配置） |
| [url_resolver.go](../../model/url_resolver.go) | `ResolveURL(baseURL, defaultPath, fullURL)` — `full_url` 非空时完全覆盖拼接 |
| [content_mapping.go](../../model/content_mapping.go) | `ContentMapping` 类型、`ExtractByPath`（点号/斜杠 + 数组索引路径提取）、`DefaultOpenAIContentMapping` |
| [think_normalizer.go](../../model/think_normalizer.go) | `LeadingThinkNormalizer` 三态状态机（流式 `<think>` 分离）、`normalizeThinkingTags`/`hasThinkingTags`（大小写不敏感正则） |
| [openai_responses.go](../../model/openai_responses.go) | `OpenAIResponsesProvider`：Responses API（`/responses` 端点、`response.output_text.delta` / `response.reasoning_summary_text.delta` 事件） |
| [provider_factory.go](../../model/provider_factory.go) | 20 家 Provider 工厂函数（含 `NewOpenAIResponsesProviderFromConfig`），全部传递 `FullURL`/`ContentMapping` |
| [attempt.go](../../model/attempt.go) | `Attempt` 单次请求尝试状态，支持重试 |
| [failover.go](../../model/failover.go) | 故障转移逻辑 |
| [stream_reader.go](../../model/stream_reader.go) | `StreamReader` 流读取器 |
| [streaming_json.go](../../model/streaming_json.go) | 流式 JSON 增量解析（处理不完整 JSON 片段） |
| [validator.go](../../model/validator.go) | `OutputValidator`：L4 后置校验（3 字段硬编码检查 + JSON 结构校验 + `checkFieldType` 类型检查 + `ValidateAndRetryWithFetch` 重试） |
| [output_format.go](../../model/output_format.go) | `BuildJSONSchemaFromOutput`（L1/L2 json_schema 生成）、`forceJSONObject`（降级开关检测） |
| [router.go](../../model/router.go) | 多模型路由 |
| [cache_capability.go](../../model/cache_capability.go) | Prompt 缓存能力（Anthropic prompt caching） |
| [cache_stable.go](../../model/cache_stable.go) | 稳定序列化（`MarshalStable`，用于前缀缓存哈希） |

---

## 二、schema 模块

### 2.1 职责

`schema` 模块（`github.com/inferglow/schema`）实现**契约优先**（Contract-First）的输出校验。它通过 Go 泛型 + 反射从 struct 推导出 `OutputSchema`，再转换为 JSON Schema 注入 LLM 请求，最后用 `ContractEngine` 校验 LLM 输出是否符合契约。不依赖 `model` 模块（schema 完全独立，仅依赖 yaml.v3）。

### 2.2 核心类型

#### Schema 定义（[schema.go](../../schema/schema.go)）

```go
type DataType string
const (
    TypeString   DataType = "str"
    TypeInt      DataType = "int"
    TypeFloat    DataType = "float"
    TypeBool     DataType = "bool"
    TypeDict     DataType = "dict"
    TypeList     DataType = "list"
    TypeModel    DataType = "model"
    TypeOptional DataType = "optional"
)

type EnsurePolicy string
const (
    EnsurePresence EnsurePolicy = "presence"   // 字段必须存在
    EnsureNotNull  EnsurePolicy = "not_null"   // 字段值非 null
)

// 单个字段定义
type FieldDef struct {
    Type           DataType
    Description    string
    Ensure         EnsurePolicy
    Required       bool
    RequiredFields []string
    Children       map[string]*FieldDef  // 嵌套对象
    ItemDef        *FieldDef             // 数组元素
    OneOf          []*FieldDef           // JSON Schema oneOf
    AnyOf          []*FieldDef           // JSON Schema anyOf
}

// 输出契约
type OutputSchema struct {
    Format    OutputFormat
    EnsureAll bool
    Fields    map[string]*FieldDef
}
```

#### 泛型推导（[schema.go](../../schema/schema.go) L110-L112）

```go
// DefineOutput 从 Go struct 类型 T 推导 OutputSchema
func DefineOutput[T any]() *OutputSchema
```

推导规则：
- `json` tag 作为字段名（无 tag 的字段跳过）
- `description` tag 作为字段描述
- `json:"...,omitempty"` → `Required=false`，否则 `Required=true`
- 嵌套 struct 递归推导为 `TypeDict` + `Children`
- slice/array 推导为 `TypeList`，元素为 struct 时写入 `ItemDef.Children`

#### ContractEngine（[engine.go](../../schema/engine.go)）

```go
type ContractEngine struct {
    InputSchema  map[string]any
    ResultSchema map[string]any
    EnsureKeys   map[string]EnsurePolicy
    EnsureAll    bool
}

// 带重试的校验（指数退避 1s/2s/4s...）
func (ce *ContractEngine) ValidateWithRetry(
    ctx context.Context,
    result any,
    maxRetries int,
    retryFn func() (any, error),
) (any, error)

func (ce *ContractEngine) ValidateInput(data any) error
func (ce *ContractEngine) ValidateResult(data any) error
func (ce *ContractEngine) GenerateJSONSchema() map[string]any
```

### 2.3 关键调用链

#### 链 A：从 struct 推导 JSON Schema

```
用户定义:
    type WeatherOutput struct {
        City    string  `json:"city" description:"城市名"`
        Temp    float64 `json:"temp" description:"温度"`
    }

DefineOutput[WeatherOutput]()
    │
    ├── reflect.TypeOf((*WeatherOutput)(nil)).Elem()
    │
    ├── 遍历字段:
    │     City  → FieldDef{Type:TypeString, Required:true, Description:"城市名"}
    │     Temp  → FieldDef{Type:TypeFloat, Required:true, Description:"温度"}
    │
    └─▶ &OutputSchema{Fields: {"city":..., "temp":...}}

OutputSchema.ToJSONSchema()  (jsonschema.go)
    └─▶ map[string]any{
            "type": "object",
            "properties": {
                "city": {"type":"string","description":"城市名"},
                "temp": {"type":"number","description":"温度"},
            },
            "required": ["city","temp"],
        }
```

#### 链 B：ContractEngine 校验 + 重试

```
ce := &ContractEngine{
    EnsureKeys: {"city": EnsurePresence, "temp": EnsureNotNull},
    EnsureAll:  true,
}

ce.ValidateWithRetry(ctx, llmOutput, 3, retryFn)
    │
    ├── ValidateResult(llmOutput)
    │     │
    │     ├── 检查是否 map[string]any
    │     ├── ensurePathExists(dict, "city")    // presence
    │     └── checkNotNull(dict, "temp")        // not_null
    │
    ├── 通过 → 返回 result
    │
    └── 失败 → 重试 (1s, 2s, 4s 退避)
          ├── retryFn()  → 重新请求 LLM
          └── ValidateResult(newResult)
              └── 最多 maxRetries 次
```

### 2.4 路径校验语义

`ContractEngine` 支持点分路径与通配符（[path.go](../../schema/path.go)）：

| 路径表达式 | 语义 |
|-----------|------|
| `city` | 顶层字段 `city` 存在 |
| `address.city` | 嵌套字段 `address.city` 存在 |
| `items[*].name` | 数组所有元素的 `name` 字段存在 |

通配符 `[*]` 要求**所有**数组元素都满足剩余路径（空数组视为不满足）。

### 2.5 其他重要文件

| 文件 | 内容 |
|------|------|
| [derive.go](../../schema/derive.go) | `DefineOutputFromType` 反射推导实现 |
| [jsonschema.go](../../schema/jsonschema.go) | `OutputSchema` → JSON Schema 转换 |
| [extractor.go](../../schema/extractor.go) | 从 LLM 输出提取 JSON（处理 markdown 代码块、噪声文本） |
| [path.go](../../schema/path.go) | `ParsePath` 路径解析（支持 `[*]` 通配符） |
| [blueprint.go](../../schema/blueprint.go) | `Blueprint` 数据结构模板 |
| [serialize.go](../../schema/serialize.go) | Schema 序列化/反序列化 |
| [type_adapter.go](../../schema/type_adapter.go) | 类型适配器 |
| [version.go](../../schema/version.go) | Schema 版本管理 |

### 2.6 model 与 schema 的关系

```
用户定义 struct T
    │
    ▼
schema.DefineOutput[T]()  ──▶ schema.OutputSchema
    │
    ▼
OutputSchema.ToJSONSchema()  ──▶ map[string]any (JSON Schema)
    │
    ▼
model.ModelRequest.Output = &model.OutputSchema{...}  (轻量 schema)
    │
    ▼
Provider.GenerateRequestData()  ──▶ HTTP body 含 response_format
    │   ├─ force_json:true + Properties 非空 → {"type":"json_schema","json_schema":{...}}  (L1/L2 硬约束)
    │   └─ 否则 → {"type":"json_object"}  (降级模式)
    │
    ▼
LLM 返回 JSON
    │
    ▼
model.OutputValidator.validate(response)  ──▶ L4 后置校验
    │   ├─ 解析 Content 为 JSON
    │   ├─ 校验 Required 字段存在性
    │   └─ 校验字段类型 (checkFieldType: string/integer/number/boolean/object/array)
    │
    ▼ (flow 场景)
schema.ContractEngine.ValidateResult(stepOutput)  ──▶ Flow Step.Schema 校验
```

`schema` 不依赖 `model`（schema 完全独立，仅依赖 yaml.v3；go.mod 中无 model 的 require）。两套 `OutputSchema` 类型并存：

| 类型 | 定义位置 | 字段 | 用途 |
|------|---------|------|------|
| `model.OutputSchema` | model/model.go | Type / Properties / Required | 轻量，用于 L1/L2 response_format 生成和 L4 后置校验 |
| `schema.OutputSchema` | schema/schema.go | Format / EnsureAll / Fields map[string]*FieldDef | 重量，用于 Flow Step.Schema 迭代校验 |

在 `orchestrator` 中，`Engine` 直接用 `model.OutputSchema` 构造请求（见 [engine.go](../../orchestrator/agent/engine.go) L235-L275）。`WithOutputSchema` RunOption 启用 L3 prompt 兜底 + L4 后置校验。Flow 的 `Step.Schema` 使用 `schema.OutputSchema`，由 `validateStepOutput` 在每步执行后独立校验。
