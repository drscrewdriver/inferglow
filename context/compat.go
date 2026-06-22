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

package contextmgr

// SessionCompat provides compatibility layer between contextmgr and session.
// It handles dual-write (session.AddMessage + ctxManager.Ingest) and
// context injection (session.SetChatHistory with BuildContext output).
type SessionCompat struct {
	sessionID string
	linkStore StepStoreLike
}

// NewSessionCompat creates a session compatibility layer.
func NewSessionCompat(sessionID string, store StepStoreLike) *SessionCompat {
	return &SessionCompat{
		sessionID: sessionID,
		linkStore: store,
	}
}

// CreateStepLink records the mapping between step_id and session message index.
// This is called during Ingest to maintain the link.
func (s *SessionCompat) CreateStepLink(stepID, msgIndex int) StepSessionLink {
	return StepSessionLink{
		StepID:    stepID,
		MsgIndex:  msgIndex,
		SessionID: s.sessionID,
	}
}

// ShouldInjectContext returns true if the context manager is not in passthrough mode.
// When true, the caller should use BuildContext output to override session.ContextWindow.
func (s *SessionCompat) ShouldInjectContext(mode Mode) bool {
	return mode != ModePassthrough
}

// ReplayFromFullContext generates L0 jsonl from existing Session.FullContext.
// This is used when switching from passthrough → hybrid mode.
func (s *SessionCompat) ReplayFromFullContext(messages []StepRecord) error {
	for _, msg := range messages {
		if err := s.linkStore.AppendStep(msg); err != nil {
			return err
		}
	}
	return nil
}
