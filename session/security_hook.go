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

package session

// MessageHook is invoked before a message is appended to the session.
// Returning a non-nil error prevents the message from being added.
// Implementations must be safe for concurrent use.
//
// The session module deliberately keeps only this interface so it has no
// hard dependency on the security module. The default prompt-injection
// backed implementation (SecurityHook) now lives in the security module's
// sessionhook subpackage, which depends on session and wires its hook in
// via WithSecurityHook. Callers may also provide their own MessageHook
// implementation.
type MessageHook interface {
	// BeforeAddMessage Inspects the pending (role, content, name) tuple.
	// A non-nil error blocks the append; a nil error allows it.
	BeforeAddMessage(role string, content any, name string) error
}

// SessionOption configures a Session constructed via
// NewSessionWithOptions. Options are applied after the baseline fields
// are initialized, so they can override defaults.
type SessionOption func(*Session) //nolint:revive

// WithSecurityHook injects a MessageHook into the session. The hook is
// consulted on every AddMessage / AddMessageChecked call; a non-nil
// error from the hook prevents the message from being appended. Pass
// nil to disable an existing hook.
//
// The concrete SecurityHook implementation has moved to the security
// module's sessionhook subpackage; this option accepts any MessageHook
// so the session module stays decoupled from security.
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
