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

package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/inferglow/model"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CallbacksTracer bridges AgentCallbacks lifecycle hooks to OpenTelemetry
// spans. Each lifecycle event creates a semantic span via the otel.Tracer:
//
//   - OnRunStart/OnRunEnd → SpanAgentRun
//   - OnLLMCallStart/OnLLMCallEnd → SpanLLMCall
//   - OnToolCallStart/OnToolCallEnd → SpanToolCall
//
// Use NewCallbacksTracer to create an instance, then pass Callbacks() to
// WithCallbacks when constructing an Agent.
//
// Example:
//
//	tracer := otel.NewTracer("inferglow")
//	ct := NewCallbacksTracer(tracer, "session-1")
//	agent := New(sess, actExt, modelReq, WithCallbacks(ct.Callbacks()))
type CallbacksTracer struct {
	tracer    SpanStarter
	sessionID string

	mu      sync.Mutex
	runSpan trace.Span
	llmSpan trace.Span
	toolSpans map[string]trace.Span
}

// NewCallbacksTracer creates a CallbacksTracer backed by the given tracer.
// sessionID is recorded as a span attribute for correlation.
func NewCallbacksTracer(tracer SpanStarter, sessionID string) *CallbacksTracer {
	return &CallbacksTracer{
		tracer:    tracer,
		sessionID: sessionID,
		toolSpans: make(map[string]trace.Span),
	}
}

// Callbacks returns an *AgentCallbacks that creates OTel spans for each
// lifecycle event. Pass this to WithCallbacks.
func (ct *CallbacksTracer) Callbacks() *AgentCallbacks {
	return &AgentCallbacks{
		OnRunStart: func(ctx context.Context, userMessage string) {
			ct.mu.Lock()
			defer ct.mu.Unlock()
			_, span := ct.tracer.Start(ctx, semanticSpanName(SpanAgentRun, "inferglow.agent.run"),
				trace.WithAttributes(
					attribute.String("inferglow.session_id", ct.sessionID),
					attribute.String("inferglow.user_message_preview", truncateStr(userMessage, 100)),
				),
			)
			ct.runSpan = span
		},
		OnRunEnd: func(ctx context.Context, response string, err error) {
			ct.mu.Lock()
			defer ct.mu.Unlock()
			if ct.runSpan != nil {
				if err != nil {
					ct.runSpan.SetStatus(2, err.Error()) // codes.Error = 2
				}
				ct.runSpan.SetAttributes(
					attribute.String("inferglow.response_preview", truncateStr(response, 100)),
				)
				ct.runSpan.End()
				ct.runSpan = nil
			}
		},
		OnLLMCallStart: func(ctx context.Context, round int) {
			ct.mu.Lock()
			defer ct.mu.Unlock()
			_, span := ct.tracer.Start(ctx, semanticSpanName(SpanLLMCall, fmt.Sprintf("inferglow.llm.call.%d", round)),
				trace.WithAttributes(
					attribute.Int("inferglow.llm.round", round),
				),
			)
			ct.llmSpan = span
		},
		OnLLMCallEnd: func(ctx context.Context, round int, tokens int, usage *model.UsageInfo) {
			ct.mu.Lock()
			defer ct.mu.Unlock()
			if ct.llmSpan != nil {
				ct.llmSpan.SetAttributes(
					attribute.Int("llm.usage.completion_tokens", tokens),
				)
				ct.llmSpan.End()
				ct.llmSpan = nil
			}
		},
		OnToolCallStart: func(ctx context.Context, toolName string) {
			ct.mu.Lock()
			defer ct.mu.Unlock()
			_, span := ct.tracer.Start(ctx, semanticSpanName(SpanToolCall, fmt.Sprintf("inferglow.tool.%s", toolName)),
				trace.WithAttributes(
					attribute.String("tool.name", toolName),
				),
			)
			ct.toolSpans[toolName] = span
		},
		OnToolCallEnd: func(ctx context.Context, toolName string, err error) {
			ct.mu.Lock()
			defer ct.mu.Unlock()
			if span, ok := ct.toolSpans[toolName]; ok {
				if err != nil {
					span.SetStatus(2, err.Error())
				}
				span.End()
				delete(ct.toolSpans, toolName)
			}
		},
	}
}

// truncateStr returns the first n characters of s, or s if shorter.
func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
