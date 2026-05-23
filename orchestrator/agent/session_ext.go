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

package agent

import (
	"fmt"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/security/pii"
	"github.com/inferglow/session"
)

// Compile-time guard: *pii.Masker must satisfy session.MessageMasker so the
// agent can wire it into the session as a PII hook.
var _ session.MessageMasker = (*pii.Masker)(nil)

// SessionExtension wraps the session.Session and provides
// a simplified interface for the orchestrator to manage conversation history.
type SessionExtension struct {
	s *session.Session
}

// NewSessionExtension creates a SessionExtension wrapping the given session.
func NewSessionExtension(s *session.Session) *SessionExtension {
	return &SessionExtension{s: s}
}

// AddUserMessage adds a user message to the session history.
func (e *SessionExtension) AddUserMessage(content string) {
	e.s.AddMessage("user", content, "")
}

// AddAssistantMessage adds an assistant message to the session history.
func (e *SessionExtension) AddAssistantMessage(content string) {
	e.s.AddMessage("assistant", content, "")
}

// SetMessageMasker installs (or clears, when m is nil) a PII/security
// masker on the underlying session. The masker is consulted by
// Session.AddMessageChecked to redact string content before it is
// appended to the history. This is the imperative bridge between the
// agent-level WithPIIMasker option and the session's masking hook.
func (e *SessionExtension) SetMessageMasker(m session.MessageMasker) {
	e.s.SetMessageMasker(m)
}

// AddActionResult adds an action execution result as a text message to the session.
// A nil result is silently ignored (no message added) so callers cannot
// crash the orchestrator by passing a nil pointer after a failed action
// lookup.
func (e *SessionExtension) AddActionResult(actionName string, result *action.ActionResult) {
	if result == nil {
		// O-HIGH-3: silently skip nil results to avoid a nil-pointer panic
		// on result.Status / result.Result below. Returning without
		// recording a message preserves the pre-fix observable behavior
		// for callers that always pass non-nil results.
		return
	}
	msg := fmt.Sprintf("Action %q executed: status=%s, result=%v", actionName, result.Status, result.Result)
	if result.Error != "" {
		msg = fmt.Sprintf("Action %q failed: %s", actionName, result.Error)
	}
	e.s.AddMessage("system", msg, "")
}

// PreparePrompt returns the session's context window as chat messages.
func (e *SessionExtension) PreparePrompt() []model.ChatMessage {
	history := e.s.PreparePrompt()
	prompt := make([]model.ChatMessage, len(history))
	for i, msg := range history {
		prompt[i] = model.ChatMessage{
			Role:    msg.Role,
			Content: fmt.Sprintf("%v", msg.Content),
			Name:    msg.Name,
		}
	}
	return prompt
}
