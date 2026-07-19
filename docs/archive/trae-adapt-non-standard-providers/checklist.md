# Checklist

## DEFAULT_SETTINGS 配置完整性

- [x] `DEFAULT_SETTINGS["stepfun"]` 存在，base_url=`https://api.stepfun.com/v1`，model=`step-3.7-flash`
- [x] `DEFAULT_SETTINGS["stepfun_anthropic"]` 存在，base_url=`https://api.stepfun.com/step_plan`，max_tokens=1024
- [x] `DEFAULT_SETTINGS["baidu"]` 存在，base_url=`https://qianfan.baidubce.com/v2`（/v2 路径），model=`ernie-5.0`
- [x] `DEFAULT_SETTINGS["spark"]` 存在，base_url=`https://spark-api-open.xf-yun.com/agent/v1/`（含尾斜杠），model=`spark-x`
- [x] `DEFAULT_SETTINGS["sensenova"]` 存在，base_url=`https://token.sensenova.cn/v1`，model=`sensenova-6.7-flash-lite`
- [x] `DEFAULT_SETTINGS["sensenova_anthropic"]` 存在，base_url=`https://token.sensenova.cn/v1/messages`，max_tokens=1024
- [x] 现有 7 家 Provider 配置未被改动

## Factory 函数完整性

- [x] `NewStepFunProviderFromConfig` 存在并返回 `*OpenAICompatibleProvider`，`Name()`=="stepfun"
- [x] `NewStepFunAnthropicProviderFromConfig` 存在并返回 `*AnthropicCompatibleProvider`
- [x] `NewBaiduProviderFromConfig` 存在并返回 `*OpenAICompatibleProvider`，`Name()`=="baidu"
- [x] `NewSparkProviderFromConfig` 存在并返回 `*OpenAICompatibleProvider`，`Name()`=="spark"
- [x] `NewSenseNovaProviderFromConfig` 存在并返回 `*OpenAICompatibleProvider`，`Name()`=="sensenova"
- [x] `NewSenseNovaAnthropicProviderFromConfig` 存在并返回 `*AnthropicCompatibleProvider`
- [x] 每个 factory 在 APIKey 缺失时返回的错误 `errors.Is(err, ErrMissingRequiredConfig)` 为 true
- [x] 每个 factory 的错误信息包含对应 prefix（如 "load stepfun provider config: ..."）

## 测试覆盖

- [x] 6 个 factory 各有对应的单元测试
- [x] 测试覆盖默认配置加载（base_url/model 来自 DEFAULT_SETTINGS）
- [x] 测试覆盖 StaticConfigProvider 覆盖 base_url/model 场景
- [x] 测试覆盖 APIKey 缺失错误场景
- [x] 测试使用 StaticConfigProvider 提供 api_key，不依赖环境变量

## reasoning_content 字段解析（G1-02）

- [x] `openAIChunk.Delta` 结构体包含 `ReasoningContent *string json:"reasoning_content,omitempty"` 字段
- [x] `processOpenAILine` 在读取 `reasoning` 后读取 `reasoning_content`，后者存在时覆盖前者
- [x] `toStreamChunk` 向后兼容函数同步处理 `reasoning_content`
- [x] 测试覆盖：仅含 `reasoning_content`、两字段并存时优先级、仅含 `reasoning` 向后兼容
- [x] 现有 `openai_extra_test.go` 的 reasoning 测试无回归

## `<think>` 标签归一化（G1-04）

- [x] `hasThinkingTags(content string) bool` 函数存在并正确检测 `<think>...</think>` 标签
- [x] `normalizeThinkingTags(content string) (reasoning, cleaned string)` 函数存在并正确提取/清除标签
- [x] `BroadcastResponse` 在 `EventDone` 分支对 `ModelResponse` 做防御性归一化（仅当 `Reasoning` 为空时触发）
- [x] 测试覆盖：标签提取、无标签透传、BroadcastResponse 端到端归一化
- [x] `fullReasoning` 非空时不触发归一化（避免双重处理）

## 不修改 Provider 实现（更新）

- [x] `model/anthropic.go` 未被本次变更修改
- [x] `model/ollama.go` 未被本次变更修改
- [x] `model/openai.go` 的改动仅限：`openAIChunk.Delta` 新增字段、`processOpenAILine`/`toStreamChunk` 读取 `reasoning_content`、新增 `hasThinkingTags`/`normalizeThinkingTags` 函数、`BroadcastResponse` 调用归一化

## 编译与测试

- [x] `cd model && go build ./...` 成功
- [x] `cd model && go test ./... -timeout 120s` 全部通过，无回归
- [x] `cd model && go vet ./...` 无警告

## 验证结果摘要

- **配置层**：`config.go` 的 `DEFAULT_SETTINGS` 在 `kimi` 之后追加 6 个新条目，未改动原 7 家配置
- **Factory 层**：`provider_factory.go` 末尾追加 6 个 factory 函数，严格遵循 `NewKimiProviderFromConfig` 的代码风格
- **测试层**：新建 `provider_factory_nonstandard_test.go`，6 个 Test × 3 子测试（default_config / override_config / missing_api_key）= 18 个子测试全部 PASS
- **验证命令**：
  - `go build ./...` → exit 0
  - `go vet ./...` → exit 0，无警告
  - `go test ./... -timeout 120s` → `ok github.com/inferglow/model`
- **git diff**：仅 `model/config.go` 与 `model/provider_factory.go` 有修改，`model/openai.go` / `model/anthropic.go` / `model/ollama.go` 未改动（满足"不修改 Provider 实现代码"约束）
- **新增文件**：`model/provider_factory_nonstandard_test.go`（untracked）
