package model

import "encoding/json"

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
	Arguments map[string]any `json:"-"`
}

// toolCallWire is the on-wire shape: arguments is a JSON STRING (per OpenAI /
// Anthropic API spec), not a JSON object. M-MEDIUM-8.
type toolCallWire struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// MarshalJSON serializes ToolCall so that Arguments becomes a JSON string on
// the wire (matching OpenAI / Anthropic tool_calls[].function.arguments).
// M-MEDIUM-8: arguments as JSON string, not JSON object.
func (t ToolCall) MarshalJSON() ([]byte, error) {
	args := "{}"
	if t.Arguments != nil {
		b, err := json.Marshal(t.Arguments)
		if err != nil {
			return nil, err
		}
		args = string(b)
	}
	return json.Marshal(toolCallWire{
		ID:        t.ID,
		Name:      t.Name,
		Arguments: args,
	})
}

// UnmarshalJSON deserializes ToolCall, accepting both the wire form
// (arguments as JSON string) and the loose form (arguments as JSON object)
// for backward compatibility. M-MEDIUM-8.
func (t *ToolCall) UnmarshalJSON(data []byte) error {
	var wire toolCallWire
	if err := json.Unmarshal(data, &wire); err == nil && wire.Arguments != "" {
		// First try: arguments is a JSON string (canonical wire form).
		t.ID = wire.ID
		t.Name = wire.Name
		var args map[string]any
		if err := json.Unmarshal([]byte(wire.Arguments), &args); err != nil {
			// arguments string isn't a JSON object — keep raw form.
			args = map[string]any{"_raw": wire.Arguments}
		}
		t.Arguments = args
		return nil
	}
	// Fallback: arguments may be a JSON object (loose form).
	var loose struct {
		ID        string         `json:"id"`
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(data, &loose); err != nil {
		return err
	}
	t.ID = loose.ID
	t.Name = loose.Name
	t.Arguments = loose.Arguments
	return nil
}

// UsageInfo 记录 token 使用情况
type UsageInfo struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}
