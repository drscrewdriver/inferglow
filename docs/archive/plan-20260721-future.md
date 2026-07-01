# 远期能力规划（2026-Q3+）

> 生成日期：2026-07-21
> 基于 `inferglow-maturity-v2.md` 成熟度分析的改进项细化

---

## 一、沙箱外的安全能力规划

### 现状（已有）

| 能力 | 模块 | 成熟度 |
|------|------|--------|
| 链式哈希审计日志 | `audit/` — tamper-evident + HMAC 签名 | ⭐⭐⭐⭐ |
| 7 种沙箱后端 | `sandbox/` — Docker/gVisor/Landlock/Seatbelt/WindowsAppContainer/TrustedLocal/E2B | ⭐⭐⭐⭐⭐ |
| 审批门控 | `sandbox/approval.go` — InputTimeoutFail/FailClosed/AutoApprove/AutoAllow | ⭐⭐⭐⭐ |
| 资源限制 | `sandbox/policy.go` — CPU/Memory/Disk/NPROC/Network/Filesystem | ⭐⭐⭐⭐ |
| LoopGuard | `orchestrator/agent/loop_guard.go` — 防止循环调用 | ⭐⭐⭐ |
| Panic 恢复 | `orchestrator/actionruntime/dispatcher.go` — goroutine recover | ⭐⭐⭐ |
| ActionSpec 安全规格 | `action/spec.go` — SideEffectLevel/ApprovalRequired/SandboxRequired | ⭐⭐⭐ |

### 新增安全能力矩阵（按优先级）

```
security/                              ← 新增安全模块
├── prompt_injection/                  ← P0：Prompt 注入防护
│   ├── detector.go                    规则检测（已知注入模式）
│   ├── classifier.go                  轻量分类模型（可选）
│   └── config.go                      检测等级配置（off/strict/relaxed）
├── pii/                               ← P0：PII 脱敏
│   ├── mask.go                        正则匹配脱敏（邮箱/电话/身份证/卡号）
│   ├── ner.go                         命名实体识别（可选，依赖第三方）
│   └── patterns.go                    可配置敏感模式列表
├── rbac/                              ← P0：RBAC 访问控制
│   ├── policy.go                      角色定义（viewer/editor/admin/custom）
│   ├── matrix.go                      角色 → Action 权限矩阵
│   ├── context.go                     从 context 提取用户角色
│   └── middleware.go                  与 SandboxApprovalService 整合的拦截器
├── ratelimit/                         ← P0：API 速率控制
│   ├── bucket.go                      TokenBucket 实现
│   ├── provider_limiter.go            per-provider 独立限额
│   └── policy.go                      硬限制（直接拒绝）/ 软限制（排队/退避）
├── cost/                              ← P1：成本追踪
│   ├── pricing.go                     模型定价表（每模型 per-token 价格）
│   ├── tracker.go                     基于 UsageInfo 计算成本
│   ├── budget.go                      会话级/项目级预算 + 阈值告警
│   └── audit_ext.go                   扩展 audit.AuditEntry 记录 token 和成本
├── input/                             ← P1：输入验证
│   ├── validator.go                   输入长度/类型/特殊字符校验
│   ├── schema_validate.go             复用 schema/ 的泛型契约做输入校验
│   └── sanitizer.go                   过滤 null byte / 控制字符
└── code_injection/                    ← P1：代码注入防护（沙箱外）
    ├── scanner.go                     静态扫描危险模式（rm -rf / wget | bash）
    └── whitelist.go                   白名单命令列表
```

### 各能力详细说明

#### P0：Prompt 注入防护

**对标**：LangSmith `prompt_injection_detection`（基于分类模型）

**实现方案（三级）**：

```
L1 规则层（必做）：
  - 检测已知注入关键词："Ignore previous"、"You are now"、"System:"
  - 检测指令注入模式（prompt 中混入非用户自然语言）
  - 复杂度：低，纯正则 + 关键词匹配

L2 轻量分类（选做）：
  - 使用小型分类模型（如 <10MB 的 BERT）对输入/输出做注入评分
  - 阈值可配置（strict = 0.7, relaxed = 0.9）

L3 LLM 作为 Judge（远期）：
  - 用另一个 LLM 判断输入是否包含注入意图
  - 成本高，仅在安全要求极高时开启
```

**输入侧**：用户消息进入 Session 前检测
**输出侧**：LLM 返回前检测（防止 LLM 被污染后输出恶意指令）

#### P0：PII 脱敏

**对标**：Microsoft Presidio（业界标准）

**实现方案**：

```go
// PII Patterns（可配置）
type PIIType string

const (
    Email       PIIType = "email"
    Phone       PIIType = "phone"
    IDCard      PIIType = "id_card"
    CreditCard  PIIType = "credit_card"
    BankAccount PIIType = "bank_account"
    IPAddr      PIIType = "ip_address"
    Custom      PIIType = "custom"
)

// MaskConfig 脱敏配置
type MaskConfig struct {
    Patterns    map[PIIType]string    // PII 类型 → 正则
    MaskChar    string                // 替换字符（默认 "***"）
    KeepPrefix  int                   // 保留前 N 个字符
    ApplyOn     MaskOn                // 输入时/输出时/双向
}

type MaskOn int
const (
    MaskOnInput  MaskOn = 1 << iota  // 输入时
    MaskOnOutput                     // 输出时
)
```

**位置**：
- 输入脱敏：`Session.AddMessage()` 前（`session/session.go`）
- 输出脱敏：`Agent.Run()` 返回前（`orchestrator/agent/engine.go`）

#### P0：RBAC 访问控制

**对标**：LangChain 无内置 → 自主设计

**实现方案**：

```go
type Role string
const (
    RoleViewer   Role = "viewer"   // 仅 read
    RoleEditor   Role = "editor"   // read + write
    RoleAdmin    Role = "admin"    // full access
    RoleCustom   Role = "custom"   // 自定义权限
)

// PermissionMatrix 角色 → Action 权限矩阵
type PermissionMatrix struct {
    // 每个 Action 的权限位
    permissions map[string]map[Role]bool  // action_name → role → allowed
}

// 与现有 ActionSpec 整合
func (pm *PermissionMatrix) Allow(role Role, actionName string) bool {
    roles, ok := pm.permissions[actionName]
    if !ok {
        return false  // 默认拒绝
    }
    return roles[role]
}

// RBACMiddleware 拦截器，在 ActionDispatcher.Execute 前检查
type RBACMiddleware struct {
    matrix *PermissionMatrix
    role   func(ctx context.Context) Role  // 从 context 提取角色
}
```

**与现有体系整合**：
- `sandbox/approval.go` 的 `ApprovalService` 已有审批门控概念，RBAC 是"谁可以请求执行"
- `sandbox/approval.go` 审批的是"执行是否被批准"（安全门控）
- 两者配合：RBAC 决定谁能提交请求，ApprovalService 决定是否允许执行

#### P0：Rate Limit

**对标**：LangChain 基础支持（`AsyncChatOllama.requests_per_minute`），无全局管理器

**实现方案**：

```go
// ProviderRateLimit 每个 Provider 的限额
type ProviderRateLimit struct {
    MaxRequestsPerMinute int           // RPM
    MaxTokensPerMinute   int           // TPM
    MaxTokensPerDay      int           // 日限额
    Bucket               *TokenBucket  // TokenBucket 实现
}

// RateLimiter 全局速率控制器
type RateLimiter struct {
    providers map[string]*ProviderRateLimit  // provider_name → limit
    fallback  *ProviderRateLimit             // 默认限额（未知 Provider）
}

// 硬限制 vs 软限制
type LimitMode int
const (
    LimitHard LimitMode = iota  // 超出直接拒绝
    LimitSoft                   // 超出排队等待
)
```

**与现有体系整合**：
- 在 `model/provider_factory.go` 创建 Provider 时绑定 RateLimit
- 在 `Engine.executeLoop` 调用 `RequestModel` 前检查
- 与 `model/attempt.go` 的 retry 机制互补（RateLimit 防超限，Retry 处理瞬态失败）

#### P1：成本追踪

**对标**：LangChain `TokenCountingCallbackHandler` + LangSmith 自动计算

**实现方案**：

```go
// ModelPricing 模型定价表（per 1M tokens）
type ModelPricing struct {
    InputCostPer1M    float64  // 输入每百万 token 成本
    OutputCostPer1M   float64  // 输出每百万 token 成本
}

// CostTracker 成本追踪器
type CostTracker struct {
    pricing  map[string]ModelPricing  // model_name → pricing
    sessions map[string]float64       // session_id → 累计成本
}

func (t *CostTracker) RecordUsage(model string, tokens model.UsageInfo) float64 {
    p, ok := t.pricing[model]
    if !ok { return 0 }
    cost := float64(tokens.PromptTokens)/1_000_000*p.InputCostPer1M +
            float64(tokens.CompletionTokens)/1_000_000*p.OutputCostPer1M
    return cost
}
```

**与现有体系整合**：
- 扩展 `audit.AuditEntry`，新增 `TokenCost float64` 字段
- 在 `Engine.executeLoop` 收集 `StreamChunk.Usage` 时记录成本
- 支持预算告警：超过阈值时回调通知

#### P1：输入验证

**对标**：LangChain 通过 `ChatPromptTemplate` InputSchema，Agently Contract-First Schema

**实现方案**：

```go
type InputValidator struct {
    MaxChars   int    // 最大输入字符数
    MaxTokens  int    // 最大输入 token 数
    AllowedTypes []string  // 允许的输入类型（string/structured/file）
    BlockPatterns []string  // 禁止的模式（null byte、控制字符等）
}
```

**复用**：已有的 `schema/` 模块泛型契约可双向使用（不仅校验 LLM 输出，也可校验用户输入）。

#### P1：代码注入防护（沙箱外）

**对标**：AutoGen `code_execution_config` 限制 + Agently 沙箱

**实现方案**：

```go
// 在代码送入沙箱前做静态扫描
type CodeScanner struct {
    dangerousPatterns []*regexp.Regexp  // rm -rf、wget | bash 等
    whitelist         []string          // 允许的命令白名单
}
```

---

## 二、模型供应商支持分析

### Agently（Python）支持的模型供应商

根据成熟度文档分析，Agently 支持 **~8 家**供应商，覆盖情况：

| 供应商 | 协议 | Agently 方式 |
|--------|------|-------------|
| OpenAI | OpenAI Chat Completions | 内置 Provider |
| Anthropic | Anthropic Messages API | 内置 Provider |
| DeepSeek | OpenAI 兼容 | 内置 Provider |
| Qwen/通义千问 | OpenAI 兼容 (DashScope) | 内置 Provider |
| Ollama | Ollama /api/chat | 内置 Provider |
| Google Gemini | Google AI Studio | 内置 Provider |
| Azure OpenAI | OpenAI 兼容 (Azure 端点) | 内置 Provider |
| 任意 OpenAI 兼容端点 | OpenAI 兼容 | 通用配置 |

**Agently 的统一接口**：

```python
# Agently 模型层统一抽象
class ModelProvider:
    def request(self, messages, tools=None, output_schema=None):
        """所有 Provider 统一入口"""
    
    def stream(self, messages, tools=None, output_schema=None):
        """流式输出统一入口"""
```

### InferGlow 与 Agently 的差异（2026-07-22 更新）

| 维度 | InferGlow (Go) | Agently (Python) |
|------|---------------|------------------|
| 供应商数 | **20 个 Factory 函数，覆盖 21 家 Provider** | ~8 家独立 Provider 实现 |
| 统一接口 | `ModelRequester` 接口（完善） | `ModelProvider` 接口（完善） |
| 协议适配 | `OpenAICompatibleProvider` + `AnthropicCompatibleProvider` + `RoleMapping` | 类似 |
| 配置系统 | `ConfigProvider` + `CompositeConfigProvider` | 类似 |
| 重试机制 | `AttemptRunner` 指数退避（完善） | 基础 |
| Prefix Cache | `CacheAwareProvider` + 7 家画像 | 无 |
| 结构化输出 | `OutputSchema` + Provider 各自处理 | 类似 |
| 跨 Provider 降级 | **缺失**（G1-08 ModelPool 计划中） | **缺失**（所有框架通病） |

### InferGlow Provider 架构现状（2026-07-22 更新）

```
OpenAICompatibleProvider  ← 17 家 (openai/deepseek/qwen/glm/kimi/stepfun/baidu/spark/sensenova/mimo/tencent/volcengine/zeroone/minimax/siliconflow/openrouter)
    ├── 统一 OpenAI 协议
    ├── RoleMapping (developer→system 自动映射) 对 deepseek/qwen/glm/kimi
    └── ReservedFields 白名单 (防 Options 覆盖)

AnthropicCompatibleProvider  ← 3 家 (anthropic/stepfun_anthropic/sensenova_anthropic/mimo_anthropic)
    ├── 独立 Messages API 实现
    └── system/max_tokens 差异处理

OllamaProvider  ← 1 家 (ollama)
    ├── /api/chat JSON-lines
    └── options 子对象嵌套

总计: 21 家 Provider 配置项, 20 个 Factory 函数
```

### 扩展建议

**P0：新增 5 家覆盖**

| 新增 Provider | 协议 | Base URL | 工作量 |
|--------------|------|----------|--------|
| Google Gemini | Google AI Studio | `https://generativelanguage.googleapis.com/v1beta` | 中 |
| Azure OpenAI | OpenAI 兼容 (Azure 端点) | `{resource}.openai.azure.com` | 小（复用 OpenAICompatible + API-Key header 差异） |
| OpenRouter | OpenAI 兼容 | `https://openrouter.ai/api/v1` | 小 |
| Mistral | OpenAI 兼容 | `https://api.mistral.ai/v1` | 小 |
| Bedrock (AWS) | AWS SDK | AWS SDK 集成 | 大 |

**P1：跨 Provider 路由/降级**

```go
// 路由策略（LLM 决定）
type RoutingPolicy int
const (
    RoutingCost      RoutingPolicy = iota  // 按成本最低
    RoutingLatency                         // 按延迟最低
    RoutingQuality                         // 按模型能力分级
    RoutingFallback                        // 故障时自动切换
)

// ProviderRouter 多供应商路由/降级
type ProviderRouter struct {
    providers   map[string]ModelRequester
    policy      RoutingPolicy
    fallbackChain []string  // 降级顺序
}
```

**关于"动态路由由 LLM 决定"**：这不是当前主要方向，但可考虑在 `Engine.executeLoop` 中增加 `ModelChoice` 字段，让 LLM 的 Decision 输出中包含模型选择建议，由路由层执行实际切换。

---

## 三、可观测性规划

### 现状

InferGlow 已有的 `audit/` 模块已经是**差异化优势**：

```
audit/
├── types.go         AuditEntry (prev_hash → hash 链式结构)
├── chain.go         AuditChain (append-only, tamper-evident)
├── sign.go          HMAC-SHA256 签名
├── verify.go        链式验证（检测篡改）
├── hash.go          SHA-256 哈希计算
├── export.go        JSON/CSV/Text 导出
└── query.go         查询过滤
```

但缺少行业标准集成和评估能力。

### 推荐方案：分层架构

```
observability/                       ← 新增模块
│
├── layer1: audit/                   ← 已有（合规级审计）
│   ├── chain.go                     链式哈希审计
│   ├── sign.go                      HMAC 签名
│   └── verify.go                    链式验证
│
├── layer2: otel/                    ← P0：OpenTelemetry 集成
│   ├── tracer.go                    OTel Tracer 封装
│   ├── agent_span.go                Agent.Run() Span
│   ├── llm_span.go                  LLM Call Span
│   ├── tool_span.go                 Tool Call Span
│   └── exporters.go                 Jaeger/Prometheus/Stackdriver 导出器
│
├── layer3: langfuse/                ← P1：Langfuse Go SDK 集成
│   ├── sdk.go                       Langfuse Go SDK 封装
│   ├── trace.go                     Trace 创建与关联
│   └── score.go                     评分（人工/自动）
│
├── layer4: eval/                    ← P1：评估框架
│   ├── engine.go                    评估引擎
│   ├── rule_eval.go                 规则评估（Regex/ExactMatch）
│   ├── golden_dataset.go            Golden Dataset 管理
│   └── llm_as_judge.go              LLM-as-Judge（可选）
│
└── layer5: abtest/                  ← P2：A/B 测试
    ├── runner.go                    测试执行器
    ├── metrics.go                   多维度指标收集
    └── analysis.go                  统计分析
```

### Layer 2：OpenTelemetry 集成（P0）

**为什么需要 OTel**：

| 维度 | audit/ | OTel |
|------|--------|------|
| 标准 | 自定义格式 | 工业标准（CNCF） |
| 可视化 | 自定义（JSON 文件） | Jaeger/Grafana/Prometheus |
| 跨服务追踪 | 不支持 | 原生支持 |
| 性能指标 | 无 | 原生 Counter/Histogram |
| 厂商锁定 | 低 | 低 |
| Go SDK | 无（自实现） | `go.opentelemetry.io/otel` 完善 |

**InferGlow 语义化 Span 定义**：

```go
// OTel 语义化 Span
type SpanKind int
const (
    SpanAgentRun SpanKind = iota
    SpanLLMCall
    SpanToolCall
    SpanFlowExecute
    SpanPause
    SpanResume
)

// 语义属性
const (
    AttrModelName      = "llm.model_name"
    AttrModelProvider  = "llm.provider_name"
    AttrToolName       = "tool.name"
    AttrSessionID      = "inferglow.session_id"
    AttrRunID          = "inferglow.run_id"
    AttrTokensPrompt   = "llm.usage.prompt_tokens"
    AttrTokensCompletion = "llm.usage.completion_tokens"
)
```

### Layer 3：Langfuse 集成（P1）

**为什么选 Langfuse 而非 LangSmith**：

| 维度 | LangSmith | Langfuse |
|------|-----------|----------|
| 开源 | 否（SaaS） | 是（Apache 2.0） |
| Go SDK | 无 | **有**（`github.com/langfuse/langfuse-go`） |
| 框架绑定 | LangChain 优先 | 框架无关 |
| 自托管 | 商业版 | 完全支持 |
| 价格 | 付费 | 免费 |
| LLM 语义 | 原生 | 原生 |

**集成方案**：利用 Langfuse Go SDK 将 `audit/` 的 `AuditEntry` 映射为 Langfuse Trace。

### Layer 4：评估框架（P1）

```go
// EvaluationEngine 评估引擎
type EvaluationEngine struct {
    evaluators []Evaluator
}

type Evaluator interface {
    Name() string
    Evaluate(input, expected, actual any) (*Score, error)
}

// 内置评估器
type RuleEvaluator struct { /* Regex/ExactMatch */ }
type GoldenDatasetEvaluator struct { /* 批量测试 */ }
type LLMAsJudgeEvaluator struct { /* LLM 打分 */ }
```

### Layer 5：A/B 测试（P2）

```go
// ABTestRunner A/B 测试执行器
type ABTestRunner struct {
    datasets    []TestItem           // 固定数据集
    configs     []AgentConfig        // A/B 配置组
    evaluator   EvaluationEngine     // 评估器
}

type TestResult struct {
    ConfigName   string
    Metrics      map[string]float64   // 准确率/延迟/成本/成功率
    PValue       float64              // 统计显著性
}
```

---

## 四、MCP 协议支持与移植计划

### 现状：已完成 80%

InferGlow 的 MCP 实现**已相当完善**，基于 JSON-RPC 2.0 自研实现（无第三方依赖）：

| 文件 | 内容 | 状态 |
|------|------|------|
| `action/mcp/types.go` | Tool/Content/ServerInfo 数据类型 | ✅ 完成 |
| `action/mcp/client.go` | JSON-RPC 2.0 客户端（337 行） | ✅ 完成 |
| `action/mcp/transport.go` | Transport 接口抽象 | ✅ 完成 |
| `action/mcp/transport_stdio.go` | stdio 传输实现（167 行） | ✅ 完成 |
| `action/mcp/discovery.go` | 工具发现（tools/list） | ✅ 完成 |
| `action/mcp/config.go` | 工厂函数 | ✅ 完成 |
| `action/mcp/client_test.go` | 客户端测试（394 行） | ✅ 完成 |
| `action/mcp/transport_stdio_test.go` | stdio 集成测试（172 行） | ✅ 完成 |
| `action/executor_mcp.go` | MCPExecutor（194 行） | ✅ 完成 |
| `action/executor_mcp_test.go` | Executor 测试（271 行） | ✅ 完成 |

**协议版本**：`2024-11-05`（符合 MCP 官方 specification）

### 已完成功能

```
✅ JSON-RPC 2.0 基础
✅ initialize 握手
✅ tools/list（工具发现）
✅ tools/call（工具调用）
✅ stdio 传输（命令行子进程）
✅ Content 类型：text / image / resource_link
✅ 并发安全（pending map + readLoop goroutine）
✅ 错误分类（JSON-RPC 标准错误码）
✅ 无第三方依赖（仅 stdlib）
```

### 待完成功能

| 功能 | 优先级 | 工作量 | 说明 |
|------|--------|--------|------|
| **HTTP/SSE 传输** | P0 | 中 | `transport_http.go` — SSE 接收 + HTTP POST 发送 |
| **resources/get** | P1 | 小 | 资源读取 |
| **prompts/list + prompts/get** | P2 | 小 | Prompt 模板管理 |
| **samples** | P3 | 小 | 示例数据 |
| **根发现（roots）** | P2 | 中 | 文件系统根路径 |
| **分页（pagination）** | P1 | 小 | 大列表分页 |
| **资源订阅（subscriptions）** | P2 | 中 | SSE 流式资源更新 |
| **PICTEL 采样** | P3 | 小 | 大消息采样 |
| **官方 SDK 兼容** | P2 | 中 | 与 `modelcontextprotocol/sdk` 互通 |

### HTTP/SSE 传输实现方案

```go
// transport_http.go
type HTTPTransport struct {
    baseURL    string
    httpClient *http.Client
    reader     *sseReader  // SSE 流解析器
}

func (t *HTTPTransport) Start(ctx context.Context) error {
    // 建立 SSE 长连接，启动后台 reader goroutine
    // 等价于 stdio 的 readLoop
}

func (t *HTTPTransport) Send(ctx context.Context, msg []byte) error {
    // HTTP POST 发送 JSON-RPC 请求
    resp, err := t.httpClient.Post(t.baseURL, "application/json", bytes.NewReader(msg))
    // HTTP/SSE 模式下，请求-响应在同一 HTTP 连接上，不需要单独的 Recv
    return err
}

func (t *HTTPTransport) Recv(ctx context.Context) ([]byte, error) {
    // 从 SSE stream 读取 events
    // 解析 data: {...} 行
}

func (t *HTTPTransport) Stop(ctx context.Context) error {
    // 关闭 SSE 连接
}
```

### 移植决策

**结论：不需要移植，已自研完成核心功能。**

InferGlow 的 MCP 客户端实现已覆盖了 MCP 规范中最核心的 3 个方法：
- `initialize`（握手）
- `tools/list`（工具发现）
- `tools/call`（工具调用）

这与 Agently 的 MCPExecutor 完全等效。唯一缺的是 HTTP/SSE 传输层，这是网络层差异，不影响协议兼容性。

**建议**：优先补齐 HTTP/SSE 传输，即可支持 100% 的 MCP server（包括通过 HTTP 暴露的 server）。stdio 方式已可覆盖所有 CLI 工具。

### 与官方 SDK 的对比

| 维度 | InferGlow 自研 | @modelcontextprotocol/sdk (TS) |
|------|---------------|-------------------------------|
| 核心方法 | ✅ initialize/tools/list/tools/call | ✅ 全部 |
| 传输层 | stdio ✅ / HTTP/SSE ❌ | stdio ✅ / HTTP-SSE ✅ / Streamable HTTP ✅ |
| 资源管理 | ❌ | ✅ |
| 提示词管理 | ❌ | ✅ |
| 分页 | ❌ | ✅ |
| 订阅 | ❌ | ✅ |
| 依赖 | 无第三方 | zod + type-fest |
| 代码量 | ~800 行 Go | ~2000 行 TS |
| 适用场景 | 工具调用（Action Runtime） | 通用 MCP 客户端 |

**策略**：InferGlow 不需要实现完整的 MCP 规范（资源/提示词等）。当前 Action Runtime 只需要工具调用，已有的实现已足够。

---

## 五、内置工具包与权限控制方案

### 核心矛盾：灵活 vs 安全

```
方案 A：直接让 Agent 自由执行
  优点：极致灵活，Agent 可动态选择和组合工具
  缺点：安全风险极高，无审计/审批/限流

方案 B：审定后再执行（推荐）
  优点：安全可控，有审计和审批
  缺点：需要额外配置层
```

### 推荐方案：三层权限模型

```
┌──────────────────────────────────────────────┐
│ Layer 1：工具注册时声明权限                      │
│ ActionSpec.SideEffectLevel                   │
│ ActionSpec.ApprovalRequired                  │
│ ActionSpec.SandboxRequired                   │
├──────────────────────────────────────────────┤
│ Layer 2：Agent 执行时检查权限                    │
│ Agent.Run() → Engine.executeLoop()           │
│   ├─ 根据 session user role 查 PermissionMatrix │
│   ├─ 检查 Tool 是否在角色权限列表中              │
│   └─ 超限则降级/拒绝                           │
├──────────────────────────────────────────────┤
│ Layer 3：执行前安全门控                          │
│ ActionDispatcher                             │
│   ├─ PolicyApproval（审批门控）                 │
│   ├─ SandboxExecutor（隔离执行）                │
│   ├─ RateLimiter（速率控制）                    │
│   └─ AuditHook（审计日志）                     │
└──────────────────────────────────────────────┘
```

### 内置工具包设计

```
builtins/                                ← 新增模块
├── actions/                             内置 Action 注册
│   ├── calculator.go                    计算器
│   ├── web_search.go                    网页搜索
│   ├── file_read.go                     文件读取
│   ├── file_write.go                    文件写入（需审批）
│   ├── code_executor.go                 代码执行（需沙箱 + 审批）
│   ├── url_fetch.go                     URL 内容获取
│   ├── bash_executor.go                 Bash 执行（需沙箱 + 审批）
│   └── json_processor.go                JSON 处理
│
├── policies/                            预设权限策略
│   ├── restrictive.go                   严格模式（仅 read）
│   ├── balanced.go                      平衡模式（read + 受限 write）
│   └── permissive.go                    宽松模式（全部允许）
│
└── tools/                               工具描述生成
    ├── schema_from_func.go              从 Go 函数签名生成 ToolDefinition
    └── docstring_parser.go              docstring 解析
```

### 内置工具包安全策略

**核心设计：工具按风险等级分类**

| 工具 | 风险等级 | SideEffectLevel | ApprovalRequired | SandboxRequired | 默认权限 |
|------|---------|-----------------|------------------|-----------------|---------|
| Calculator | 低 | none | false | false | viewer/editor/admin |
| WebSearch | 低 | read | false | false | viewer/editor/admin |
| URLFetch | 低 | read | false | false | editor/admin |
| FileRead | 低 | read | false | false | editor/admin |
| FileWrite | 中 | write | true | false | admin |
| CodeExecutor | 高 | exec | true | true | admin |
| BashExecutor | 高 | exec | true | true | admin |

### 审批门控流程

```
用户提交 Agent 请求 (role=editor)
  │
  ▼
Engine.executeLoop()
  │
  ├── LLM 返回 action_calls: [calculator, file_write, code_executor]
  │
  ├── 1. RBAC 检查
  │     ├─ calculator: editor ✅
  │     ├─ file_write: editor ❌ → 拒绝执行
  │     └─ code_executor: editor ❌ → 拒绝执行
  │
  ├── 2. 仅放行 calculator
  │
  └── ActionDispatcher.Execute()
        ├─ calculator → LocalFunctionExecutor ✅
        └─ 结果回传 LLM
```

### 预审定模式（Blueprint-based）

适合高安全要求场景，Agent 在运行前工具列表已完全确定：

```go
// Blueprint-based 工具预审定
type ToolBlueprint struct {
    Name        string
    Description string
    Parameters  map[string]any
    Policy      ActionPolicy
    AllowedRoles []Role
}

// Agent 启动时加载 Blueprint，运行时仅执行预审定工具
func NewAgentWithBlueprint(ctx context.Context, blueprint []ToolBlueprint, role Role) *Agent {
    registry := action.NewRegistry()
    for _, tool := range blueprint {
        if !roleAllowed(tool.AllowedRoles, role) {
            continue  // 跳过该角色无权访问的工具
        }
        registry.Register(tool.ToAction())
    }
    return NewAgent(session, NewActionExtension(registry), modelReq)
}
```

---

## 六、实施路线图

### Phase 1：安全基础（Q3 2026）

| 项目 | 优先级 | 模块 | 预计工作量 |
|------|--------|------|-----------|
| Rate Limit | P0 | `security/ratelimit/` | 1 周 |
| 成本追踪 | P1 | 扩展 `audit/` | 0.5 周 |
| 内置工具包（精简） | P1 | `builtins/actions/` | 1 周 |
| 预审定模式 | P2 | `orchestrator/` | 0.5 周 |

### Phase 2：安全增强（Q4 2026）

| 项目 | 优先级 | 模块 | 预计工作量 |
|------|--------|------|-----------|
| Prompt 注入防护 | P0 | `security/prompt_injection/` | 2 周 |
| PII 脱敏 | P0 | `security/pii/` | 1 周 |
| RBAC 访问控制 | P0 | `security/rbac/` | 2 周 |
| 输入验证 | P1 | `security/input/` | 0.5 周 |

### Phase 3：可观测性（Q1 2027）

| 项目 | 优先级 | 模块 | 预计工作量 |
|------|--------|------|-----------|
| OpenTelemetry 集成 | P0 | `observability/otel/` | 2 周 |
| Langfuse 集成 | P1 | `observability/langfuse/` | 1 周 |
| 评估框架基础 | P1 | `observability/eval/` | 2 周 |

### Phase 4：模型扩展（Q1-Q2 2027）

| 项目 | 优先级 | 模块 | 预计工作量 |
|------|--------|------|-----------|
| 新增模型供应商 | P0 | `model/` | 2 周 |
| HTTP/SSE MCP 传输 | P0 | `action/mcp/` | 1 周 |
| 跨 Provider 路由 | P1 | `model/` | 2 周 |

---

## 附录：对标参考

### 各框架安全能力对比

| 能力 | LangChain | Agently | AutoGen | InferGlow |
|------|-----------|---------|---------|-----------|
| Prompt 注入防护 | ✅ (LangSmith) | ❌ | ❌ | ❌ → 待开发 |
| PII 脱敏 | ❌ | ❌ | ❌ | ❌ → 待开发 |
| RBAC | ❌ | ❌ | 部分 | ❌ → 待开发 |
| Rate Limit | ✅ 基础 | ❌ | ❌ | ❌ → 待开发 |
| 成本追踪 | ✅ | ❌ | ❌ | ❌ → 待开发 |
| 审计日志 | ✅ | ✅ action_logs | ✅ Docker audit | ✅ 链式哈希 |
| 沙箱隔离 | ❌ | ❌ | ✅ Docker | ✅ 7 后端 |
| 审批门控 | ❌ | ❌ | 部分 | ✅ 4 模式 |

### 各框架可观测性对比

| 能力 | LangChain+LangSmith | Agently | AutoGen | InferGlow |
|------|:---:|:---:|:---:|:---:|
| 分布式追踪 | ✅ 成熟 | ✅ 基础 | ✅ Azure | ✅ audit/ |
| 评估框架 | ✅ LangSmith | ❌ | 部分 | ❌ → 待开发 |
| A/B 测试 | ✅ LangSmith | ❌ | ❌ | ❌ → 待开发 |
| 成本追踪 | ✅ LangSmith | ❌ | 部分 | ❌ → 待开发 |
| OpenTelemetry | 间接 | ❌ | Azure | ❌ → 待开发 |
| Tamper-evident | ❌ | ❌ | ❌ | ✅ audit/ |

---

## 七、实施进度追踪（2026-07-21 审计）

> 基于代码库实际审计结果，对照第九/十章改进项清单，记录各项真实实现状态。

### 7.1 示例程序（9.1 — P2）

| 模块 | 计划状态 | 实际状态 | 文件 |
|------|---------|---------|------|
| model/ | 缺失 | ✅ 已实现 | `examples/example_model.go`（3 Provider + Streaming + Retry + Schema + Validator） |
| orchestrator/ | 缺失 | ✅ 已实现 | `examples/example_orchestrator.go`（Agent + Engine + LoopGuard + Audit + mock LLM） |
| audit/ | 缺失 | ✅ 已实现 | `examples/example_audit.go`（Chain + Sign + Verify + Export JSON/CSV/Text） |
| workspace/ | 缺失 | ✅ 已实现 | `examples/example_workspace.go`（Safe IO + Lineage + 血缘查询 + Cycle Detection） |
| bonus | 未计划 | ✅ 额外实现 | `example_action.go`、`example_flow.go`、`example_session.go`、`example_schema.go`、`integration_demo.go` |

**结论：100% 完成，超出预期（多出 5 个 bonus 示例）。**

### 7.2 Windows Runtime 完整实现（9.2 — P3）

| 后端 | 计划状态 | 实际状态 | 说明 |
|------|---------|---------|------|
| WindowsSandbox | stub | ✅ 框架完整，stub 级 | `Start()` 仅设状态，未接 `Microsoft.WindowsSandbox.exe` VM 启动 |
| AppContainer | stub | ✅ 框架完整，stub 级 | `Execute()` 未接 `StartAppContainerOperation`（shell32.dll），走普通 exec |
| RestrictedToken | stub | ✅ 框架完整，stub 级 | `CreateRestrictedToken()` 返回 sentinel error，未接 `DuplicateTokenEx`/`CreateRestrictedToken`（advapi32.dll） |

**Provider 层**：`WindowsRuntimeProvider` 完整实现（auto-select: Sandbox > AppContainer > RestrictedToken），`!windows` build tag 返回 `ErrProviderUnavailable`。12 个测试函数覆盖生命周期/状态转换/策略检查。

**结论：框架 100%，真实 OS 隔离 0%（需 Windows API 接入）。**

### 7.3 缺失核心模块（9.3 — P1-P3）

| 模块 | 优先级 | 计划状态 | 实际状态 | 说明 |
|------|--------|---------|---------|------|
| `security/` | P3 | 7 个子模块设计 | ✅ **已实现** | pii/prompt_injection/ratelimit/rbac/agenthook/sessionhook |
| `builtins/` | P1 | 8 个内置 Action 设计 | ✅ **已实现** | actions/policies/tools 子包 |
| `resource/` | P0 | Agently 等价 | ✅ **已实现** | 独立 Go module，执行资源生命周期管理 |
| `approval/` | P1 | Agently 等价 | ✅ **已实现** | 独立 Go module，策略审批框架 |
| `rag/` | P1 | 完整管道设计 | ❌ **未实现** | 零代码 |
| `model/pool.go` | P1 | ModelPool 路由/降级设计 | ❌ **未实现** | model/ 目录无 pool.go |
| `session/memory_plugin.go` | P2 | 记忆插件系统设计 | ❌ **未实现** | session/ 只有 persistence.go |
| `orchestrator/eventbus.go` | P3 | EventCenter 设计 | ❌ **未实现** | orchestrator/ 新增 recordstore/taskcontext/taskdag/skill/blocks 子包，但无 eventbus |

**结论：原 6 个缺失模块中 4 个已实现（security/builtins/resource/approval），2 个仍未实现（rag/model pool），另新增 Agently 等价组件 10 个全部完成。**

### 7.4 代码质量改进（9.4 — P3）

| 问题 | 计划状态 | 实际状态 | 说明 |
|------|---------|---------|------|
| 错误传播不一致 | 待统一 | ✅ 良好 | 统一 `fmt.Errorf(": %w", err)` 模式 |
| Stub 返回值 | 改用 sentinel | ⚠️ 部分完成 | Windows stub 有明确注释，但 `CreateRestrictedToken()` 仍用 `fmt.Errorf` 而非 sentinel error |
| 审计静默忽略 | 至少 log | ✅ 有意设计 | `NoOpHook` 返回 `("", nil)` 是"zero overhead"设计，非 bug |
| flow/operator 错误传播 | 统一 wrapping | 待检查 | 需具体审查 `flow/operator_handlers.go` |

**结论：整体代码质量良好，无静默错误忽略生产代码。**

### 7.5 测试覆盖增强（9.5 — P3）

| 测试类型 | 计划状态 | 实际状态 | 说明 |
|---------|---------|---------|------|
| 跨模块集成测试 | 端到端链路 | ⚠️ 部分完成 | 5 个模块有 `integration_test.go`（action/flow/schema/session/sandbox），但 model+orchestrator+action 串联测试缺失 |
| Mock 层测试 | mock HTTP client | ✅ 良好 | 21 个 mock 文件，内联 struct 模式，无生成式 mock 库 |
| Race condition | -race 检测 | ✅ 良好 | 26 处并发测试（session/flow/sandbox），atomic counter + 1000 轮迭代 |
| Benchmark | 性能基准 | ❌ **零覆盖** | 全项目无 `func Benchmark` 函数 |

**结论：并发测试覆盖良好，Benchmark 完全缺失。**

### 7.6 长期愿景（9.6 — P4-P5）

| 项目 | 计划状态 | 实际状态 | 说明 |
|------|---------|---------|------|
| TaskDAG | DAG 图编排 | ✅ 已实现 | `orchestrator/taskdag/` — TopoSort + Executor + Compile(→flow.Flow) |
| Agently 等价组件 | 10 个组件 | ✅ 已实现 | resource/approval/recordstore/taskcontext/taskdag/skill/blocks/dag_flow/strategy/workspace增强 |
| REST/WebSocket 服务 | HTTP API | ❌ 未实现 | MCP 仅支持 stdio，HTTP/SSE 传输缺失 |
| Deep Agents | 子 Agent 派生 | ❌ 未实现 | 零代码 |
| Multi-Agent | 多 Agent 协作 | ❌ 未实现 | 零代码 |

**结论：TaskDAG 和 Agently 等价组件已完整实现，其余长期愿景仍处于零进度。**

### 7.7 综合完成率

| 分类 | 计划项数 | 已完成 | 部分完成 | 未实现 | 完成率 |
|------|---------|:------:|:------:|:------:|:------:|
| 示例程序 | 4 | 4 | 0 | 0 | **100%** |
| Windows Runtime | 3 | 0 | 3（框架） | 0（API） | **框架100%/API 0%** |
| 核心模块 | 6 | 0 | 0 | 6 | **0%** |
| 代码质量 | 4 | 2 | 1 | 1 | **50%** |
| 测试覆盖 | 4 | 2 | 1 | 1 | **50%** |
| 长期愿景 | 5 | 0 | 0 | 5 | **0%** |
| **总计** | **22** | **6** | **5** | **11** | **~27%** |

### 7.8 优先级调整建议（基于实际审计）

原计划优先级可能需根据实际进度重新评估：

| 原优先级 | 原计划 | 调整建议 | 理由 |
|---------|--------|---------|------|
| **短期 P1** | 示例 + RAG + 内置 Action + ModelPool | 示例已做，聚焦 RAG + Action + Pool | RAG 和内置 Action 是对标 Agently 的关键差异能力 |
| **短期 P1** | — | **新增 Benchmark** | 零覆盖率，性能基线是质量保障基础 |
| **中期 P2** | security 独立 + 跨模块测试 + Workspace 完整 | 跨模块测试优先 | 已有 5 个单模块集成测试，串联测试能发现更多接口问题 |
| **中期 P3** | Session 插件 | 保持 | 依赖记忆系统的架构设计 |
| **长期 P3+** | Windows 真实隔离 | 保持 | 需 Windows 环境 + API 知识 |
| **长期 P4** | REST 服务 | 提前到 P1 | MCP HTTP/SSE 传输是 REST 的前置依赖，先补齐 MCP 传输层 |

---

## 八、Goal 1（G1-01 ~ G1-08）实施进度追踪（2026-07-22 更新）

> 基于 `tasks-20260721-goal-1.md` 任务列表与实际代码审计结果，逐项核实实现状态。

### G1-01：补齐国内 Provider 配置层

**状态：✅ 已完成（21/21 家 Provider 100% 实现）**

- `config.go`：**21 个配置项** ✅
- `provider_factory.go`：**20 个 Factory 函数** ✅
- 覆盖：openai/anthropic/ollama/deepseek/qwen/glm/kimi/stepfun/stepfun_anthropic/baidu/spark/sensenova/sensenova_anthropic/mimo/mimo_anthropic/tencent/volcengine/zeroone/minimax/siliconflow/openrouter

### G1-02：MiMo `reasoning_content` 字段适配

**状态：✅ 已实现（2026-07-27 更新）**

- ✅ `openAIChunk.Delta` 已包含 `ReasoningContent *string` 和 `ReasoningDetails *string` 字段
- ✅ `processOpenAILine` 同时解析 `reasoning` 和 `reasoning_content`，后者优先级更高
- ✅ `ChatMessage` 包含 `ReasoningContent` 和 `ReasoningDetails` 字段（`omitempty`）
- ✅ Session 层存储 reasoning_content 到 Meta，PreparePrompt 提取并回传
- ✅ Engine 层 stream loop 累积 reasoning 并传入 Decision 和 Session
- ✅ Anthropic 端点自动将 reasoning_content 转换为 thinking block（Kimi/DeepSeek/MiMo 兼容）
- ✅ 空值替代：空 reasoning_content 替换为 " "（空格）避免 DeepSeek V4 400 错误
- ✅ OpenRouter `reasoning_details` 字段解析已支持
- 覆盖 Provider：MiMo、讯飞星火（spark）、商汤（sensenova）、DeepSeek、OpenRouter

### G1-03：深度思考参数传递（thinking / reasoning_effort）

**状态：❌ 未实现**

- `Options["thinking"]` 和 `Options["reasoning_effort"]` 均无特殊处理逻辑

### G1-04：`<thinking>` tag 归一化

**状态：❌ 未实现**

- 无 `normalizeThinkingTags()` 函数

### G1-05：推理内容预算控制

**状态：❌ 未实现**

### G1-06：推理 token 单独计费

**状态：❌ 未实现**

### G1-07：核心路径 Benchmark

**状态：❌ 未实现**

### G1-08：ModelPool 路由降级

**状态：❌ 未实现**

### G1 系列综合完成率

| 任务 | 优先级 | 状态 | 完成度 |
|------|:------:|------|:------:|
| G1-01（Provider 配置层） | P1 | ✅ 已完成 | **100%** |
| G1-02（reasoning_content） | P1 | ✅ 已实现 | **100%** |
| G1-03（思考参数） | P1 | ❌ 未实现 | **0%** |
| G1-04（thinking tag） | P2 | ❌ 未实现 | **0%** |
| G1-05（推理预算） | P2 | ❌ 未实现 | **0%** |
| G1-06（推理计费） | P2 | ❌ 未实现 | **0%** |
| G1-07（Benchmark） | P3 | ❌ 未实现 | **0%** |
| G1-08（ModelPool） | P2 | ❌ 未实现 | **0%** |
| **总计** | — | — | **~12.5%** |

### G1 执行建议

按实际依赖关系，建议按以下顺序执行：

```
第一批（可并行）：
  G1-01 剩余 6 家 Provider 补全 → 完成 100%
  G1-07 Benchmark 基础 → 建立性能基线

第二批（依赖 G1-01）：
  G1-02 reasoning_content → 影响 MiMo/星火/商汤
  G1-03 思考参数 → 影响 MiMo/阶跃/商汤

第三批（依赖 G1-02）：
  G1-04 thinking tag 归一化
  G1-05 推理预算控制
  G1-06 推理 token 计费

第四批（独立）：
  G1-08 ModelPool 路由降级
```

---

## G1-01 已全部完成，无需未实现清单

> G1-01 Provider 配置层 20/20 家（100%）已实现，以下列表已清理。
> 剩余待开发任务为 G1-02 ~ G1-08（推理字段适配、thinking tag、Benchmark、ModelPool）。
