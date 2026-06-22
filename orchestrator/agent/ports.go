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

	"go.opentelemetry.io/otel/trace"
)

// SpanStarter is a narrow interface that decouples the orchestration layer
// from the concrete observability/otel implementation. It mirrors the
// standard trace.Tracer.Start signature so that *otel.Tracer (and any
// trace.Tracer) satisfies it without adaptation.
//
// S1: replaces direct import of "github.com/inferglow/observability/otel"
// in 7 files within orchestrator/agent/.
type SpanStarter interface {
	Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span)
}

// SemanticSpanKind identifies the semantic category of an InferGlow span.
// It replaces otel.SpanKind so that the agent package does not depend on
// the observability module for constant values.
type SemanticSpanKind int

const (
	// SpanAgentRun marks a top-level agent run span.
	SpanAgentRun SemanticSpanKind = iota
	// SpanLLMCall marks a language-model invocation span.
	SpanLLMCall
	// SpanToolCall marks a tool/action invocation span.
	SpanToolCall
	// SpanFlowExecute marks a flow execution span.
	SpanFlowExecute
	// SpanPause marks a flow pause span.
	SpanPause
	// SpanResume marks a flow resume span.
	SpanResume
)

// semanticSpanName maps a SemanticSpanKind to its stable semantic span name.
// A non-empty name overrides the default.
// Mirrors observability/otel.spanName but lives in the agent package to
// avoid the import dependency.
func semanticSpanName(kind SemanticSpanKind, name string) string {
	if name != "" {
		return name
	}
	switch kind {
	case SpanAgentRun:
		return "inferglow.agent.run"
	case SpanLLMCall:
		return "inferglow.llm.call"
	case SpanToolCall:
		return "inferglow.tool.call"
	case SpanFlowExecute:
		return "inferglow.flow.execute"
	case SpanPause:
		return "inferglow.flow.pause"
	case SpanResume:
		return "inferglow.flow.resume"
	default:
		return "inferglow.span"
	}
}

// noopSpanStarter is a SpanStarter that returns the existing span from
// context (or a no-op span). Used as the default when no tracer is
// configured, ensuring zero overhead.
type noopSpanStarter struct{}

func (noopSpanStarter) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}
