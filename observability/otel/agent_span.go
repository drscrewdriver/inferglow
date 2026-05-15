package otel

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// StartAgentSpan starts a SpanAgentRun span tagged with the session and run
// IDs. Callers (typically Agent.Run) must call span.End() when the run
// completes, including on error paths.
func StartAgentSpan(ctx context.Context, tracer *Tracer, sessionID, runID string) (context.Context, trace.Span) {
	return tracer.StartSpan(ctx, SpanAgentRun, "", WithSessionAttrs(sessionID, runID))
}
