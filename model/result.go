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
