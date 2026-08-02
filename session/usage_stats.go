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

import "time"

// UsageRecord is a single LLM call record within a session.
type UsageRecord struct {
	Timestamp        time.Time `json:"timestamp"`
	Model            string    `json:"model"`
	Provider         string    `json:"provider"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	CachedTokens     int       `json:"cached_tokens"`
	ReasoningTokens  int       `json:"reasoning_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	Cost             float64   `json:"cost"`
	Currency         string    `json:"currency"`
}

// SessionUsageStats aggregates all usage for a session.
type SessionUsageStats struct {
	SessionID              string        `json:"session_id"`
	TotalPromptTokens      int           `json:"total_prompt_tokens"`
	TotalCompletionTokens  int           `json:"total_completion_tokens"`
	TotalCachedTokens      int           `json:"total_cached_tokens"`
	TotalReasoningTokens   int           `json:"total_reasoning_tokens"`
	TotalTokens            int           `json:"total_tokens"`
	TotalCost              float64       `json:"total_cost"`
	Currency               string        `json:"currency"`
	RecordCount            int           `json:"record_count"`
	Records                []UsageRecord `json:"records,omitempty"`
}