# Tasks

- [x] Task 1: 在 `model/config.go` 的 `DEFAULT_SETTINGS` 新增 6 个 Provider 配置项
  - [x] SubTask 1.1: 新增 `stepfun` 配置（model=step-3.7-flash, base_url=https://api.stepfun.com/v1, temperature=0.7, max_tokens=4096）
  - [x] SubTask 1.2: 新增 `stepfun_anthropic` 配置（model=step-3.7-flash, base_url=https://api.stepfun.com/step_plan, max_tokens=1024）
  - [x] SubTask 1.3: 新增 `baidu` 配置（model=ernie-5.0, base_url=https://qianfan.baidubce.com/v2, temperature=0.7, max_tokens=4096）
  - [x] SubTask 1.4: 新增 `spark` 配置（model=spark-x, base_url=https://spark-api-open.xf-yun.com/agent/v1/, temperature=0.7, max_tokens=4096）
  - [x] SubTask 1.5: 新增 `sensenova` 配置（model=sensenova-6.7-flash-lite, base_url=https://token.sensenova.cn/v1, temperature=0.7, max_tokens=4096）
  - [x] SubTask 1.6: 新增 `sensenova_anthropic` 配置（model=sensenova-6.7-flash-lite, base_url=https://token.sensenova.cn/v1/messages, max_tokens=1024）

- [x] Task 2: 在 `model/provider_factory.go` 新增 6 个 factory 函数
  - [x] SubTask 2.1: `NewStepFunProviderFromConfig` → `*OpenAICompatibleProvider`，ProviderName="stepfun"，复用 `LoadProviderConfig(cp, "stepfun")`
  - [x] SubTask 2.2: `NewStepFunAnthropicProviderFromConfig` → `*AnthropicCompatibleProvider`，复用 `LoadProviderConfig(cp, "stepfun_anthropic")`
  - [x] SubTask 2.3: `NewBaiduProviderFromConfig` → `*OpenAICompatibleProvider`，ProviderName="baidu"，注释说明 /v2 路径
  - [x] SubTask 2.4: `NewSparkProviderFromConfig` → `*OpenAICompatibleProvider`，ProviderName="spark"，注释说明 /agent/v1/ 前缀
  - [x] SubTask 2.5: `NewSenseNovaProviderFromConfig` → `*OpenAICompatibleProvider`，ProviderName="sensenova"，注释说明 Token Plan 域名
  - [x] SubTask 2.6: `NewSenseNovaAnthropicProviderFromConfig` → `*AnthropicCompatibleProvider`，复用 `LoadProviderConfig(cp, "sensenova_anthropic")`
  - [x] SubTask 2.7: 每个 factory 在 APIKey 缺失时返回 `fmt.Errorf("load <prefix> provider config: %w", err)` 包装错误

- [x] Task 3: 在 `model/provider_factory_test.go` 新增 6 组单元测试
  - [x] SubTask 3.1: `TestNewStepFunProviderFromConfig` — 验证默认 base_url、model、ProviderName="stepfun"；用 StaticConfigProvider 覆盖 base_url/model 后字段更新；APIKey 缺失时 `errors.Is(err, ErrMissingRequiredConfig)` 为 true
  - [x] SubTask 3.2: `TestNewStepFunAnthropicProviderFromConfig` — 验证 Anthropic 实例字段与默认 base_url=`https://api.stepfun.com/step_plan`
  - [x] SubTask 3.3: `TestNewBaiduProviderFromConfig` — 验证默认 base_url=`https://qianfan.baidubce.com/v2`（/v2 路径）、ProviderName="baidu"
  - [x] SubTask 3.4: `TestNewSparkProviderFromConfig` — 验证默认 base_url=`https://spark-api-open.xf-yun.com/agent/v1/`、ProviderName="spark"
  - [x] SubTask 3.5: `TestNewSenseNovaProviderFromConfig` — 验证默认 base_url=`https://token.sensenova.cn/v1`、ProviderName="sensenova"
  - [x] SubTask 3.6: `TestNewSenseNovaAnthropicProviderFromConfig` — 验证 Anthropic 实例默认 base_url=`https://token.sensenova.cn/v1/messages`
  - [x] SubTask 3.7: 所有测试使用 `StaticConfigProvider` 提供 `<prefix>.api_key`，避免依赖环境变量

- [x] Task 4: 验证全部变更
  - [x] SubTask 4.1: `cd model && go build ./...` 编译成功
  - [x] SubTask 4.2: `cd model && go test ./... -timeout 120s` 全部通过，无回归
  - [x] SubTask 4.3: `cd model && go vet ./...` 无警告
  - [x] SubTask 4.4: 确认 `openai.go` 与 `anthropic.go` 未被修改（git diff 为空）

- [x] Task 5: 在 `model/openai.go` 实现 `reasoning_content` 字段解析（G1-02）
  - [x] SubTask 5.1: 在 `openAIChunk.Delta` 结构体中新增 `ReasoningContent *string json:"reasoning_content,omitempty"` 字段（紧邻现有 `Reasoning` 字段）
  - [x] SubTask 5.2: 在 `processOpenAILine` 中，先读取 `c.Delta.Reasoning`，再读取 `c.Delta.ReasoningContent`（若存在则覆盖，体现 `reasoning_content` > `reasoning` 优先级）
  - [x] SubTask 5.3: 在 `toStreamChunk` 向后兼容函数中同步添加 `reasoning_content` 读取逻辑
  - [x] SubTask 5.4: 新增测试验证：仅含 `reasoning_content` 的 chunk、同时含两字段时 `reasoning_content` 优先、仅含 `reasoning` 向后兼容

- [x] Task 6: 在 `model/openai.go` 实现 `<think>` 标签归一化（G1-04）
  - [x] SubTask 6.1: 新增 `hasThinkingTags(content string) bool` 函数，检测 content 是否含 `<think>...</think>` 标签
  - [x] SubTask 6.2: 新增 `normalizeThinkingTags(content string) (reasoning string, cleaned string)` 函数，提取标签内容到 reasoning，从 content 清除标签并去除前导空白
  - [x] SubTask 6.3: 在 `BroadcastResponse` 的 `EventDone` 分支中，构造 `ModelResponse` 后检查：若 `resp.Reasoning == ""` 且 `hasThinkingTags(resp.Content)`，调用 `normalizeThinkingTags` 更新 `resp.Reasoning` 与 `resp.Content`
  - [x] SubTask 6.4: 新增测试验证：标签提取、无标签透传、多标签场景、BroadcastResponse 端到端归一化

- [x] Task 7: 验证 G1-02 与 G1-04 变更
  - [x] SubTask 7.1: `cd model && go build ./...` 编译成功
  - [x] SubTask 7.2: `cd model && go test ./... -timeout 120s` 全部通过，无回归
  - [x] SubTask 7.3: `cd model && go vet ./...` 无警告
  - [x] SubTask 7.4: 确认现有 reasoning 测试（openai_extra_test.go 等）无回归

# Task Dependencies

- Task 1 必须先完成（factory 函数依赖 DEFAULT_SETTINGS 中的配置项）
- Task 2 依赖 Task 1
- Task 3 依赖 Task 2
- Task 4 依赖 Task 1-3 全部完成
- Task 5 / Task 6 互相独立，可并行，但都依赖 Task 4 完成（确保基线稳定）
- Task 7 依赖 Task 5 与 Task 6 全部完成

# 完成情况说明

## 第一阶段（Task 1-4，配置层 + factory 适配）

- 4 个 Task 全部完成，所有 SubTask 已勾选
- Task 4 验证结果：
  - `go build ./...` 退出码 0
  - `go vet ./...` 退出码 0，无警告
  - `go test ./... -timeout 120s`：`ok github.com/inferglow/model` 全部通过
  - 新增 6 个 Test × 3 子测试 = 18 个子测试全部 PASS
  - `git diff --stat` 确认仅 `config.go` 与 `provider_factory.go` 被修改，`openai.go` / `anthropic.go` / `ollama.go` 未改动
- 新增文件：`model/provider_factory_nonstandard_test.go`（18 个子测试）
- 新增配置项：6 个（stepfun / stepfun_anthropic / baidu / spark / sensenova / sensenova_anthropic）
- 新增 factory 函数：6 个

## 第二阶段（Task 5-7，reasoning_content + think 标签归一化）

已完成。用户反馈：文档明确要求 3 家非标准 Provider（MiMo/讯飞星火/商汤）使用 `reasoning_content` 字段，且文档"方法论 D"给出实现代码，应纳入本次 spec 范围。

- Task 5（G1-02）完成：`openAIChunk.Delta` 新增 `ReasoningContent` 字段，`processOpenAILine` 与 `toStreamChunk` 均按 `reasoning_content` > `reasoning` 优先级读取
- Task 6（G1-04）完成：新增 `hasThinkingTags` / `normalizeThinkingTags` 函数，`BroadcastResponse` 在 `EventDone` 分支做防御性归一化（仅当 `Reasoning` 为空时触发）
- Task 7 验证通过：
  - `go build ./...` 退出码 0
  - `go vet ./...` 退出码 0，无警告
  - `go test ./... -timeout 120s`：`ok github.com/inferglow/model` 全部通过
  - 9 个新测试函数全部 PASS（5 个 reasoning_content 测试 + 4 个 think 标签测试）
  - 现有 `openai_extra_test.go` / `openai_bugfix_test.go` reasoning 测试无回归
- 额外修改：`medium_bugfix_test.go` 与 `openai_extra_test.go` 中 3 处内联匿名结构体定义需同步新增 `ReasoningContent` 字段（Go 匿名结构体类型严格匹配要求）
