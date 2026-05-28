# 非标准 OpenAI 兼容 Provider 分析报告

> 生成日期：2026-07-21
> 调研对象：阶跃星辰、百度千帆、讯飞星火、商汤日日新
> 目的：分析各 Provider 的特殊 API 特征，确定适配方案

---

## 总览

| Provider | 特殊点数量 | 核心差异 | 适配方案 |
|----------|:---------:|---------|---------|
| 阶跃星辰 | 2 | Anthropic 兼容端点、step_plan 专用路径 | 双协议 Provider + 路径前缀 |
| 百度千帆 | 2 | `/v2` 路径版本、bce-v3 API Key 格式 | 标准 OpenAI 兼容（v2 仅路径不同） |
| 讯飞星火 | 3 | 双协议端点、WebSocket 原生端点、agent 路径 | HTTP 兼容端点为主，WS 端点可选 |
| 商汤日日新 | 3 | 双协议端点、兼容-mode 路径、reasoning_effort | 标准 OpenAI 兼容（双端点可任选） |

---

## 1. 阶跃星辰（StepFun）

### 1.1 基本信息

| 项目 | 值 |
|------|-----|
| 公司 | 阶跃星辰（StepFun） |
| 开放平台 | https://platform.stepfun.com/ |
| API 文档 | https://platform.stepfun.com/docs/zh/api-reference/chat/chat-completion-create |
| 推荐模型 | `step-3.7-flash`、`step-3.5-flash` |
| API Key 格式 | `sk-xxxxxxxx` |
| 认证方式 | `Authorization: Bearer $STEP_API_KEY` |

### 1.2 特殊点

#### 特殊点 1：双协议端点

阶跃星辰同时提供 **OpenAI 兼容** 和 **Anthropic 兼容** 两个端点：

| 协议 | Base URL | 说明 |
|------|----------|------|
| OpenAI 兼容 | `https://api.stepfun.com/v1` | 标准 `/v1/chat/completions` |
| OpenAI 兼容（Step Plan 专用） | `https://api.stepfun.com/step_plan/v1` | 订阅计划专用路径 |
| Anthropic 兼容 | `https://api.stepfun.com/step_plan` | Anthropic Messages API（不含 `/v1`） |
| Anthropic 兼容（step_plan 域名） | `https://api.stepfun.ai/step_plan` | 推理模型专用域名 |

**适配方案**：
- 在 `DEFAULT_SETTINGS` 中新增两个配置项：
  ```go
  "stepfun": {
      "model":      "step-3.7-flash",
      "base_url":   "https://api.stepfun.com/v1",
      "max_tokens": 4096,
  },
  "stepfun_anthropic": {
      "model":      "step-3.7-flash",
      "base_url":   "https://api.stepfun.com/step_plan",
      "max_tokens": 1024,
  },
  ```
- `stepfun_anthropic` 对应 `AnthropicCompatibleProvider`

#### 特殊点 2：推理强度参数

支持 `reasoning_effort` 参数（`low`/`medium`/`high`）控制思考深度，通过 Options 透传：

```go
Options["reasoning_effort"] = "high"  // 或 "medium", "low"
```

### 1.3 接入建议

- 优先级：P2（第二梯队，但有特色推理模型）
- 需要 `NewStepFunProviderFromConfig` + `NewStepFunAnthropicProviderFromConfig`
- 支持 Developer 角色（OpenAI 兼容端点）

---

## 2. 百度千帆（Qianfan）

### 2.1 基本信息

| 项目 | 值 |
|------|-----|
| 公司 | 百度智能云 |
| 开放平台 | https://console.bce.baidu.com/qianfan/ |
| API 文档 | https://cloud.baidu.com/doc/qianfan-api/s/3m7of64lb |
| 推荐模型 | `ernie-5.0`、`ernie-5.1`、`deepseek-v3.2` |
| API Key 格式 | `bce-v3/ALTAK-xxxx/xxxx` |
| 认证方式 | `Authorization: Bearer $QIANFAN_API_KEY` |

### 2.2 特殊点

#### 特殊点 1：API 路径版本号为 `/v2`

百度千帆 OpenAI 兼容端点的 base_url 路径为 `/v2` 而非标准的 `/v1`：

| 端点 | 值 |
|------|-----|
| OpenAI 兼容 | `https://qianfan.baidubce.com/v2` |
| 原生（旧版，已弃用） | `https://aip.baidubce.com/rpc/2.0/ai_custom/v1/...` |

**适配方案**：仅需在 base_url 中写对 `/v2`，其余完全兼容 OpenAI 协议。无需额外适配逻辑。

#### 特殊点 2：API Key 格式特殊

百度的 API Key 格式为 `bce-v3/ALTAK-xxxx/xxxx`，以 `bce-v3/` 开头。但认证方式仍是标准的 `Bearer Token`，所以 `OpenAICompatibleProvider` 无需特殊处理。

### 2.3 接入建议

- 优先级：P2（第二梯队，中文场景强）
- 只需一个 Factory：`NewBaiduProviderFromConfig`
- 无 RoleMapping 需求（支持标准角色）
- 注意：旧版 OAuth access_token 流程已弃用，新版直接用 Bearer Token

---

## 3. 讯飞星火（Spark）

### 3.1 基本信息

| 项目 | 值 |
|------|-----|
| 公司 | 科大讯飞 |
| 开放平台 | https://console.xfyun.cn/ |
| API 文档 | https://www.xfyun.cn/doc/spark/X2-Flash.html（X2-Flash）/ https://www.xfyun.cn/doc/spark/X1http.html（X1 HTTP）/ https://www.xfyun.cn/doc/spark/X1ws.html（X1 WS） |
| 推荐模型 | `spark-x`（X2-Flash）、`spark-x1` |
| API Key 格式 | HTTP: `AK:SK` 拼接；WS: 独立 APIKey/ApiSecret |
| 认证方式 | HTTP: `Authorization: Bearer AK:SK`；WS: 独立签名 |

### 3.2 特殊点

#### 特殊点 1：双协议端点

讯飞星火提供 **OpenAI 兼容 HTTP 端点** 和 **原生 WebSocket 端点**：

| 协议 | Base URL | 说明 |
|------|----------|------|
| OpenAI 兼容（X2-Flash） | `https://spark-api-open.xf-yun.com/agent/v1/` | `/chat/completions` |
| Anthropic 兼容（X2-Flash） | `https://spark-api-open.xf-yun.com/anthropic/agent/v1/` | `/messages` |
| 原生 WebSocket（历史版本） | `wss://spark-api.xf-yun.com/vX.X/chat` | V1/V2/V3/X.X 不同版本，仅 WSS |

**适配方案**：
- HTTP 端点完全兼容 OpenAI 协议，直接走 `OpenAICompatibleProvider`
- base_url 注意路径为 `/agent/v1/`（多了 `agent` 前缀）
- WebSocket 原生端点不适合 Go HTTP 客户端，不建议接入 InferGlow
- 建议只接入 HTTP OpenAI 兼容端点

#### 特殊点 2：推理内容字段

X2-Flash 模型在 SSE 流中返回 `reasoning_content` 字段（与 MiMo 相同），需 G1-02 适配。

```go
if hasattr(chunk.choices[0].delta, 'reasoning_content') {
    reasoning_content = chunk.choices[0].delta.reasoning_content
}
```

#### 特殊点 3：API Key 格式

HTTP 认证使用 `AK:SK` 拼接格式（如 `AK123:SK456`），本质还是 Bearer Token，无需特殊处理。

### 3.3 接入建议

- 优先级：P2（第二梯队，语音能力最强）
- 只需一个 Factory：`NewSparkProviderFromConfig`
- 注意 RoleMapping 需实测（可能支持 developer 角色）
- WebSocket 原生端点不纳入适配范围

---

## 4. 商汤日日新（SenseNova）

### 4.1 基本信息

| 项目 | 值 |
|------|-----|
| 公司 | 商汤科技 |
| 开放平台 | https://platform.sensenova.cn/ |
| API 文档 | https://platform.sensenova.cn/docs |
| 推荐模型 | `sensenova-6.7-flash-lite`、`deepseek-v4-flash`、`sensenova-u1-fast` |
| API Key 格式 | `sk-xxxxxxxx` |
| 认证方式 | `Authorization: Bearer $SENSENOVA_API_KEY` |

### 4.2 特殊点

#### 特殊点 1：双协议端点

商汤 SenseNova 同时提供 **OpenAI 兼容** 和 **Anthropic 兼容** 两个端点：

| 协议 | Base URL | 说明 |
|------|----------|------|
| OpenAI 兼容（Token Plan） | `https://token.sensenova.cn/v1` | 公测/订阅计划专用 |
| OpenAI 兼容（标准） | `https://api.sensenova.cn/compatible-mode/v2` | 标准 API 路径 |
| Anthropic 兼容 | `https://token.sensenova.cn/v1/messages` | Anthropic Messages API |
| Anthropic 兼容（标准） | `https://api.sensenova.cn/compatible-mode/v1/messages` | 标准 API 路径 |

**适配方案**：
- Token Plan 端点（`token.sensenova.cn`）是公测免费渠道，适合试用
- 标准端点（`api.sensenova.cn`）是正式商用路径
- 建议 `DEFAULT_SETTINGS` 中只加入 Token Plan 端点（免费版），标准端点后续按需补充
- 两个端点的 API 格式完全一致，只是域名不同

#### 特殊点 2：推理参数

商汤的推理控制使用 `reasoning_effort`（`low`/`medium`/`high`/`none`），通过 Options 传递。

注意：商汤原生接口使用 `thinking.enabled` 布尔值（非 OpenAI 标准），但 OpenAI 兼容端点使用 `reasoning_effort`。

#### 特殊点 3：原生接口路径差异

商汤的 **原生接口** 路径为 `https://chatapi.sensenova.cn/v1/llm/chat-completions`，与 OpenAI 兼容端点不同。原生接口使用完全不同的请求体格式（`max_new_tokens` 而非 `max_tokens`，`thinking.enabled` 而非 `reasoning_effort`）。

**适配方案**：不接入原生接口，仅通过 OpenAI 兼容端点接入。

### 4.3 接入建议

- 优先级：P2（目前公测免费，有吸引力）
- 需要两个 Factory：`NewSenseNovaProviderFromConfig`（OpenAI）+ `NewSenseNovaAnthropicProviderFromConfig`（Anthropic）
- 注意 base_url 使用 `token.sensenova.cn/v1`（Token Plan 域名）
- 支持 developer 角色（实测确认）
- reasoning_content 需 G1-02 适配

---

## 汇总：所有 Provider 的特殊特征矩阵

| Provider | 标准 `/v1` 路径 | 特殊路径 | 双协议 | Reasoning 字段 | RoleMapping 需实测 |
|----------|:---:|----------|:---:|:---:|:---:|
| OpenAI | ✅ | - | - | `reasoning` | 否 |
| Anthropic | - | `/v1/messages` | - | `thinking` | N/A |
| Ollama | ✅ | - | - | `reasoning` | 否 |
| DeepSeek | ✅ | - | - | `reasoning` | ✅ 已有 |
| Qwen | ✅ | - | - | `reasoning` | ✅ 已有 |
| GLM | ✅ | - | - | `reasoning` | ✅ 已有 |
| Kimi | ✅ | - | - | `reasoning` | ✅ 已有 |
| MiMo | ✅ | Anthropic 端点 | ✅ | **`reasoning_content`** | 需实测 |
| 腾讯混元 | ✅ | - | - | `reasoning` | 否 |
| Volcengine | ✅ | - | - | `reasoning` | 可能 |
| 01.AI | ✅ | - | - | `reasoning` | 需实测 |
| MiniMax | ✅ | - | - | `reasoning` | 需实测 |
| SiliconFlow | ✅ | - | - | `reasoning` | 需实测 |
| **阶跃星辰** | ✅ | **step_plan 路径** | **✅** | `reasoning` | 否 |
| **百度千帆** | ✅ | **`/v2` 路径** | - | `reasoning` | 否 |
| **讯飞星火** | ✅ | **`/agent/v1/` 路径** | **✅** | **`reasoning_content`** | 需实测 |
| **商汤日日新** | ✅ | **双域名** | **✅** | **`reasoning_content`** | 需实测 |

---

## G1-01 扩展：新增 Provider 配置清单（2026-07-22 更新）

> **G1-01 已全部完成**：20 家 Provider 100% 实现（12 已有 + 8 新增 = 20 家，19 Factory 函数，20 配置项）

### 已完成全部 Provider 配置清单

| 批次 | Provider | 配置 key | Base URL | 特殊处理 | 状态 |
|------|----------|---------|----------|---------|:----:|
| 已有 | OpenAI | `openai` | `api.openai.com/v1` | — | ✅ |
| 已有 | Anthropic | `anthropic` | `api.anthropic.com` | Anthropic Messages API | ✅ |
| 已有 | Ollama | `ollama` | `localhost:11434` | Ollama 专用协议 | ✅ |
| 已有 | DeepSeek | `deepseek` | `api.deepseek.com/v1` | RoleMapping | ✅ |
| 已有 | Qwen | `qwen` | `dashscope.aliyuncs.com/compatible-mode/v1` | RoleMapping | ✅ |
| 已有 | GLM | `glm` | `open.bigmodel.cn/api/paas/v4` | RoleMapping | ✅ |
| 已有 | Kimi | `kimi` | `api.moonshot.cn/v1` | RoleMapping | ✅ |
| G1-01 | **阶跃星辰** | `stepfun` | `api.stepfun.com/v1` | `reasoning_effort` | ✅ |
| G1-01 | **阶跃星辰** | `stepfun_anthropic` | `api.stepfun.com/step_plan` | Anthropic 端点 | ✅ |
| G1-01 | **百度千帆** | `baidu` | `qianfan.baidubce.com/v2` | `/v2` 路径 | ✅ |
| G1-01 | **讯飞星火** | `spark` | `spark-api-open.xf-yun.com/agent/v1/` | `reasoning_content` | ✅ |
| G1-01 | **商汤日日新** | `sensenova` | `token.sensenova.cn/v1` | `reasoning_content` + `reasoning_effort` | ✅ |
| G1-01 | **商汤日日新** | `sensenova_anthropic` | `token.sensenova.cn/v1/messages` | Anthropic 端点 | ✅ |
| G1-01 | **MiMo** | `mimo` | `api.xiaomimimo.com/v1` | `reasoning_content` + `thinking.type` | ✅ |
| G1-01 | **MiMo** | `mimo_anthropic` | `api.xiaomimimo.com/anthropic` | Anthropic 端点 | ✅ |
| G1-01 | **腾讯混元** | `tencent` | `api.hunyuan.cloud.tencent.com/v1` | 标准 OpenAI | ✅ |
| G1-01 | **豆包 Seed** | `volcengine` | `ark.cn-beijing.volces.com/api/v3` | 标准 OpenAI | ✅ |
| G1-01 | **01.AI** | `zeroone` | `api.01.ai/v1` | 标准 OpenAI | ✅ |
| G1-01 | **MiniMax** | `minimax` | `api.minimax.chat/v1` | 标准 OpenAI | ✅ |
| G1-01 | **SiliconFlow** | `siliconflow` | `api.siliconflow.cn/v1` | 聚合平台 | ✅ |

### 实现进度

```
████████████████████  100%  (20/20 家)
✅ 20 家全部实现: openai/anthropic/ollama/deepseek/qwen/glm/kimi/stepfun/stepfun_anthropic/baidu/spark/sensenova/sensenova_anthropic/mimo/mimo_anthropic/tencent/volcengine/zeroone/minimax/siliconflow
```

---

## 适配规则总结

### 不需要修改 Provider 实现的场景

以下差异只需在 `DEFAULT_SETTINGS` 的 `base_url` 中正确配置，`OpenAICompatibleProvider` 无需修改：

1. `/v2` 路径（百度千帆）
2. `/agent/v1/` 路径（讯飞星火）
3. `/step_plan/v1` 路径（阶跃星辰计划端）
4. 不同域名（商汤 token.sensenova.cn vs api.sensenova.cn）
5. API Key 格式不同（百度 bce-v3、讯飞 AK:SK）— 本质都是 Bearer Token

### `full_url` 完全覆盖（model-parity Phase 1）

> 当 `base_url + default_path` 拼接无法满足非标端点时，`full_url` 配置项提供完全覆盖。

**机制**：`ResolveURL(baseURL, defaultPath, fullURL)` — 当 `fullURL != ""` 时直接使用，跳过拼接逻辑。

**配置方式**：
```yaml
my_provider:
  api_key: "sk-xxx"
  base_url: "https://api.example.com/v1"
  full_url: "https://gateway.proxy.com/custom/llm/chat"  # 完全覆盖
```

**适用场景**：
- API 网关/代理重写路径（base_url 不变但实际请求路径不同）
- 自定义端点路径无法用 `base_url + /chat/completions` 拼接表达
- 灰度发布/AB 测试时切换到不同端点 URL

**已覆盖 Provider**：`OpenAICompatibleProvider`、`AnthropicCompatibleProvider`、`OllamaProvider`、`OpenAIResponsesProvider` 均已集成 `FullURL` 字段。

### `content_mapping` 非标字段路径提取（model-parity Phase 3）

> 当 Provider 的 SSE JSON 字段路径与标准 OpenAI 格式不同时，`content_mapping` 配置项允许自定义提取路径。

**机制**：`ContentMapping` 是 `map[string]string`，`ExtractByPath(data, path)` 支持点号/斜杠分隔 + 数组索引（`choices[0].delta.content`）。

**默认映射**（`DefaultOpenAIContentMapping`）：
```go
{
    "reasoning": "choices[0].delta.reasoning_content",
    "delta":     "choices[0].delta.content",
}
```

**配置方式**：
```yaml
my_provider:
  api_key: "sk-xxx"
  content_mapping:
    reasoning: "data.thinking"       # 非标推理字段路径
    delta: "message.content"          # 非标内容字段路径
```

**适用场景**：
- 某些代理/网关重写了 SSE JSON 结构（字段嵌套层级或名称不同）
- Ollama 兼容端点返回非标结构（`data.thinking` 而非 `message.reasoning`）

**已覆盖 Provider**：`OpenAICompatibleProvider`（`processOpenAILine` 中 `ContentMapping != nil` 分支）、`OllamaProvider`（`processOllamaLineWithMapping`）。`ContentMapping == nil` 时走原有 struct 解析，完全向后兼容。

### 需要 G1-02 推理字段适配的场景

以下 Provider 使用 `reasoning_content` 而非 `reasoning`：

- MiMo（OpenAI 兼容端点）
- 讯飞星火 X2-Flash
- 商汤 SenseNova

### 需要 G1-03 深度思考参数透传的场景

以下 Provider 有非标准 `thinking` 参数：

- MiMo: `thinking.type = "enabled"/"disabled"`
- 阶跃星辰: `reasoning_effort = "low"/"medium"/"high"`
- 商汤: `reasoning_effort = "low"/"medium"/"high"/"none"`

### 需要新增 AnthropicCompatibleProvider 的场景

以下 Provider 同时提供 Anthropic 兼容端点：

- MiMo: `api.xiaomimimo.com/anthropic`
- 阶跃星辰: `api.stepfun.com/step_plan`
- 商汤: `token.sensenova.cn/v1/messages`

---

## 推理内容特殊处理汇总

> 推理内容（reasoning/thinking/thinking-content）在不同 Provider 中的返回格式差异较大，
> 需要在 `OpenAICompatibleProvider` 和 `AnthropicCompatibleProvider` 中做统一归一化处理。

### 推理内容字段映射表

| Provider | 推理字段名 | 来源文档 | 归一化目标 |
|----------|-----------|---------|-----------|
| OpenAI (o1/o3) | `reasoning` | OpenAI API 文档 | `ModelResponse.Reasoning` |
| Anthropic (Claude) | `thinking` | Anthropic Messages API | `ModelResponse.Reasoning` |
| DeepSeek (R1/V3/V4) | `reasoning` | DeepSeek API 文档 | `ModelResponse.Reasoning` |
| **MiMo** | **`reasoning_content`** | **MiMo API 文档** | **`ModelResponse.Reasoning`** |
| **讯飞星火 X2-Flash** | **`reasoning_content`** | **星火 HTTP 协议文档** | **`ModelResponse.Reasoning`** |
| **商汤 SenseNova** | **`reasoning_content`** | **商汤兼容模式文档** | **`ModelResponse.Reasoning`** |

### `<think>` Tag 归一化

> InferGlow 已实现 `LeadingThinkNormalizer`（[think_normalizer.go](../model/think_normalizer.go)），对标 Agently 的 `LeadingThinkEventNormalizer`。

部分模型在 **content 字段**中返回 `<think>` 标签包裹的推理内容（而非单独的推理字段）：

| Provider | 场景 | 返回格式 | 归一化方案 |
|----------|------|---------|-----------|
| DeepSeek-R1/V4 前端 | 非流式/某些 proxy | `{"content": "<think>...</think> 回答"}` | 正则提取 `</think>` 前内容到 `Reasoning` |
| 商汤 SenseNova 原生接口 | 深度思考模式 | `{"content": "<think>...</think> ..."}` | 同 DeepSeek 方案 |

**实现细节**：
- `LeadingThinkNormalizer` 是三态状态机（unknown/reasoning/answer），在 `BroadcastResponse` 流式路径中逐 chunk 调用 `FeedDelta` 分离 reasoning 与 answer
- 支持分块缓冲（`<thi` + `nk>foo` 跨 chunk 拼接）和大小写不敏感匹配（`<THINK>` / `</THINK>`）
- `chunk.Reasoning` 已由 Provider 独立字段填充时跳过状态机，避免双重处理
- `FeedDone` 方法提供非流式提取，`normalizeThinkingTags`/`hasThinkingTags` 作为内部辅助函数（已从 `openai.go` 迁移至 `think_normalizer.go`）
- 三个 Provider（OpenAI/Anthropic/Ollama）的 `BroadcastResponse` 均已集成，结束时 `fullReasoning == ""` 触发 `normalizeThinkingTags` 防御性兜底

**注意**：
- 标准 API 流式返回中，推理内容和最终回答在**不同的 delta chunk** 中（`reasoning`/`reasoning_content` vs `content`）
- 只有非流式或某些前端/代理会返回 `<think>` 标签包裹的内容
- 归一化函数 `normalizeThinkingTags()` 在 SSE 解析后和 non-stream 响应处理中分别调用

### 非流式 Thinking 标签处理结论

> 经对 InferGlow 代码库搜索分析，以下是非流式场景下 thinking 处理的完整结论。

#### 当前状态：normalizeThinkingTags 已实现

| 项目 | 现状 |
|------|------|
| 非流式请求能力 | **不存在** — 所有 Provider 硬编码 `stream: true` |
| `processOpenAILineNonStream` | **不存在** — 没有非流式 SSE 解析函数 |
| `normalizeThinkingTags` | **已实现** — 位于 `think_normalizer.go`，大小写不敏感正则提取 |
| `reasoning_content` 非流式解析 | **已实现** — `processOpenAILine` 同时解析 `reasoning` 和 `reasoning_content` |

**代码证据**：
- `RequestModel` 接口（`model.go`）返回类型为 `(<-chan *StreamChunk, error)`，始终为流式 channel
- 所有 Provider 实现（`openai.go`、`anthropic.go`、`ollama.go`）的 `RequestModel` 方法硬编码 `"stream": true`
- `inferglow-agent-blindspot-analysis.md` 标注 `LeadingThinkEventNormalizer` 为 **不存在**

#### 非流式场景下各厂商推理内容格式

> 需要确认：哪些厂商的非流式 API 会返回 `think` 标签包裹在 content 中？

| 厂商 | 非流式推理字段 | 是否可能含 <think> 标签 | 说明 |
|------|--------------|:---:|------|
| **DeepSeek** | `reasoning` | ✅ **可能** | 文档标注"非流式/某些 proxy"场景会返回标签包裹格式 |
| **商汤 SenseNova** | `reasoning_content` | ✅ **可能**（原生接口） | 原生接口 `thinking.enabled` 模式可能返回标签包裹，但已排除不接入 |
| **阶跃星辰** | `reasoning` | ❓ 未明确 | 文档未说明其非流式是否混入标签 |
| **百度千帆** | `reasoning` | ❓ 未明确 | 文档未说明 |
| **讯飞星火** | `reasoning_content` | ❓ 未明确 | 文档未说明 |
| **OpenAI (o1/o3)** | `reasoning` | ❌ 否 | OpenAI 官方 API 非流式始终返回 `reasoning` 独立字段 |
| **其他厂商** | `reasoning`/`reasoning_content` | ❌ 否 | 标准 OpenAI 兼容端点的非流式响应中，推理字段独立于 content |

#### 结论与建议

1. **`</think>` 标签包裹在 content 中，主要是 DeepSeek 非流式场景的典型特征**（以及使用 DeepSeek R1 模型的代理/前端）
2. 其他厂商的非流式 API 遵循标准 OpenAI 兼容格式：推理内容在独立的 `reasoning` 或 `reasoning_content` 字段
3. 当前 InferGlow 只支持流式（`stream: true`），因此推理内容始终从 SSE 的独立 delta chunk 读取
4. **G1-04 优先级**：实现 `normalizeThinkingTags()` 主要是为了兼容 DeepSeek 非流式场景和未来可能的非流式需求

### G1-02 与 G1-03 的覆盖范围

| 任务 | 覆盖 Provider | 说明 |
|------|:-----------:|------|
| **G1-02** (`reasoning_content` 解析) | MiMo、讯飞星火、商汤 | 新增 `ReasoningContent` 字段解析，优先于 `Reasoning` |
| **G1-04** (`<think>` 归一化) | DeepSeek、商汤原生接口 | 正则提取标签内容，不影响标准字段解析 |
| **G1-03** (思考参数透传) | MiMo (`thinking.type`)、阶跃星辰/商汤 (`reasoning_effort`) | 通过 Options 透传 |

### 特殊处理决策

| 问题 | 处理方式 | 归属任务 |
|------|---------|---------|
| `reasoning` vs `reasoning_content` 字段名差异 | SSE 解析时同时读取两个字段，`reasoning_content` 优先 | G1-02 |
| `thinking` 字段（Anthropic 兼容端点） | AnthropicCompatibleProvider 已有处理 | 已有 |
| `<think>` 标签包裹在 content 中 | 新增 `normalizeThinkingTags()` 函数 | G1-04 |
| `thinking.enabled` 布尔值（商汤原生接口） | **不接入原生接口**，仅通过 OpenAI 兼容端点使用 `reasoning_effort` | — |
| `reasoning_effort` 字符串参数 | 通过 Options 透传到 request body | G1-03 |

---

## Provider 适配器方法论

> 本节综合 OpenClaw（TypeScript）、Hermes Agent（Python）、inference-go/trpc-agent-go（Go）等主流框架的
> Provider 适配实践，提炼出可复用的方法论。目标：让 InferGlow 的 Provider 适配遵循统一模式，
> 新增 Provider 只需改配置，不改核心逻辑。

### 方法论 A：四层层叠适配器架构

**来源**：Hermes/Eino 社区总结的"Profile + Router + Workflow + Adapter"四层架构

```
Layer 4: 业务代码          ChatRequest{ Profile: "deepseek-v3", Input: "你好" }
Layer 3: Router           按 Profile 名选择 Provider 接口
Layer 2: Workflow/协议适配   openai-compat / anthropic-compat / ... 处理请求/响应格式
Layer 1: Provider 实现     HTTP 调用 + SSE 解析 + 归一化
```

**核心原则**：
- 业务代码只接触 `Provider` 接口，不感知下面是哪家
- `Profile` 配置驱动（YAML/JSON），加新 Provider 只需加一行
- 90%+ 国产模型走 `openai-compat` workflow，**零代码** 接入
- 只有接口格式特殊的（Anthropic）才需要独立 workflow

**InferGlow 映射**：
- Layer 4 → `ModelRequest`
- Layer 3 → `provider_factory.go`（按 name 选 Provider）
- Layer 2 → `OpenAICompatibleProvider` / `AnthropicCompatibleProvider`
- Layer 1 → Provider 内部实现（`generate.go`、`broadcast.go`）

### 方法论 B：三阶段流式归一化

**来源**：OpenClaw 的 "resolve → wrap → normalize" 三阶段适配器模式

```
┌─────────────────────────────────────────────────────┐
│ Phase 1: Model Resolution（模型解析）                 │
│ 输入: modelId="mimo-v2.5-pro"                       │
│ 输出: provider="mimo", protocol="openai-compat"     │
│ 决策: registry lookup → capabilities flag           │
├─────────────────────────────────────────────────────┤
│ Phase 2: Stream Wrapping（流包装）                    │
│ 输入: raw HTTP stream from provider                  │
│ 输出: provider-specific SSE parser                   │
│ 决策: openai-stream-wrappers.go / anthropic-stream... │
├─────────────────────────────────────────────────────┤
│ Phase 3: Output Normalization（输出归一化）            │
│ 输入: SSE chunks in provider-specific format         │
│ 输出: uniform StreamChunk{Role, Content, Reasoning}  │
│ 决策: reasoning_content → Reasoning, thinking → Reasoning│
└─────────────────────────────────────────────────────┘
```

**核心规则**：
- 每个 wrapper 文件实现**相同的输出契约**
- 新增 Provider 只需写一个 wrapper 文件，不动任何 core logic
- 归一化输出统一为 `StreamChunk{ Role, Content, Reasoning, ToolCalls }`

**InferGlow 映射**：
- Phase 1 → `provider_factory.go` 中 `resolveProvider(name)`
- Phase 2 → `OpenAICompatibleProvider.RequestModel` 中的 HTTP 调用
- Phase 3 → `processOpenAILine` / `BroadcastResponse` 中的 SSE 解析 + 字段映射

### 方法论 C：Quirks 数据驱动输出修复

**来源**：Hermes/Eino 社区的 Quirks 配置驱动修复

不同 Provider 有各种输出"怪癖"（quirks），用数据驱动方式修复：

| Quirk 类型 | 表现 | 修复方式 |
|-----------|------|---------|
| Markdown 链接注入 | DeepSeek 偶发 URL 混入结构化字段 | `regex_replace` 移除 |
| 布尔值序列化 | 智谱返回 `1/0/'yes'/'no'` | 类型转换 |
| 嵌套对象 | 千问把单值字段包成 `{value:x}` | 解包 |
| 尾逗号 | 小模型 JSON 带尾逗号 | `json` 解析重试 |
| Code fence 包裹 | Claude 偶发 ` ```json ... ``` ` | 正则提取 |
| `<think>` 标签 | DeepSeek-R1/V4 返回标签包裹 | 正则提取到 Reasoning |

**InferGlow 映射**：
- 在 `BroadcastResponse` 输出前加一层 "quirk fixer" 中间件
- 按 provider name 路由到不同的修复规则
- 规则可通过配置扩展，不硬编码在 provider 实现中

```go
// QuirkFixer 伪代码
func fixResponse(providerName string, content string) string {
    switch providerName {
    case "mimo":
        content = stripThinkingTags(content)    // reasoning_content 处理
    case "deepseek":
        content = stripMarkdownLinks(content)   // URL 注入修复
    case "openai":
        content = stripCodeFence(content)       // ```json ... ``` 提取
    }
    return content
}
```

### 方法论 D：推理字段统一映射

**来源**：所有框架的共识——推理字段名不统一，必须在归一化阶段处理

| 阶段 | 处理方式 |
|------|---------|
| 字段名差异 | SSE 解析时**同时读取** `reasoning`、`reasoning_content`、`thinking` 三个字段，合并到统一 `Reasoning` 字段 |
| 字段优先级 | `reasoning_content` > `reasoning` > `thinking`（覆盖顺序） |
| 标签包裹 | 解析后检查 `content` 是否含 `<think>`，提取到 `Reasoning`，clean 掉标签内容 |
| 参数差异 | `thinking.type`、`reasoning_effort`、`thinking.enabled` 统一走 Options 透传 |

**InferGlow 代码映射**：

```go
// processOpenAILine 中
if c.Delta.Reasoning != nil {
    result.Reasoning += *c.Delta.Reasoning
}
if c.Delta.ReasoningContent != nil {
    // reasoning_content 覆盖 reasoning（MiMo/Spark/SenseNova 用此字段）
    result.Reasoning += *c.Delta.ReasoningContent
}

// 解析完成后
if result.Reasoning == "" && hasThinkingTags(result.Content) {
    reasoning, cleaned := normalizeThinkingTags(result.Content)
    result.Reasoning = reasoning
    result.Content = cleaned
}
```

### 方法论 E：Go 语言实现模式

**来源**：inference-go、trpc-agent-go、Eino 的 Go 实践

| 维度 | 模式 | 说明 |
|------|------|------|
| **接口定义** | 统一 `Model` interface | `Generate()`, `Stream()`, `Embed()`, `Tokenize()` |
| **流式返回** | `chan *StreamChunk` | Go 原生 channel 传递，天然支持 goroutine |
| **HTTP 调用** | `net/http.Client` + `bufio.Scanner` | 标准库实现，零外部依赖 |
| **SSE 解析** | 逐行扫描 + `json.Unmarshal` | 每行独立解析，不缓冲整个响应 |
| **并发安全** | `sync.RWMutex` 保护 Provider registry | 配置变更不阻塞请求 |
| **错误处理** | 分层错误包装 | `fmt.Errorf(": %w", err)` + `errors.Is/As` |
| **配置加载** | `map[string]Config` 结构体 | 从 YAML/JSON/环境变量 多层加载 |
| **测试策略** | Mock SSE stream + `httptest` | 无网络依赖的单元测试 |

**inference-go 的特殊实践**（值得参考）：
- `AddProviderFromPreset`：通过预设名一键注册 Provider（`openai`, `anthropic`, `mistral` 等）
- `ModelCapabilityResolver`：自动从模型 ID 推断能力（thinking、tool calling、structured output）
- `CapabilityOverride`：允许 override preset 的能力判断

```go
// inference-go 模式（Go）
provider := &openai.CompatibleProvider{
    BaseURL: "https://api.xiaomimimo.com/v1",
    Model:   "mimo-v2.5-pro",
}
provider.SetCapabilities(
    openai.WithReasoning(true),
    openai.WithThinkingField("reasoning_content"),  // 指定推理字段名
)
```

**InferGlow 可借鉴的点**：
1. `SetCapabilities` 或类似机制标注 Provider 特性（thinking、tool calling）
2. `ProviderPreset` 预定义常用 Provider 模板
3. `NormalizeReasoningField` 作为可配置的能力属性

### 方法论 F：配置驱动的 Provider 注册

**来源**：OpenClaw 的 `normalizeConfig` + Eino 的 `profiles.yaml`

**推荐方案**：结合两者的优势

```yaml
# inferglow/config/providers.yaml
providers:
  openai:
    presets: default_openai
    base_url: ${OPENAI_BASE_URL:https://api.openai.com/v1}
    auth: ${OPENAI_API_KEY}
    capabilities:
      reasoning: true
      tool_calling: true
      thinking_field: reasoning

  mimo:
    presets: default_openai
    base_url: ${MIMO_BASE_URL:https://api.xiaomimimo.com/v1}
    auth: ${MIMO_API_KEY}
    capabilities:
      reasoning: true
      thinking_field: reasoning_content     # 关键字段
      thinking_param: thinking.type        # 思考参数名
      extra_body:
        thinking:
          type: enabled

  stepfun:
    presets: default_openai
    base_url: ${STEPFUN_BASE_URL:https://api.stepfun.com/v1}
    auth: ${STEPFUN_API_KEY}
    capabilities:
      reasoning: true
      reasoning_effort: true               # 推理强度控制
```

**优势**：
- 配置驱动：加 Provider 只需加一段 YAML
- 能力声明：`thinking_field`、`reasoning_effort` 等能力元数据
- 环境变量兜底：`${VAR:default}` 语法
- Presets 复用：`default_openai` 包含通用配置模板

---

## 参考实现关键代码摘录

> 本节从已解压的四个官方仓库中提取核心代码片段（20-50 行），供 InferGlow 实现时参考。
> 所有文件路径基于本地解压目录：
> - `/openclaw-extract/openclaw-main/` — TypeScript
> - `/hermes-extract/hermes-agent-main/` — Python
> - `/langchain-extract/langchain-main/` — Python
> - `/langchaingo-extract/langchaingo-main/` — Go

### 1. OpenClaw（TypeScript）— Provider Plugin Runtime Hook 体系

**核心模式**：插件声明 + 30+ 运行时钩子（hooks）接管差异化逻辑

#### 1.1 Provider 注册查找

**文件**：`src/plugins/provider-runtime.ts:110-121`（provider 引用解析）+ `src/plugins/provider-hook-runtime.ts:229-305`（运行时插件查找）

```typescript
// 通过 provider id + api 别名构建查找键
function resolveProviderHookRefs(
  provider: string,
  providerConfig?: ModelProviderConfig,
  modelApi?: string,
): string[] {
  const refs = [provider];
  const apiRef = normalizeOptionalString(modelApi ?? providerConfig?.api);
  if (apiRef && normalizeProviderId(apiRef) !== normalizeProviderId(provider)) {
    refs.push(apiRef);
  }
  return uniqueStrings(refs);
}

// 运行时插件查找（带缓存）
export function resolveProviderRuntimePlugin(
  params: ProviderRuntimePluginLookupParams,
): ProviderPlugin | undefined {
  const apiOwnerHint = resolveProviderConfigApiOwnerHint({ ... });
  const providerRefs = apiOwnerHint ? [params.provider, apiOwnerHint] : [params.provider];
  const loadedPlugin = findProviderRuntimePluginInLoadedRegistries({ lookup, apiOwnerHint });
  if (loadedPlugin) return loadedPlugin;
  // ... 缓存逻辑 + 查找注册表 → 按 provider id 匹配
  return plugin ?? undefined;
}
```

#### 1.2 推理输出模式钩子

**文件**：`src/plugins/provider-model-shared.ts:15-49`（resolveReasoningOutputMode）+ `src/plugins/provider-runtime.ts:581-598`

```typescript
function resolveReasoningOutputMode(params: { ... }): "native" | "tagged" {
  const provider = normalizeOptionalString(params.provider);
  if (!provider) return "native";

  const pluginMode = resolveProviderReasoningOutputModeWithPlugin({
    provider, context: { config, provider, modelId, modelApi, model },
  });
  if (pluginMode) return pluginMode;
  // 默认回 native
}

// Provider 推理模式钩子
export function resolveProviderReasoningOutputModeWithPlugin(params: { ... }) {
  const mode = ensureProviderRuntimePluginHandle({...}).plugin
    ?.resolveReasoningOutputMode?.(params.context);
  return mode === "native" || mode === "tagged" ? mode : undefined;
}
```

#### 1.3 流式包装器钩子

**文件**：`src/plugins/provider-hook-runtime.ts:439-450` + `src/plugins/provider-runtime.ts:600-613`

```typescript
// 流式包装器查找
export function wrapProviderStreamFn(params: {
  provider: string; config?: OpenClawConfig; ...;
  context: ProviderWrapStreamFnContext;
}) {
  return ensureProviderRuntimePluginHandle(params)
    .plugin?.wrapStreamFn?.(params.context) ?? undefined;
}

// 流式函数创建
export function resolveProviderStreamFn(params: {
  provider: string; ...; context: ProviderCreateStreamFnContext
}) {
  const plugin = params.allowRuntimePluginLoad === false
    ? resolveLoadedProviderRuntimePlugin(params)
    : resolveProviderRuntimePlugin(params);
  return plugin?.createStreamFn?.(params.context) ?? undefined;
}
```

#### 1.4 Thinking 级别声明

**文件**：`src/plugins/provider-thinking.types.ts:39-48`

```typescript
type ProviderThinkingLevelId =
  | "off" | "minimal" | "low" | "medium" | "high"
  | "xhigh" | "adaptive" | "max" | "ultra";
```

---

### 2. Hermes Agent（Python）— Registry Map + api_mode 桥接

**核心模式**：硬编码注册表 + 插件自动扩展 + 三种 API 模式 dispatch

#### 2.1 PROVIDER_REGISTRY 注册表

**文件**：`hermes_cli/auth.py:159-445`

```python
@dataclass
class ProviderConfig:
    """Describes a known inference provider."""
    id: str
    name: str
    auth_type: str  # "oauth_device_code", "oauth_external", "api_key"
    api_key_env_vars: tuple = ()      # API key 环境变量
    base_url_env_var: str = ""        # base URL 覆盖变量

PROVIDER_REGISTRY: Dict[str, ProviderConfig] = {
    "openai-api": ProviderConfig(
        auth_type="api_key",
        api_key_env_vars=("OPENAI_API_KEY",),
    ),
    "anthropic": ProviderConfig(
        auth_type="api_key",
        api_key_env_vars=("ANTHROPIC_API_KEY",),
    ),
    # ... 40+ providers
}

# 插件自动扩展 — 从 providers 插件目录自动注册
try:
    from providers import list_providers as _list_providers_for_registry
    for _pp in _list_providers_for_registry():
        if _pp.name in PROVIDER_REGISTRY: continue
        PROVIDER_REGISTRY[_pp.name] = ProviderConfig(...)
except Exception: pass
```

#### 2.2 api_mode 桥接 dispatch

**文件**：`agent/chat_completion_helpers.py:372-432`

```python
def _dispatch_nonstreaming_api_request(agent, api_kwargs: dict, *, make_client):
    """per-api_mode dispatch — codex / anthropic / bedrock / OpenAI-compatible"""
    if agent.api_mode == "codex_responses":
        return agent._run_codex_stream(api_kwargs, client=request_client, ...)
    if agent.api_mode == "anthropic_messages":
        return agent._anthropic_messages_create(api_kwargs, client=request_client)
    if agent.api_mode == "bedrock_converse":
        client = _get_bedrock_runtime_client(region)
        return normalize_converse_response(raw_response)
    # 默认：chat_completions (OpenAI-compatible)
    return request_client.chat.completions.create(**api_kwargs)
```

#### 2.3 推理回调处理

**文件**：`agent/chat_completion_helpers.py:1264-1280`

```python
if reasoning_text and agent.reasoning_callback:
    # Skip callback when streaming is active — reasoning was already
    # displayed during the stream via _fire_reasoning_delta
    if not agent.stream_delta_callback and not agent._stream_callback:
        try:
            agent.reasoning_callback(reasoning_text)
        except Exception:
            pass
```

#### 2.4 后台线程 API 调用

**文件**：`agent/chat_completion_helpers.py:506-631`

```python
def interruptible_api_call(agent, api_kwargs: dict):
    """Run API call in background thread for interrupt responsiveness."""
    # 每个 request 有独立的 OpenAI client，线程隔离
    def _call():
        result["response"] = _dispatch_nonstreaming_api_request(...)
    t = threading.Thread(target=_call, daemon=True)
    t.start()
    while t.is_alive():
        t.join(timeout=0.3)
        # 每 ~30s 更新 UI 活动状态
    return result["response"]
```

---

### 3. LangChain（Python）— 独立包 + init_chat_model 工厂

**核心模式**：每个 provider 独立包 + `init_chat_model("provider:model")` 工厂函数

#### 3.1 _BUILTIN_PROVIDERS 注册表

**文件**：`libs/langchain_v1/langchain/chat_models/base.py:38-78`

```python
_BUILTIN_PROVIDERS: dict[str, tuple[str, str, Callable]] = {
    "anthropic": ("langchain_anthropic", "ChatAnthropic", _call),
    "azure_openai": ("langchain_openai", "AzureChatOpenAI", _call),
    "deepseek": ("langchain_deepseek", "ChatDeepSeek", _call),
    "openai": ("langchain_openai", "ChatOpenAI", _call),
}
```

#### 3.2 Provider 自动推断

**文件**：`libs/langchain_v1/langchain/chat_models/base.py:523-596`

```python
def _attempt_infer_model_provider(model_name: str) -> str | None:
    model_lower = model_name.lower()
    if any(model_lower.startswith(pre) for pre in ("gpt-", "o1", "o3", "chatgpt")):
        return "openai"
    if model_lower.startswith("claude"):
        return "anthropic"
    if model_lower.startswith("gemini"):
        return "google_vertexai"
    if model_lower.startswith(("mistral", "mixtral")):
        return "mistralai"
    return None
```

#### 3.3 `base_url` 万能适配

**文件**：`libs/partners/openai/langchain_openai/chat_models/base.py:703-715`

```python
openai_api_base: str | None = Field(default=None, alias="base_url")
"""Base URL path for API requests, leave blank if not using a proxy or service emulator.

Resolution order (first match wins):
1. Explicit `base_url` (or `openai_api_base`) kwarg.
2. Env var `OPENAI_API_BASE` (read by LangChain at init).
3. Env var `OPENAI_BASE_URL` (read by the underlying `openai` SDK client).
"""
```

#### 3.4 Reasoning 内容提取

**文件**：`libs/core/langchain_core/messages/base.py:24-44`

```python
def _extract_reasoning_from_additional_kwargs(message: BaseMessage) -> ReasoningContentBlock | None:
    """Extract `reasoning_content` from `additional_kwargs`.
    Handles: Ollama, DeepSeek, XAI, Groq etc.
    """
    additional_kwargs = getattr(message, "additional_kwargs", {})
    reasoning_content = additional_kwargs.get("reasoning_content")
    if reasoning_content is not None and isinstance(reasoning_content, str):
        return {"type": "reasoning", "reasoning": reasoning_content}
    return None
```

---

### 4. LangChainGo（Go）— Option 构造函数模式

**核心模式**：Go 接口 + 函数选项模式（Functional Options），与 InferGlow 技术栈最接近

#### 4.1 Model 接口定义

**文件**：`langchaingo-extract/langchaingo-main/llms/llms.go:14-29`

```go
// Model is an interface multi-modal models implement.
type Model interface {
	GenerateContent(ctx context.Context, messages []MessageContent, options ...CallOption) (*ContentResponse, error)
	Call(ctx context.Context, prompt string, options ...CallOption) (string, error)
}

// ReasoningModel extends Model with reasoning capability.
type ReasoningModel interface {
	Model
	SupportsReasoning() bool
}
```

#### 4.2 CallOption + StreamingFunc

**文件**：`langchaingo-extract/langchaingo-main/llms/options.go:200-204`

```go
type CallOptions struct {
	StreamingFunc func(ctx context.Context, chunk []byte) error `json:"-"`
}

func WithStreamingFunc(streamingFunc func(ctx context.Context, chunk []byte) error) CallOption {
	return func(o *CallOptions) { o.StreamingFunc = streamingFunc }
}
```

#### 4.3 OpenAI Option 模式

**文件**：`langchaingo-extract/langchaingo-main/llms/openai/openaillm_option.go:63-148`

```go
type Option func(*options)

func WithToken(token string) Option {
	return func(opts *options) { opts.token = token }
}
func WithModel(model string) Option {
	return func(opts *options) { opts.model = model }
}
func WithBaseURL(baseURL string) Option {
	return func(opts *options) { opts.baseURL = baseURL }
}
```

#### 4.4 New 构造函数

**文件**：`langchaingo-extract/langchaingo-main/llms/openai/openaillm.go:80-96`

```go
var (
	_ llms.Model          = (*LLM)(nil)  // 编译期接口实现检查
	_ llms.ReasoningModel = (*LLM)(nil)
)

func New(opts ...Option) (*LLM, error) {
	opt, c, err := newClient(opts...)
	if err != nil {
		return nil, err
	}
	return &LLM{
		client:           c,
		CallbacksHandler: opt.callbackHandler,
		model:            c.Model,
	}, err
}
```

---

### 五、关键代码摘录对比

| 模式 | OpenClaw | Hermes | LangChain | LangChainGo | InferGlow 映射 |
|------|---------|---------|-----------|-------------|:---:|
| **注册模式** | Plugin hooks | PROVIDER_REGISTRY + 插件扩展 | 独立包 + 工厂 | Interface + Option | Registry + Factory |
| **推理处理** | `resolveReasoningOutputMode` hook | `reasoning_callback` | `_extract_reasoning_from_additional_kwargs` | 无统一抽象 | G1-02 统一映射 |
| **流式处理** | `wrapStreamFn` / `createStreamFn` | `interruptible_api_call` + background thread | `AIMessageChunk` 迭代 | `StreamingFunc` callback | SSE channel |
| **配置驱动** | manifest + 30+ hooks | YAML + env vars | `init_chat_model("provider:model")` | env vars + constructor | DEFAULT_SETTINGS |
| **万能适配** | `normalizeConfig` hook | `api_mode` bridge | `base_url` | `WithBaseURL()` | `OpenAICompatibleProvider` |

---

## 不同协议类型的特征概述与统一适配方案

> 20 家 Provider 虽然实现各异，但核心差异可归类为 5 种**协议类型**。
> 每种类型有对应的适配策略，**共性在于：所有差异都在配置层解决，不修改 Provider 核心逻辑。**

### 协议类型分类

```
                    ┌─────────────────────────────────────────────┐
                    │         统一 Provider 接口（Provider）         │
                    │   Generate() / Stream() / Chat() 统一契约     │
                    └──────────┬──────────────┬──────────┬────────┘
                               │              │          │
            ┌──────────────────┘              │          │
            ▼                                 │          │
   ┌─────────────────┐                        │          │
   │ Type A: OpenAI  │      ┌─────────────────┤          │
   │   兼容协议       │      │                │          │
   │   （主流 13 家）  │      │   ┌────────────┴────┐    │
   └─────────────────┘      │   │  Type B: Anthropic│    │
            │               │   │  兼容协议（5 家）   │    │
            │               │   └───────────────────┘    │
            ▼               │            │               │
   ┌─────────────────┐      │            │               │
   │ base_url 决定    │      │   ┌────────┴────────┐     │
   │ 路径前缀差异     │      │   │  Type C: 特殊路径 │     │
   │  无需代码修改     │      │   │  变体（已在A/B内） │     │
   └─────────────────┘      │   └─────────────────┘     │
                              │                          │
                            ┌─┴──────────────────────────┴──┐
                            │  统一归一化层（Normalize）       │
                            │  • reasoning_content → Reasoning │
                            │  • thinking → Reasoning          │
                            │  • <think> → Reasoning          │
                            │  • Quirks 修复                   │
                            └─────────────────────────────────┘
```

### 类型一：OpenAI 兼容（主流 13 家）

**涵盖 Provider**：OpenAI、Ollama、DeepSeek、Qwen、GLM、Kimi、MiMo、腾讯混元、Volcengine、01.AI、MiniMax、SiliconFlow、讯飞星火

**共同特征**：
- 请求体：`{"model": "...", "messages": [...], "max_tokens": ...}`
- 响应体：`{"choices": [{"delta": {"role": "assistant", "content": "..."}}]}`
- SSE 流：`data: {"choices": [{"delta": {...}}]}`

**适配方法**：`OpenAICompatibleProvider` 统一处理
- **唯一的差异是 `base_url`**，在 `DEFAULT_SETTINGS` 中配置即可
- 部分 Provider 使用 `reasoning_content`（非标准 `reasoning`），G1-02 统一处理

```
差异点                        处理方式
─────────────────────────────────────────
base_url 路径前缀不同          DEFAULT_SETTINGS 中配置不同值
推理字段名不同（reasoning vs   G1-02 同时读取，统一映射到 Reasoning
      reasoning_content）
思考参数名不同                 G1-03 通过 Options 透传
API Key 格式不同               本质都是 Bearer Token，无需特殊处理
```

### 类型二：Anthropic 兼容（5 家）

**涵盖 Provider**：Anthropic、MiMo Anthropic 端点、阶跃星辰 Anthropic 端点、商汤 Anthropic 端点

**共同特征**：
- 请求体：`{"model": "...", "messages": [...], "max_tokens": ..., "system": "..."}`
- 响应体：`{"content": [{"type": "text", "text": "..."}, {"type": "thinking", "thinking": "..."}]}`
- SSE 流：`data: {"type": "content_block_delta", "delta": {"type": "text_delta", "text": "..."}}`

**适配方法**：`AnthropicCompatibleProvider` 统一处理
- 推理内容在 `thinking` 字段，归一化为 `Reasoning`
- 所有 Anthropic 兼容端点的 SSE 格式完全一致

```
差异点                        处理方式
─────────────────────────────────────────
base_url 不同                 DEFAULT_SETTINGS 中配置不同值
max_tokens 上限不同            Anthropic 端点上限较低（~1024-4096），配置中调整
```

### 类型三：路径/域名变体（已在类型一/二中）

**涵盖 Provider**：百度千帆（`/v2`）、讯飞星火（`/agent/v1/`）、阶跃星辰（`/step_plan`）、商汤（`token.sensenova.cn`）

**共同特征**：这些 Provider 同时提供 OpenAI 和 Anthropic 双协议端点，但路径或域名有差异。

**适配方法**：
- 不需要新的 Provider 实现
- 只需在 `DEFAULT_SETTINGS` 中为每个协议端点添加一个配置项
- 路径差异体现在 `base_url` 中

```
Provider        差异类型          base_url                      归属协议
─────────────────────────────────────────────────────────────────────
百度千帆        路径版本          qianfan.baidubce.com/v2        OpenAI
讯飞星火        路径前缀          spark-api-open.xf-yun.com/agent/v1/
阶跃星辰        路径前缀          api.stepfun.com/v1             OpenAI
阶跃星辰        路径前缀          api.stepfun.com/step_plan      Anthropic
商汤            不同域名          token.sensenova.cn/v1          OpenAI
商汤            不同域名          token.sensenova.cn/v1/messages Anthropic
```

### 类型四：原生接口（不接入）

**涵盖 Provider**：商汤原生接口、百度千帆旧版 RPC、讯飞星火 WebSocket

**为什么排除**：
- **商汤原生**：请求体格式完全不同（`max_new_tokens` 而非 `max_tokens`，`thinking.enabled` 而非 `reasoning_effort`），且 OpenAI 兼容端点已提供相同能力
- **百度千帆旧版**：已弃用，用新版 `/v2` 即可
- **讯飞星火 WebSocket**：Go HTTP 客户端无法处理 WS 协议，且 HTTP 兼容端点可用

```
策略：只接入 OpenAI 兼容端点，不接入原生接口
```

### 类型五：推理内容特殊格式（归一化处理）

**涵盖 Provider**：DeepSeek、商汤、MiMo 等

**共同特征**：推理内容不在标准 `reasoning` 字段，需归一化。

```
格式                        Provider 示例              归一化方案
───────────────────────────────────────────────────────────────
reasoning_content 字段      MiMo、星火、商汤            G1-02 同时读取，合并到 Reasoning
reasoning 字段              标准 API                    G1-02 直接读取
thinking 字段               Anthropic 兼容端点          AnthropicCompatibleProvider 处理
content 中包 <think> 标签    DeepSeek-R1/V4、商汤原生   G1-04 normalizeThinkingTags()
reasoning_effort 参数       阶跃星辰、商汤              G1-03 Options 透传
thinking.type 参数          MiMo                      G1-03 Options 透传
```

---

### 统一适配方法论总结

```
新增一个 Provider 的适配流程：

1. 检查 API 类型
   ├─ OpenAI 兼容 → 只需加 DEFAULT_SETTINGS 配置项
   ├─ Anthropic 兼容 → 同上 + 确保 AnthropicCompatibleProvider 存在
   └─ 原生接口 → 不接入，排除

2. 检查推理字段
   ├─ 使用 reasoning → 无额外工作
   ├─ 使用 reasoning_content → G1-02 已覆盖
   ├─ 使用 thinking → AnthropicCompatibleProvider 已覆盖
   └─ 使用  <think> 标签 → G1-04 已覆盖

3. 检查思考参数
   ├─ 使用 reasoning_effort → G1-03 Options 透传
   ├─ 使用 thinking.type → G1-03 Options 透传
   └─ 无特殊参数 → 无额外工作

结论：20 家 Provider，核心代码只需修改以下几处：
   • config.go — DEFAULT_SETTINGS（配置）
   • openai.go — processOpenAILine（G1-02 reasoning_content）
   • think_normalizer.go — LeadingThinkNormalizer（G1-04/P1 流式标签归一化）
   • url_resolver.go — ResolveURL（P1 full_url 覆盖）
   • content_mapping.go — ExtractByPath（P1 content_mapping 非标字段路径）
   • openai_responses.go — OpenAIResponsesProvider（P1 Responses API）
```
