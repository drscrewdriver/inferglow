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

// Package sessionhook provides the default prompt-injection backed
// implementation of session.MessageHook. It lives in the security module
// so that the session module remains free of any hard dependency on
// security. Callers wire a SecurityHook into a session via
// session.WithSecurityHook.
package sessionhook

import (
	"errors"
	"sync"

	"github.com/inferglow/session"
	promptinjection "github.com/inferglow/security/prompt_injection"
)

// Compile-time guarantee that SecurityHook satisfies session.MessageHook.
var _ session.MessageHook = (*SecurityHook)(nil)

// ErrPromptInjectionBlocked is returned by SecurityHook.BeforeAddMessage
// when the configured detector resolves a detection to ActionBlock. The
// integration layer (Session.AddMessage) treats a non-nil error as
// "do not append the message".
var ErrPromptInjectionBlocked = errors.New("prompt injection detected: message blocked")

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

// BeforeAddMessage implements session.MessageHook. It converts content
// to text via session.ContentToString, runs the detector, resolves the
// Action, and applies the block/flag/allow policy.
func (h *SecurityHook) BeforeAddMessage(role string, content any, name string) error {
	if h == nil || h.detector == nil {
		return nil
	}
	text := session.ContentToString(content)
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
