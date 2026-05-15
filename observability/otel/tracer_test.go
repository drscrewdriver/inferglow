package otel

import (
	"context"
	"testing"

	gootel "go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newTestTracer builds a Tracer backed by an in-memory exporter using a
// synchronous span processor, so ended spans are immediately observable. It
// constructs the Tracer directly (rather than via NewTracer) to avoid
// mutating the global TracerProvider across tests.
func newTestTracer(t *testing.T) (*Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return &Tracer{tr: tp.Tracer("test")}, exp
}

func TestSpanName(t *testing.T) {
	cases := []struct {
		kind SpanKind
		name string
		want string
	}{
		{SpanAgentRun, "", "inferglow.agent.run"},
		{SpanLLMCall, "", "inferglow.llm.call"},
		{SpanToolCall, "", "inferglow.tool.call"},
		{SpanFlowExecute, "", "inferglow.flow.execute"},
		{SpanPause, "", "inferglow.flow.pause"},
		{SpanResume, "", "inferglow.flow.resume"},
		{SpanAgentRun, "custom.name", "custom.name"},
		{SpanKind(999), "", "inferglow.span"},
	}
	for _, c := range cases {
		if got := spanName(c.kind, c.name); got != c.want {
			t.Errorf("spanName(%v, %q) = %q, want %q", c.kind, c.name, got, c.want)
		}
	}
}

func TestNewTracerUsesGlobalProvider(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	prev := gootel.GetTracerProvider()
	gootel.SetTracerProvider(tp)
	t.Cleanup(func() {
		gootel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	})

	tracer := NewTracer("inferglow-test")
	_, span := tracer.StartSpan(context.Background(), SpanAgentRun, "")
	span.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "inferglow.agent.run" {
		t.Errorf("span name = %q, want %q", spans[0].Name, "inferglow.agent.run")
	}
}

func TestStartSpanCustomNameOverridesKind(t *testing.T) {
	tracer, exp := newTestTracer(t)
	_, span := tracer.StartSpan(context.Background(), SpanLLMCall, "llm.specific.model")
	span.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "llm.specific.model" {
		t.Errorf("span name = %q, want %q", spans[0].Name, "llm.specific.model")
	}
}

func TestStartSpanEndsCleanly(t *testing.T) {
	tracer, exp := newTestTracer(t)
	for i := 0; i < 3; i++ {
		_, span := tracer.StartSpan(context.Background(), SpanFlowExecute, "")
		span.End()
	}
	if got := len(exp.GetSpans()); got != 3 {
		t.Errorf("expected 3 ended spans, got %d", got)
	}
}
