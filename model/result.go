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

package model

// ResultEventType 定义事件类型
type ResultEventType string

const (
	// EventDelta is the result event carrying an incremental content delta.
	EventDelta ResultEventType = "delta"
	// EventDone is the result event signaling the stream has completed.
	EventDone ResultEventType = "done"
	// ErrorEvent is the result event carrying an error.
	ErrorEvent ResultEventType = "error"
	// MetaEvent is the result event carrying metadata such as token usage.
	MetaEvent ResultEventType = "meta"
	// StatusEvent is the result event carrying a status update.
	StatusEvent ResultEventType = "status"
	// ToolCallsEvent is the result event carrying tool call requests.
	ToolCallsEvent ResultEventType = "tool_calls"
	// ReasoningDelta is the result event carrying an incremental reasoning content delta.
	ReasoningDelta ResultEventType = "reasoning_delta"
	// ReasoningDone is the result event signaling the reasoning content has completed.
	ReasoningDone ResultEventType = "reasoning_done"
	// OriginalDelta is the result event carrying an incremental raw (untransformed) content delta.
	OriginalDelta ResultEventType = "original_delta"
	// OriginalDone is the result event signaling the raw content stream has completed.
	OriginalDone ResultEventType = "original_done"
)

// ResultEvent 广播到事件流的统一事件
type ResultEvent struct {
	EventType ResultEventType
	Payload   any
}

// ReasoningTokenMeta 推理 token 元数据，作为 MetaEvent 的 Payload 出现。
// 当 Provider 在 usage.completion_tokens_details.reasoning_tokens 中报告
// 推理 token 计数时，BroadcastResponse 会发出此事件，便于上层单独追踪
// 推理计费（G1-06）。
type ReasoningTokenMeta struct {
	Count int
}
