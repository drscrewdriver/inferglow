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
	SpanAgentRun SpanKind = iota
	SpanLLMCall
	SpanToolCall
	SpanFlowExecute
	SpanPause
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
