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
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// PromptTokensDetails 携带 prompt 端的细分计费信息，例如
	// {"cached_tokens": 100}。部分 Provider（OpenAI/MiMo 等）会返回该字段。
	// 为 nil 表示 Provider 未返回。
	PromptTokensDetails map[string]int `json:"prompt_tokens_details,omitempty"`
	// CompletionTokensDetails 携带 completion 端的细分计费信息，例如
	// {"reasoning_tokens": 500}。MiMo/OpenRouter 等 Provider 通过此字段
	// 单独报告推理 token 计数，用于推理 token 单独计费（G1-06）。
	// 为 nil 表示 Provider 未返回。
	CompletionTokensDetails map[string]int `json:"completion_tokens_details,omitempty"`
}

// ReasoningTokens 返回 completion 端的 reasoning_tokens 计数。
// 若 Provider 未返回 CompletionTokensDetails 或其中无 reasoning_tokens 键，
// 返回 0。便于上层直接读取推理 token 计费信息。
func (u *UsageInfo) ReasoningTokens() int {
	if u == nil || u.CompletionTokensDetails == nil {
		return 0
	}
	return u.CompletionTokensDetails["reasoning_tokens"]
}
