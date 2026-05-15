package otel

import (
	"context"
	"testing"

	gootel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func resourceHasServiceName(kvs []attribute.KeyValue, want string) bool {
	for _, kv := range kvs {
		if string(kv.Key) == "service.name" && kv.Value.AsString() == want {
			return true
		}
	}
	return false
}

func TestInstallNewProviderExportsSpans(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	prev := gootel.GetTracerProvider()
	cleanup := InstallNewProvider(exp, "inferglow-test")
	t.Cleanup(func() {
		gootel.SetTracerProvider(prev)
	})

	tracer := NewTracer("test")
	_, span := tracer.StartSpan(context.Background(), SpanAgentRun, "")
	span.End()

	spans := exp.GetSpans()
	cleanup()

	if len(spans) != 1 {
		t.Fatalf("expected 1 exported span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name != "inferglow.agent.run" {
		t.Errorf("name = %q, want %q", s.Name, "inferglow.agent.run")
	}
	if s.Resource == nil || !resourceHasServiceName(s.Resource.Attributes(), "inferglow-test") {
		t.Error("span resource missing service.name=inferglow-test")
	}
}

func TestInstallNewProviderReturnsCleanup(t *testing.T) {
	exp := tracetest.NewNoopExporter()
	prev := gootel.GetTracerProvider()
	cleanup := InstallNewProvider(exp, "inferglow-cleanup-test")
	t.Cleanup(func() {
		gootel.SetTracerProvider(prev)
	})
	if cleanup == nil {
		t.Fatal("expected non-nil cleanup function")
	}
	cleanup()
}

func TestNewOTLPExporter(t *testing.T) {
	exp, err := NewOTLPExporter("localhost:4318")
	if err != nil {
		t.Fatalf("NewOTLPExporter: %v", err)
	}
	if exp == nil {
		t.Fatal("exporter is nil")
	}
	_ = exp.Shutdown(context.Background())
}

func TestNewJaegerExporter(t *testing.T) {
	exp, err := NewJaegerExporter("localhost:4318")
	if err != nil {
		t.Fatalf("NewJaegerExporter: %v", err)
	}
	if exp == nil {
		t.Fatal("exporter is nil")
	}
	_ = exp.Shutdown(context.Background())
}

func TestNewPrometheusExporter(t *testing.T) {
	reader, err := NewPrometheusExporter()
	if err != nil {
		t.Fatalf("NewPrometheusExporter: %v", err)
	}
	if reader == nil {
		t.Fatal("reader is nil")
	}
	_ = reader.Shutdown(context.Background())
}
