# pi-ai 参考：effort 档位设置 + Provider 差距清单

> 数据源：`@earendil-works/pi-ai` 0.82.1（deepseek-harness rc.2 内置），从 `dist/providers/data/*.json` 与 `dist/api/openai-completions.js` 提取。
>
> **移植状态（2026-08-25）**：wire 翻译层已移植到 `model/effort.go`（10+ format）；provider 数据已生成到
> `model/provider_profiles_gen.go`（23 个生成 + 8 个手写核心）；`/effort` 档位展示按模型收紧；
> **Google 原生协议已实现**（`model/google.go`：streamGenerateContent + thinkingConfig + thought part 分离）；
> 新增 provider 共 12 个（google/mistral/groq/xai/together/zai/moonshotai/nvidia/cerebras/huggingface/fireworks/qwen-token-plan-cn）。
> 本文件保留为数据源参考，供后续扩展与核对。

## 一、effort 档位全集（7 档语义尺度）

pi-ai 用统一 7 档 ThinkingLevel（升序）：`off → minimal → low → medium → high → xhigh → max`。
每个模型用 `thinkingLevelMap` 声明「支持哪些档 + 每档发到线缆上的实际值」：

| 档位 | 语义 | 说明 |
|---|---|---|
| `off` | 关闭思考 | 多数 provider 即「不传参」；也有传 `"none"` 或 `thinking:{type:disabled}` |
| `minimal` | 最小思考 | OpenAI o 系 / Gemini3 部分 / Cerebras 等支持；值可为 `minimal`/`MINIMAL` 或折叠为 `low` |
| `low` | 低 | |
| `medium` | 中 | |
| `high` | 高 | 绝大多数 reasoning 模型的默认顶档 |
| `xhigh` | 超高 | 新增档，OpenAI gpt-5.3+ / Claude opus-5 / Bedrock 等 |
| `max` | 最强 | Anthropic 全 adaptive-thinking 模型、DeepSeek v4、部分 OpenAI gpt-5.6 |

**关键语义**：`thinkingLevelMap` 里某档为 `null` = 该档位**不可用/不发送**；为字符串 = 发送该 wire 值；
某档**缺失** = 该模型**不提供**该语义档。`off` 单档可能留空值（`off:`），表示「支持关闭 = 不传参」。

## 二、10 种 thinkingFormat 的 wire 参数格式（reasoning 参数长什么样）

| format | 用途 | wire 参数 |
|---|---|---|
| `openai` | OpenAI 兼容（默认） | `reasoning_effort: "low|medium|high|..."` |
| `deepseek` | DeepSeek 官方（DSH `llm-deepseek` 权威实现） | `thinking: {type:"enabled"\|disabled}` + `reasoning_effort:"low\|high\|max"`（仅 off/low/high/max 合法） |
| `openrouter` | OpenRouter 聚合 | `reasoning: {effort:"low\|..."}` |
| `together` | Together AI | `reasoning: {enabled:bool}` + `reasoning_effort` |
| `zai` | 智谱 Z.AI / GLM | `thinking:{type:enabled\|disabled}` + `reasoning_effort` |
| `qwen` | 通义 Qwen | `enable_thinking: bool` |
| `qwen-chat-template` | Qwen 网关 | `chat_template_kwargs:{enable_thinking,bool,preserve_thinking:true}` |
| `chat-template` | 自定义模板网关 | `chat_template_kwargs`（可含 `$var` 占位） |
| `string-thinking` | 顶层 thinking 字符串 | `thinking: "low|..."` |
| `ant-ling` | 蚂蚁 Ling/Ring | `reasoning: {effort:"high\|..."}`（仅 effort 非空时） |

**off 的分发**：多数格式里 `off`=不传 reasoning 参数（默认行为）；`deepseek` 传 `thinking:{type:"disabled"}`；
`openrouter`/`string-thinking` 传 `reasoning:{effort:"none"}` / `thinking:"none"`。

## 三、各 provider 实际支持的档位（thinkingLevelMap 摘录）

（✗=该档显式不可用；缺省=模型不声明该档；`-`=无 thinkingLevelMap）

### deepseek

> ⚠️ **两套数据源**：pi-ai 静态目录（`data/deepseek.json`，旧/保守）只声明 `high/max`；
> **DSH 官方 `llm-deepseek` 适配器（权威）** 用 `off/low/high/max`（无 medium），与我们的内置表一致。

- `deepseek-v4-flash` / `deepseek-v4-pro`（**DSH `llm-deepseek` 适配器，权威**）→ **efforts:** [off, low, high, max]（defaultEffort=high）
- wire 映射（`packages/llm/llm-deepseek/src/serialize.ts`）：`off` → `thinking:{type:"disabled"}`（不传 reasoning_effort）；`low`/`high`/`max` → `thinking:{type:"enabled"}` + `reasoning_effort:"low"|"high"|"max"`。仅这 4 档合法，其他档直接 `UNSUPPORTED_REASONING_EFFORT`。
- （对比）pi-ai 静态目录：`deepseek-v4-flash`/`pro` → [high='high', max='max']（minimal/low/medium 显式✗）


### openai

- `gpt-5.4` (api=openai-responses) → **efforts:** [off='none', minimal=✗, low='low', medium='medium', high='high', xhigh='xhigh', max=✗]

- `gpt-5.6-luna` (api=openai-responses) → **efforts:** [off='none', minimal=✗, low='low', medium='medium', high='high', xhigh='xhigh', max='max']

- `o3` (api=openai-responses) → **efforts:** [off=✗, minimal=✗, low='low', medium='medium', high='high', xhigh=✗, max=✗]

- `gpt-5` (api=openai-responses) → **efforts:** [off=✗, minimal='minimal', low='low', medium='medium', high='high', xhigh=✗, max=✗]


### anthropic

- `claude-opus-4-7` (api=anthropic-messages) → **efforts:** [xhigh='xhigh', max='max']

- `claude-sonnet-5` (api=anthropic-messages) → **efforts:** [xhigh='xhigh', max='max']


### google

- `gemini-3.1-pro-preview` (api=google-generative-ai) → **efforts:** [off=✗, minimal=✗, low='LOW', medium=✗, high='HIGH']

- `gemma-4-31b-it` (api=google-generative-ai) → **efforts:** [off=✗, minimal='MINIMAL', low=✗, medium=✗, high='HIGH']


### zai

- `glm-5.2` (api=openai-completions) → **efforts:** [minimal=✗, low='high', medium='high', high='high', max='max']


### moonshotai

- `kimi-k3` (api=openai-completions) → **efforts:** [off=✗, minimal=✗, low='low', medium=✗, high='high', xhigh=✗, max='max']


### openrouter

- `deepseek/deepseek-v4-flash` (api=openai-completions) → **efforts:** [minimal=✗, low=✗, medium=✗, high='high', xhigh='xhigh', max=✗]

- `openai/gpt-5.6-luna` (api=openai-completions) → **efforts:** [xhigh='xhigh', max='max']


## 四、Provider 差距清单（pi-ai 支持 vs inferglow 已实现）

### inferglow 已内置的 Provider（22 个）

`anthropic, baidu, deepseek, glm, kimi, mimo, mimo_anthropic, minimax, ollama, openai, openai_responses, openrouter, qwen, sensenova, sensenova_anthropic, siliconflow, spark, stepfun, stepfun_anthropic, tencent, volcengine, zeroone`

### pi-ai 有、inferglow 尚未内置的 Provider（含默认 base_url）

| provider | 默认 base_url | 示例模型 |
|---|---|---|
| `amazon-bedrock` | `https://bedrock-runtime.us-east-1.amazonaws.com` | amazon.nova-2-lite-v1:0, amazon.nova-lite-v1:0, amazon.nova-micro-v1:0, amazon.nova-pro-v1:0 |
| `ant-ling` | `https://api.ant-ling.com/v1` | Ling-2.6-1T, Ling-2.6-flash, Ring-2.6-1T |
| `azure-openai-responses` | `-` | gpt-4, gpt-4-turbo, gpt-4.1, gpt-4.1-mini |
| `cerebras` | `https://api.cerebras.ai/v1` | gemma-4-31b, gpt-oss-120b, zai-glm-4.7 |
| `cloudflare-ai-gateway` | `https://gateway.ai.cloudflare.com/v1/{CLOUDFLARE_ACCOUNT_ID}/{CLOUDFLARE_GATEWAY_ID}/anthropic` | claude-3-5-haiku, claude-3-haiku, claude-3-opus, claude-3-sonnet |
| `cloudflare-workers-ai` | `https://api.cloudflare.com/client/v4/accounts/{CLOUDFLARE_ACCOUNT_ID}/ai/v1` | @cf/google/gemma-4-26b-a4b-it, @cf/ibm-granite/granite-4.0-h-micro, @cf/meta/llama-3.3-70b-instruct-fp8-fast, @cf/meta/llama-4-scout-17b-16e-instruct |
| `fireworks` | `https://api.fireworks.ai/inference` | accounts/fireworks/models/deepseek-v4-flash, accounts/fireworks/models/deepseek-v4-pro, accounts/fireworks/models/glm-5p1, accounts/fireworks/models/gpt-oss-120b |
| `github-copilot` | `https://api.individual.githubcopilot.com` | claude-haiku-4.5, claude-opus-4.5, claude-opus-4.6, claude-opus-4.7 |
| `google` | `https://generativelanguage.googleapis.com/v1beta` | deep-research-max-preview-04-2026, deep-research-preview-04-2026, gemini-2.0-flash, gemini-2.0-flash-lite |
| `google-vertex` | `https://{location}-aiplatform.googleapis.com` | gemini-2.5-flash, gemini-2.5-flash-lite, gemini-2.5-pro, gemini-3-flash-preview |
| `groq` | `https://api.groq.com/openai/v1` | llama-3.1-8b-instant, llama-3.3-70b-versatile, meta-llama/llama-4-scout-17b-16e-instruct, openai/gpt-oss-120b |
| `huggingface` | `https://router.huggingface.co/v1` | MiniMaxAI/MiniMax-M2, MiniMaxAI/MiniMax-M2.1, MiniMaxAI/MiniMax-M2.5, MiniMaxAI/MiniMax-M2.7 |
| `kimi-coding` | `https://api.kimi.com/coding` | k3, k3-256k, kimi-for-coding, kimi-for-coding-highspeed |
| `minimax-cn` | `https://api.minimaxi.com/anthropic` | MiniMax-M2.7, MiniMax-M2.7-highspeed, MiniMax-M3 |
| `mistral` | `https://api.mistral.ai` | codestral-latest, devstral-2512, devstral-latest, devstral-medium-2507 |
| `moonshotai` | `https://api.moonshot.ai/v1` | kimi-k2-0711-preview, kimi-k2-0905-preview, kimi-k2-thinking, kimi-k2-thinking-turbo |
| `moonshotai-cn` | `https://api.moonshot.cn/v1` | kimi-k2-0711-preview, kimi-k2-0905-preview, kimi-k2-thinking, kimi-k2-thinking-turbo |
| `nvidia` | `https://integrate.api.nvidia.com/v1` | meta/llama-3.1-70b-instruct, meta/llama-3.1-8b-instruct, meta/llama-3.2-11b-vision-instruct, meta/llama-3.2-90b-vision-instruct |
| `openai-codex` | `https://chatgpt.com/backend-api` | gpt-5.3-codex-spark, gpt-5.4, gpt-5.4-mini, gpt-5.5 |
| `opencode` | `https://opencode.ai/zen` | claude-fable-5, claude-haiku-4-5, claude-opus-4-1, claude-opus-4-5 |
| `opencode-go` | `https://opencode.ai/zen/go` | minimax-m3, qwen3.7-max, qwen3.7-plus, deepseek-v4-flash |
| `qwen-token-plan` | `https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1` | MiniMax-M2.5, deepseek-v3.2, deepseek-v4-flash, deepseek-v4-pro |
| `qwen-token-plan-cn` | `https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` | MiniMax-M2.5, deepseek-v3.2, deepseek-v4-flash, deepseek-v4-pro |
| `together` | `https://api.together.ai/v1` | MiniMaxAI/MiniMax-M2.7, MiniMaxAI/MiniMax-M3, Qwen/Qwen2.5-7B-Instruct-Turbo, Qwen/Qwen3.5-9B |
| `vercel-ai-gateway` | `https://ai-gateway.vercel.sh` | alibaba/qwen-3-14b, alibaba/qwen-3-235b, alibaba/qwen-3-30b, alibaba/qwen-3-32b |
| `xai` | `https://api.x.ai/v1` | grok-4.3, grok-build-0.1, grok-4.5 |
| `xiaomi` | `https://api.xiaomimimo.com/v1` | mimo-v2-flash, mimo-v2-omni, mimo-v2-pro, mimo-v2.5 |
| `xiaomi-token-plan-ams` | `https://token-plan-ams.xiaomimimo.com/v1` | mimo-v2-pro, mimo-v2.5, mimo-v2.5-pro |
| `xiaomi-token-plan-cn` | `https://token-plan-cn.xiaomimimo.com/v1` | mimo-v2-pro, mimo-v2.5, mimo-v2.5-pro |
| `xiaomi-token-plan-sgp` | `https://token-plan-sgp.xiaomimimo.com/v1` | mimo-v2-pro, mimo-v2.5, mimo-v2.5-pro |
| `zai` | `https://api.z.ai/api/coding/paas/v4` | glm-4.5-air, glm-4.7, glm-5-turbo, glm-5.1 |
| `zai-coding-cn` | `https://open.bigmodel.cn/api/coding/paas/v4` | glm-4.5-air, glm-4.7, glm-5-turbo, glm-5.1 |

### 备注

- `kimi`(moonshot.cn) inferglow 已有，但 pi-ai 的 `moonshotai`(api.moonshot.ai 国际) / `moonshotai-cn` / `kimi-coding`(K3 编程) 属不同入口。
- `glm`(bigmodel.cn 智谱开放平台) inferglow 已有，pi-ai 的 `zai`(Z.AI)/`zai-coding-cn` 是新版 GLM 入口。
- `mimo`(xiaomimimo.com) inferglow 已有，pi-ai 的 `xiaomi-token-plan-{cn,ams,sgp}` 是小米海外 token 套餐入口。
- 国内专属（inferglow 独有、pi-ai 无）：`baidu` `spark` `sensenova` `tencent` `volcengine` `zeroone` `siliconflow` `stepfun` `ollama`。
