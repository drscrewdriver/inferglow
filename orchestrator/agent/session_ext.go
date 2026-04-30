package agent

import (
	"fmt"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

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

// AddActionResult adds an action execution result as a text message to the session.
func (e *SessionExtension) AddActionResult(actionName string, result *action.ActionResult) {
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
