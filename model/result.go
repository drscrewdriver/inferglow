package model

// ResultEventType 定义事件类型
type ResultEventType string

const (
	EventDelta       ResultEventType = "delta"
	EventDone        ResultEventType = "done"
	ErrorEvent       ResultEventType = "error"
	MetaEvent        ResultEventType = "meta"
	StatusEvent      ResultEventType = "status"
	ToolCallsEvent   ResultEventType = "tool_calls"
	ReasoningDelta   ResultEventType = "reasoning_delta"
	ReasoningDone    ResultEventType = "reasoning_done"
	OriginalDelta    ResultEventType = "original_delta"
	OriginalDone     ResultEventType = "original_done"
)

// ResultEvent 广播到事件流的统一事件
type ResultEvent struct {
	EventType ResultEventType
	Payload   any
}

// ReasoningTokenMeta 推理 token 元数据，作为 MetaEvent 的 Payload 出现。
// 当 Provider 在 usage.completion_tokens_details.reasoning_tokens 中报告
// 推理 token 计数时，BroadcastResponse 会发出此事件，便于上层单独追踪
// 推理计费（G1-06）。
type ReasoningTokenMeta struct {
	Count int
}
