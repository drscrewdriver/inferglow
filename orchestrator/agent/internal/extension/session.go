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

// Package extension implements the SessionExtension and ActionExtension
// adapters that the agent package uses to bridge the session and action
// modules. It is an internal implementation detail of the agent package; the
// agent package re-exports the public types and constructors via type aliases
// so existing callers are unaffected.
package extension

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// SessionExtension wraps a SessionBackend (either *session.Session or
// *session.ThreeZoneSession) and provides a simplified interface for the
// orchestrator to manage conversation history. When a persist path is set,
// the session is automatically saved to disk after every mutation (Add*
// call), ensuring crash-recovery of conversation state.
type SessionExtension struct {
	s           session.SessionBackend
	mu          sync.Mutex
	persistPath string // if set, auto-save after each mutation

	// fileCache tracks files already read during this session to avoid
	// accumulating duplicate file contents in the context window.
	// Key: file path, Value: bytes read on first occurrence.
	fileCache map[string]int
}

// NewSessionExtension creates a SessionExtension wrapping the given backend.
// The backend must be either *session.Session or *session.ThreeZoneSession
// (both implement session.SessionBackend).
func NewSessionExtension(s session.SessionBackend) *SessionExtension {
	return &SessionExtension{s: s, fileCache: make(map[string]int)}
}

// SetPersistPath configures automatic session persistence. After each
// Add* call the session is saved to the given path as JSON.
// This ensures conversation state survives daemon crashes.
func (e *SessionExtension) SetPersistPath(path string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.persistPath = path
}

// persist saves the session to disk if a persist path is configured.
// Called internally after every mutation.
func (e *SessionExtension) persist() {
	e.mu.Lock()
	path := e.persistPath
	e.mu.Unlock()
	if path == "" {
		return
	}
	// Best-effort: log errors are silently ignored to avoid disrupting the agent loop.
	e.s.SaveJSON(path)
}

// AddUserMessage adds a user message to the session history.
func (e *SessionExtension) AddUserMessage(content string) {
	e.s.AddMessage("user", content, "")
	e.persist()
}

// AddSystemMessage adds a system message to the session history.
// Used for agent-level nudges (e.g. stale-loop warnings, halfway warnings).
func (e *SessionExtension) AddSystemMessage(content string) {
	e.s.AddMessage("system", content, "")
	e.persist()
}

// AddAssistantMessage adds an assistant message to the session history.
func (e *SessionExtension) AddAssistantMessage(content string) {
	e.s.AddMessage("assistant", content, "")
	e.persist()
}

// AddAssistantMessageWithReasoning adds an assistant message to the session
// history, attaching reasoning content in the Meta map (G1-02). When the
// next round builds the prompt via PreparePrompt, the reasoning_content is
// extracted into model.ChatMessage.ReasoningContent so it is serialized
// into the provider request body — required by DeepSeek, MiMo, etc.
func (e *SessionExtension) AddAssistantMessageWithReasoning(content, reasoning string) {
	if reasoning == "" {
		e.AddAssistantMessage(content)
		return
	}
	e.s.AddMessageWithMeta("assistant", content, "", map[string]any{
		"reasoning_content": reasoning,
	})
	e.persist()
}

// SetMessageMasker installs (or clears, when m is nil) a PII/security
// masker on the underlying session. The masker is consulted by
// Session.AddMessageChecked to redact string content before it is
// appended to the history. This is the imperative bridge between the
// agent-level WithPIIMasker option and the session's masking hook.
//
// When the backend does not support masking (e.g. ThreeZoneSession),
// the backend's SetMessageMasker is a no-op stub, so this call has no
// effect — preserving the previous type-assertion behavior.
func (e *SessionExtension) SetMessageMasker(m session.MessageMasker) {
	e.s.SetMessageMasker(m)
}

// AddActionResult adds an action execution result as a text message to the session.
// A nil result is silently ignored (no message added) so callers cannot
// crash the orchestrator by passing a nil pointer after a failed action
// lookup.
//
// NOTE: For native function-calling flows, prefer AddToolResult which
// emits a proper role="tool" message with tool_call_id.
func (e *SessionExtension) AddActionResult(actionName string, result *action.ActionResult) {
	if result == nil {
		return
	}
	msg := fmt.Sprintf("Action %q executed: status=%s, result=%v", actionName, result.Status, result.Result)
	if result.Error != "" {
		msg = fmt.Sprintf("Action %q failed: %s", actionName, result.Error)
	}
	e.s.AddMessage("system", msg, "")
	e.persist()
}

// AddAssistantToolCalls records an assistant message that contains native
// tool calls. The tool calls are stored in the session message's Meta map
// under the key "tool_calls" so PreparePrompt can forward them to the model.
// The reasoning parameter stores the model's reasoning content (G1-02) so
// it can be passed back in the next round's request body.
func (e *SessionExtension) AddAssistantToolCalls(toolCalls []model.ToolCall, reasoning string) {
	meta := map[string]any{
		"tool_calls": toolCalls,
	}
	if reasoning != "" {
		meta["reasoning_content"] = reasoning
	}
	e.s.AddMessageWithMeta("assistant", "", "", meta)
	e.persist()
}

// AddToolResult records a tool execution result as a role="tool" message
// with the corresponding tool_call_id so the model can correlate results.
func (e *SessionExtension) AddToolResult(toolCallID string, content string) {
	e.s.AddMessageWithMeta("tool", content, "", map[string]any{
		"tool_call_id": toolCallID,
	})
	e.persist()
}

// AddToolResultNamed is like AddToolResult but also receives the tool name
// so it can deduplicate repeated file_read results. When the same file path
// has already been added to the session, the content is replaced with a
// short reference marker instead of the full (potentially large) content.
func (e *SessionExtension) AddToolResultNamed(toolCallID, toolName, content string) {
	if toolName == "file_read" {
		// Try to extract the path from the JSON content.
		var parsed struct {
			Path      string `json:"path"`
			BytesRead int    `json:"bytes_read"`
		}
		if err := json.Unmarshal([]byte(content), &parsed); err == nil && parsed.Path != "" {
			e.mu.Lock()
			if prev, ok := e.fileCache[parsed.Path]; ok {
				e.mu.Unlock()
				// Already in context — replace with a short reference.
				content = fmt.Sprintf("[already in context: %s, previously read %d bytes]", parsed.Path, prev)
			} else {
				e.fileCache[parsed.Path] = parsed.BytesRead
				e.mu.Unlock()
			}
		}
	}
	e.s.AddMessageWithMeta("tool", content, "", map[string]any{
		"tool_call_id": toolCallID,
	})
	e.persist()
}

// PreparePrompt returns the session's context window as chat messages.
// It extracts tool_calls and tool_call_id from session message Meta when
// present so native function-calling conversations round-trip correctly.
func (e *SessionExtension) PreparePrompt() []model.ChatMessage {
	history := e.s.PreparePrompt()
	prompt := make([]model.ChatMessage, len(history))
	for i, msg := range history {
		prompt[i] = model.ChatMessage{
			Role:    msg.Role,
			Content: fmt.Sprintf("%v", msg.Content),
			Name:    msg.Name,
		}
		// Extract tool_calls from Meta (assistant messages with tool calls).
		if tc, ok := msg.Meta["tool_calls"]; ok {
			if toolCalls, ok := tc.([]model.ToolCall); ok {
				prompt[i].ToolCalls = toolCalls
			}
		}
		// Extract tool_call_id from Meta (tool result messages).
		if tcid, ok := msg.Meta["tool_call_id"]; ok {
			if id, ok := tcid.(string); ok {
				prompt[i].ToolCallID = id
			}
		}
		// Extract reasoning_content from Meta (G1-02: DeepSeek/MiMo passback).
		// When the stored reasoning_content is empty (e.g. model returned no
		// reasoning but the field was recorded), substitute " " (space) to
		// avoid DeepSeek V4 400 errors. Following Hermes practice.
		if rc, ok := msg.Meta["reasoning_content"]; ok {
			if s, ok := rc.(string); ok {
				if s == "" {
					s = " "
				}
				prompt[i].ReasoningContent = s
			}
		}
	}
	return prompt
}

// SetImmutablePrefix sets Zone 1 (system prompt + tool definitions) on the
// underlying session backend if it supports prefix caching (e.g.
// ThreeZoneSession). This is a no-op for backends that do not support it
// (plain Session returns nil without side effect). Called once at the
// start of executeLoop to establish the immutable prefix for prefix cache
// hits.
func (e *SessionExtension) SetImmutablePrefix(systemPrompt string, tools []any) error {
	return e.s.SetImmutablePrefix(systemPrompt, tools)
}

// ClearVolatileScratch clears Zone 3 (volatile scratchpad) on the underlying
// session backend. Called at the end of each agent loop round so per-round
// reasoning state does not leak into the next round. No-op on backends
// without a scratchpad (plain Session).
func (e *SessionExtension) ClearVolatileScratch() {
	e.s.ClearVolatileScratch()
}
