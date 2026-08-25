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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package cli

import "github.com/inferglow/model"

// turnReceipt tracks per-turn metrics for receipt display.
type turnReceipt struct {
	turnNum     int
	duration    int // seconds
	llmRounds   int
	toolCalls   int
	promptTokens     int
	completionTokens int
	// RF-6: reasoning / tool timing.
	thinkingMs       int64            // total wall-clock thinking time (ms)
	reasoningTokens  int              // from UsageInfo.ReasoningTokens()
	toolDurationsMs  map[string]int64 // tool name → cumulative duration (ms)
	totalOutputChars int              // total streamed output characters (RF-7 TPS)
	// RF-8: last provider-reported usage (for cache-hit rate).
	usage *model.UsageInfo
}
