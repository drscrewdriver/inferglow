package session

import (
	"errors"
	"sync"

	promptinjection "github.com/inferglow/security/prompt_injection"
)

// ErrPromptInjectionBlocked is returned by SecurityHook.BeforeAddMessage
// when the configured detector resolves a detection to ActionBlock. The
// integration layer (Session.AddMessage) treats a non-nil error as
// "do not append the message".
var ErrPromptInjectionBlocked = errors.New("prompt injection detected: message blocked")

// MessageHook is invoked before a message is appended to the session.
// Returning a non-nil error prevents the message from being added.
// Implementations must be safe for concurrent use.
type MessageHook interface {
	// BeforeAddMessage inspects the pending (role, content, name) tuple.
	// A non-nil error blocks the append; a nil error allows it.
	BeforeAddMessage(role string, content any, name string) error
}

// FlagRecord captures a detection that was flagged (rather than
// blocked) so callers can audit or surface it. The message was allowed
// through.
type FlagRecord struct {
	Role    string
	Content string
	Result  *promptinjection.DetectionResult
}

// SecurityHook is the default MessageHook implementation. It runs the
// L1 prompt-injection detector over the incoming message text and maps
// the result to an Action via the configured Config:
//
//   - ActionBlock: BeforeAddMessage returns ErrPromptInjectionBlocked
//     and the message is rejected.
//   - ActionFlag:  the detection is recorded in Flags() and the message
//     is allowed through (BeforeAddMessage returns nil).
//   - ActionAllow: the message is allowed through unchanged.
//
// SecurityHook is safe for concurrent use.
type SecurityHook struct {
	detector *promptinjection.Detector
	config   *promptinjection.Config

	// OnFlag, when non-nil, is invoked synchronously after a detection
	// is flagged. It must not block on session internals. May be nil.
	OnFlag func(role string, content string, result *promptinjection.DetectionResult)

	mu    sync.Mutex
	flags []FlagRecord
}

// NewSecurityHook builds a SecurityHook from a prompt-injection Config.
// A nil cfg is replaced with the default Strict config. The hook owns
// its own Detector instance.
func NewSecurityHook(cfg *promptinjection.Config) *SecurityHook {
	if cfg == nil {
		cfg = promptinjection.NewDefaultConfig()
	}
	return &SecurityHook{
		detector: promptinjection.NewDetectorWithConfig(cfg),
		config:   cfg,
	}
}

// BeforeAddMessage implements MessageHook. It converts content to text
// via ContentToString, runs the detector, resolves the Action, and
// applies the block/flag/allow policy.
func (h *SecurityHook) BeforeAddMessage(role string, content any, name string) error {
	if h == nil || h.detector == nil {
		return nil
	}
	text := ContentToString(content)
	result := h.detector.Detect(text)
	action := h.config.ResolveAction(result)
	switch action {
	case promptinjection.ActionBlock:
		return ErrPromptInjectionBlocked
	case promptinjection.ActionFlag:
		h.recordFlag(role, text, result)
		if h.OnFlag != nil {
			h.OnFlag(role, text, result)
		}
		return nil
	default:
		return nil
	}
}

// Flags returns a copy of all flagged detections recorded so far.
func (h *SecurityHook) Flags() []FlagRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]FlagRecord, len(h.flags))
	copy(out, h.flags)
	return out
}

func (h *SecurityHook) recordFlag(role, content string, result *promptinjection.DetectionResult) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.flags = append(h.flags, FlagRecord{Role: role, Content: content, Result: result})
}

// SessionOption configures a Session constructed via
// NewSessionWithOptions. Options are applied after the baseline fields
// are initialized, so they can override defaults.
type SessionOption func(*Session)

// WithSecurityHook injects a MessageHook into the session. The hook is
// consulted on every AddMessage / AddMessageChecked call; a non-nil
// error from the hook prevents the message from being appended. Pass
// nil to disable an existing hook.
func WithSecurityHook(hook MessageHook) SessionOption {
	return func(s *Session) {
		s.securityHook = hook
	}
}

// NewSessionWithOptions creates a Session with the given id and
// maxLength, then applies the supplied options. This preserves the
// original NewSession constructor (and its callers) while allowing
// opt-in features such as the security hook.
func NewSessionWithOptions(id string, maxLength int, opts ...SessionOption) *Session {
	s := NewSession(id, maxLength)
	for _, opt := range opts {
		opt(s)
	}
	return s
}
