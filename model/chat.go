// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package model

import "encoding/json"

// Role 表示聊天消息的角色类型。
type Role string

// 预定义的角色常量。
const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ChatMessage 表示一条聊天消息
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // for role="tool" messages
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

// toolCallWire is the on-wire shape following the OpenAI tool_calls envelope:
// {"type":"function","id":"...","function":{"name":"...","arguments":"..."}}
// Arguments is a JSON STRING (per OpenAI / Anthropic API spec), not a JSON object.
type toolCallWire struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// MarshalJSON serializes ToolCall in the OpenAI envelope format so that
// it can be sent back to the model in assistant messages.
func (t ToolCall) MarshalJSON() ([]byte, error) {
	args := "{}"
	if t.Arguments != nil {
		b, err := json.Marshal(t.Arguments)
		if err != nil {
			return nil, err
		}
		args = string(b)
	}
	var w toolCallWire
	w.Type = "function"
	w.ID = t.ID
	w.Function.Name = t.Name
	w.Function.Arguments = args
	return json.Marshal(w)
}

// UnmarshalJSON deserializes ToolCall, accepting both the OpenAI envelope
// form (function.name/function.arguments) and the flat form (name/arguments)
// for backward compatibility.
func (t *ToolCall) UnmarshalJSON(data []byte) error {
	// Try OpenAI envelope first.
	var w toolCallWire
	if err := json.Unmarshal(data, &w); err == nil && w.Function.Name != "" {
		t.ID = w.ID
		t.Name = w.Function.Name
		var args map[string]any
		if w.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(w.Function.Arguments), &args); err != nil {
				args = map[string]any{"_raw": w.Function.Arguments}
			}
		}
		t.Arguments = args
		return nil
	}
	// Fallback: flat form {id, name, arguments}.
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
