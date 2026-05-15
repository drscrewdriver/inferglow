package otel

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// StartToolSpan starts a SpanToolCall span tagged with the tool name.
// Callers must call span.End() when the tool call completes.
func StartToolSpan(ctx context.Context, tracer *Tracer, toolName string) (context.Context, trace.Span) {
	return tracer.StartSpan(ctx, SpanToolCall, "", WithToolAttrs(toolName))
}
