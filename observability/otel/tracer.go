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

package otel

import (
	"context"

	gootel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// SpanKind identifies the semantic category of an InferGlow span. It is
// distinct from trace.SpanKind (which describes the client/server role) and
// is used to select a stable, semantic span name.
type SpanKind int

const (
	// SpanAgentRun marks a top-level agent run span.
	SpanAgentRun SpanKind = iota
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

// Tracer wraps an OpenTelemetry trace.Tracer with InferGlow-specific span
// semantics. It does not own a TracerProvider; NewTracer resolves the tracer
// from the global provider so InstallNewProvider (or any caller that
// configures the global provider) is picked up automatically.
type Tracer struct {
	tr trace.Tracer
}

// NewTracer returns a Tracer backed by the global TracerProvider's tracer for
// name. Pass trace.TracerOption values to scope the tracer.
func NewTracer(name string, opts ...trace.TracerOption) *Tracer {
	return &Tracer{tr: gootel.GetTracerProvider().Tracer(name, opts...)}
}

// Start implements the agent.SpanStarter interface by delegating to the
// underlying trace.Tracer. It satisfies the narrow interface used by the
// orchestration layer after S1 decoupling, so that *Tracer can be injected
// without the agent package importing observability/otel directly.
func (t *Tracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tr.Start(ctx, spanName, opts...)
}

// StartSpan starts a semantic span for kind. If name is empty the span name
// is derived from kind via spanName; otherwise name is used as the span name.
// The supplied SpanOption values (typically attribute helpers such as
// WithModelAttrs) are forwarded to the underlying tracer.
func (t *Tracer) StartSpan(ctx context.Context, kind SpanKind, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tr.Start(ctx, spanName(kind, name), opts...)
}

// spanName maps a SpanKind to its semantic span name. A non-empty name
// overrides the default.
func spanName(kind SpanKind, name string) string {
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
