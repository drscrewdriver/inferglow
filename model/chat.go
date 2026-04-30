package model

// ChatMessage 表示一条聊天消息
type ChatMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content,omitempty"`
	Name      string    `json:"name,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ToolDefinition 定义一个 LLM 可调用的工具
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolCall 表示一次工具调用
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// UsageInfo 记录 token 使用情况
type UsageInfo struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}
