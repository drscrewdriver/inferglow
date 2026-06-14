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
	"fmt"
	"strings"
	"sync"
)

// SummaryMemory stores conversation history and automatically summarizes
// old messages when the total token count exceeds a configurable threshold.
// The summary is prepended to the message list, preserving recent messages
// in full.
//
// When the summarizer is nil, SummaryMemory degrades to a simple message
// store (equivalent to TokenBufferMemory without trimming).
type SummaryMemory struct {
	mu sync.Mutex

	// messages holds the current conversation state: an optional summary
	// message at index 0 followed by recent individual messages.
	messages []ChatMessage

	// tokenThreshold is the token count at which old messages are
	// summarized. When the estimated token count of messages[1:] exceeds
	// this value, messages[1:len-messages/2] are summarized and replaced
	// with a single summary message.
	tokenThreshold int

	// summarizer generates summaries. nil disables auto-summarization.
	summarizer Summarizer

	// estimateFunc estimates the token count of a string. nil uses the
	// default estimate of len(s)/4.
	estimateFunc func(string) int
}

// SummaryMemoryOption configures a SummaryMemory.
type SummaryMemoryOption func(*SummaryMemory)

// WithTokenThreshold sets the token threshold for auto-summarization.
func WithTokenThreshold(n int) SummaryMemoryOption {
	return func(m *SummaryMemory) {
		m.tokenThreshold = n
	}
}

// WithSummarizer sets the summarizer implementation.
func WithSummarizer(s Summarizer) SummaryMemoryOption {
	return func(m *SummaryMemory) {
		m.summarizer = s
	}
}

// WithEstimateFunc sets a custom token estimation function.
func WithEstimateFunc(fn func(string) int) SummaryMemoryOption {
	return func(m *SummaryMemory) {
		m.estimateFunc = fn
	}
}

// NewSummaryMemory creates a SummaryMemory with the given options.
// Default threshold is 2000 tokens; default estimate is len(s)/4.
func NewSummaryMemory(opts ...SummaryMemoryOption) *SummaryMemory {
	m := &SummaryMemory{
		tokenThreshold: 2000,
	}
	for _, opt := range opts {
		opt(m)
	}
	if m.estimateFunc == nil {
		m.estimateFunc = func(s string) int { return len(s) / 4 }
	}
	return m
}

// Load returns the current conversation memory.
func (m *SummaryMemory) Load() []ChatMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ChatMessage, len(m.messages))
	copy(out, m.messages)
	return out
}

// Save replaces the memory contents and triggers auto-summarization if
// the token count exceeds the threshold.
func (m *SummaryMemory) Save(messages []ChatMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = make([]ChatMessage, len(messages))
	copy(m.messages, messages)
	m.maybeSummarize()
}

// Clear resets all memory state.
func (m *SummaryMemory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = nil
}

// maybeSummarize checks if the token count exceeds the threshold and,
// if so, summarizes the older half of the messages (excluding any
// existing summary at index 0).
func (m *SummaryMemory) maybeSummarize() {
	if m.summarizer == nil || len(m.messages) <= 2 {
		return
	}

	// Calculate total tokens (skip index 0 if it's a summary).
	startIdx := 0
	if len(m.messages) > 0 && m.messages[0].Role == "system" && strings.HasPrefix(ContentToString(m.messages[0].Content), "[summary:") {
		startIdx = 1
	}

	totalTokens := 0
	for i := startIdx; i < len(m.messages); i++ {
		totalTokens += m.estimateFunc(ContentToString(m.messages[i].Content))
	}
	if totalTokens <= m.tokenThreshold {
		return
	}

	// Summarize the older half of messages.
	recentStart := startIdx + (len(m.messages)-startIdx)/2
	if recentStart >= len(m.messages) {
		return
	}

	var toSummarize strings.Builder
	for i := startIdx; i < recentStart; i++ {
		toSummarize.WriteString(ContentToString(m.messages[i].Content))
		toSummarize.WriteString("\n")
	}

	summary, err := m.summarizer.Summarize(toSummarize.String())
	if err != nil {
		// On summarizer failure, keep existing messages unchanged.
		return
	}

	summaryMsg := ChatMessage{
		Role:    "system",
		Content: fmt.Sprintf("[summary: %s]", summary),
	}

	// Rebuild: summary + recent messages.
	recent := make([]ChatMessage, len(m.messages)-recentStart)
	copy(recent, m.messages[recentStart:])
	m.messages = append([]ChatMessage{summaryMsg}, recent...)
}

// AsResizeHandler returns a ResizeHandler compatible with the Session's
// multi-strategy resize mechanism. Register it via:
//
//	sess.RegisterResizeHandler("summary", mem.AsResizeHandler())
func (m *SummaryMemory) AsResizeHandler() ResizeHandler {
	return func(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error) {
		m.Save(contextWindow)
		return m.Load(), nil
	}
}
