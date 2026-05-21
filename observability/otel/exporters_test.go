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
