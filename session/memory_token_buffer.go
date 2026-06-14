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

import (
	"sync"
)

// TokenBufferMemory maintains a conversation buffer trimmed by token budget.
// When the total estimated token count exceeds the budget, the oldest
// messages are dropped (always preserving at least the most recent message).
//
// Two estimation modes are supported:
//   - Precise: caller provides a custom EstimateFunc (e.g. tiktoken-based).
//   - Fast: default estimate of len(s)/4 characters per token.
type TokenBufferMemory struct {
	mu sync.Mutex

	// messages holds the current conversation buffer.
	messages []ChatMessage

	// tokenBudget is the maximum estimated token count. When exceeded,
	// oldest messages are dropped from the front.
	tokenBudget int

	// estimateFunc estimates the token count of a string. nil uses the
	// default fast estimate of len(s)/4.
	estimateFunc func(string) int
}

// TokenBufferMemoryOption configures a TokenBufferMemory.
type TokenBufferMemoryOption func(*TokenBufferMemory)

// WithTokenBudget sets the maximum token budget.
func WithTokenBudget(n int) TokenBufferMemoryOption {
	return func(m *TokenBufferMemory) {
		m.tokenBudget = n
	}
}

// WithTokenEstimateFunc sets a custom token estimation function.
func WithTokenEstimateFunc(fn func(string) int) TokenBufferMemoryOption {
	return func(m *TokenBufferMemory) {
		m.estimateFunc = fn
	}
}

// NewTokenBufferMemory creates a TokenBufferMemory with the given options.
// Default budget is 4000 tokens; default estimate is len(s)/4.
func NewTokenBufferMemory(opts ...TokenBufferMemoryOption) *TokenBufferMemory {
	m := &TokenBufferMemory{
		tokenBudget: 4000,
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.estimateFunc == nil {
		m.estimateFunc = func(s string) int { return len(s) / 4 }
	}
	return m
}

// Load returns the current conversation buffer.
func (m *TokenBufferMemory) Load() []ChatMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ChatMessage, len(m.messages))
	copy(out, m.messages)
	return out
}

// Save replaces the buffer contents and trims from the front if the
// total estimated token count exceeds the budget.
func (m *TokenBufferMemory) Save(messages []ChatMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = make([]ChatMessage, len(messages))
	copy(m.messages, messages)
	m.trimToBudget()
}

// Clear resets all memory state.
func (m *TokenBufferMemory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = nil
}

// AddMessage appends a single message to the buffer and trims if needed.
func (m *TokenBufferMemory) AddMessage(msg ChatMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	m.trimToBudget()
}

// TokenCount returns the current estimated token count.
func (m *TokenBufferMemory) TokenCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for _, msg := range m.messages {
		total += m.estimateFunc(ContentToString(msg.Content))
	}
	return total
}

// trimToBudget drops the oldest messages until the total estimated token
// count is within the budget. Always preserves at least the most recent
// message even if it alone exceeds the budget.
func (m *TokenBufferMemory) trimToBudget() {
	if m.tokenBudget <= 0 || len(m.messages) <= 1 {
		return
	}

	total := 0
	for _, msg := range m.messages {
		total += m.estimateFunc(ContentToString(msg.Content))
	}

	for total > m.tokenBudget && len(m.messages) > 1 {
		removed := m.messages[0]
		total -= m.estimateFunc(ContentToString(removed.Content))
		m.messages = m.messages[1:]
	}
}

// AsResizeHandler returns a ResizeHandler compatible with the Session's
// multi-strategy resize mechanism. Register it via:
//
//	sess.RegisterResizeHandler("token_buffer", mem.AsResizeHandler())
func (m *TokenBufferMemory) AsResizeHandler() ResizeHandler {
	return func(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error) {
		m.Save(contextWindow)
		return m.Load(), nil
	}
}
