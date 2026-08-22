# 需求族：媒体能力门控（SupportsVision 类型定义与各级别穿透）

> 状态：**已交付 ✅**（2026-08-22 核查）
> 领域：`model/` 模型层
> 来源：从 [rich-input-composer-prd.md](../rich-input-composer-prd.md) §13.1 + §11.3 缺口 1 拆出（v0.4 拆分）
> 定位：本文件保留设计记录与验收（防回归）。TUI 侧体验（提交前预检提示）属 input 线，见 PRD §7 / 验收 10，不在本需求族范围。

## 背景

`SupportsVision(modelName)` 已存在于 [capability.go](file:///e:/test/rewrite-agently/inferglow/model/capability.go)，
内置 `ModelCapabilityRegistry`（含 `Vision/Audio/Video/ToolCalling/JSONMode/Streaming/MaxContext`），
覆盖 OpenAI/Anthropic/Gemini/DeepSeek/Qwen/GLM/Kimi/Cohere 全系。但在门控落地前，它从未在任何提交路径
被调用——「非 vision 模型收到图片」会把 base64 原样发给 provider 等其报 4xx，浪费 token 且错误信息不可读。

**交付状态**：`gateMultimodal` + `ErrUnsupportedContent` 落在
[content_block.go](file:///e:/test/rewrite-agently/inferglow/model/content_block.go)；
[openai.go](file:///e:/test/rewrite-agently/inferglow/model/openai.go) 与
[anthropic.go](file:///e:/test/rewrite-agently/inferglow/model/anthropic.go) 的
`GenerateRequestData` 均已穿透调用；验收由
[content_block_gate_test.go](file:///e:/test/rewrite-agently/inferglow/model/content_block_gate_test.go) 覆盖。

## 决策：门控收口在模型层 `GenerateRequestData`

- 每个实际序列化媒体内容的 provider（OpenAI 兼容、Anthropic 兼容）在自己
  `GenerateRequestData` 拼接 `ContentBlocks` **之前**调用统一的媒体门控函数。
- 疑问「为什么不放在 orchestrator/validator」：模型层是**兜底仲裁**，保证任何上层路径
  （CLI、server、其它调用方）都无法绕过；上层提交前的主动提示属于体验优化，不替代兜底。

## 穿透策略（避免误伤）

`ModelCapabilityRegistry` 只覆盖已知模型。若对未知模型保守拒绝，会误伤本地部署 / 聚合平台
（OpenRouter、SiliconFlow）/ 企业网关的任意自定义模型名。因此：

| 模型在注册表 | 能力 | 门控结果 |
|------------|------|---------|
| 已知 & `Vision=false` | `gpt-4`、`deepseek-chat`、`qwen-max` 等 | **拒绝**图片，返回 `ErrUnsupportedContent` + 可读消息 |
| 已知 & `Vision=true` | `gpt-4o/claude-3-*/gemini-2.x/qwen-vl-*/glm-4v` | 放行 |
| **未知** | 本地/自定义/聚合模型 | **放行**（交由上游裁决，不越权拦截） |

- 对 `ContentImage` 用 `Vision`；`ContentAudio`/`ContentVideo` 分别用 `Audio`/`Video` 能力位。
- 纯文本 `ContentBlocks`（`ContentText`）不触发门控。

## API 形状（已实现）

```go
// model 包内（已存在）
var ErrUnsupportedContent = errors.New("model does not support requested content")
func gateMultimodal(modelName string, blocks []ContentBlock) error // unexported
```

- `OpenAICompatibleProvider.GenerateRequestData`、`AnthropicCompatibleProvider.GenerateRequestData`
  在 `len(req.ContentBlocks) > 0` 时先调 `gateMultimodal`，失败直接 `return nil, err`。
- 嵌套封装 provider（`failover` / `pool` / `ratelimit_wrap`）不重复 gating——它们委托底层
  `GenerateRequestData`，天然透传；如需各封装的 log 观测，可在其文档标注「媒体门控由叶 provider 兜底」。

## 验收（已达成 ✅，`content_block_gate_test.go`）

1. `model` 单测：已知非 vision 模型 + `ImageBlock` → 返回 `ErrUnsupportedContent`。
2. 已知 vision 模型 + `ImageBlock` → 正常生成请求数据；纯文本永不门控。
3. 未知模型 + `ImageBlock` → 放行（不报错）。
4. OpenAI 兼容、Anthropic 兼容两 provider 集成测试各覆盖一条穿透路径
   （`TestOpenAIGenerateRequestData_ImageGate` 拒绝路径 / `TestAnthropicGenerateRequestData_ImageGate` 放行路径）。

## 关联

- input 线：PRD §7「图片附件 × 非 vision 模型」双层门控（TUI 预检提示 + 本族兜底）、验收 10。
- backlog：视觉桥(ask/vision) 与读屏/看图 agent 依赖本族 + input 线 P1 图片附件底座，
  见 [tui-capability-backlog.md](tui-capability-backlog.md)。
