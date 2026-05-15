package otel

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// attrValue returns the value for key from a span stub's attributes, or false.
func attrValue(t *testing.T, span tracetest.SpanStub, key string) (attribute.Value, bool) {
	t.Helper()
	for _, kv := range span.Attributes {
		if string(kv.Key) == key {
			return kv.Value, true
		}
	}
	return attribute.Value{}, false
}

func requireStrAttr(t *testing.T, span tracetest.SpanStub, key, want string) {
	t.Helper()
	v, ok := attrValue(t, span, key)
	if !ok {
		t.Fatalf("span %q missing attribute %q", span.Name, key)
	}
	if got := v.AsString(); got != want {
		t.Errorf("span %q attr %q = %q, want %q", span.Name, key, got, want)
	}
}

func requireIntAttr(t *testing.T, span tracetest.SpanStub, key string, want int64) {
	t.Helper()
	v, ok := attrValue(t, span, key)
	if !ok {
		t.Fatalf("span %q missing attribute %q", span.Name, key)
	}
	if got := v.AsInt64(); got != want {
		t.Errorf("span %q attr %q = %d, want %d", span.Name, key, got, want)
	}
}

func TestStartAgentSpan(t *testing.T) {
	tracer, exp := newTestTracer(t)
	ctx, span := StartAgentSpan(context.Background(), tracer, "sess-1", "run-1")
	if span == nil {
		t.Fatal("span is nil")
	}
	span.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name != "inferglow.agent.run" {
		t.Errorf("name = %q, want %q", s.Name, "inferglow.agent.run")
	}
	requireStrAttr(t, s, AttrSessionID, "sess-1")
	requireStrAttr(t, s, AttrRunID, "run-1")

	if ctx == nil {
		t.Error("returned context is nil")
	}
}

func TestStartLLMSpan(t *testing.T) {
	tracer, exp := newTestTracer(t)
	_, span := StartLLMSpan(context.Background(), tracer, "gpt-4o", "openai")
	span.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name != "inferglow.llm.call" {
		t.Errorf("name = %q, want %q", s.Name, "inferglow.llm.call")
	}
	requireStrAttr(t, s, AttrModelName, "gpt-4o")
	requireStrAttr(t, s, AttrModelProvider, "openai")
}

func TestRecordLLMUsage(t *testing.T) {
	tracer, exp := newTestTracer(t)
	_, span := StartLLMSpan(context.Background(), tracer, "mimo", "mimo")
	RecordLLMUsage(span, LLMUsage{
		PromptTokens:     120,
		CompletionTokens: 80,
		TotalTokens:      200,
		ReasoningTokens:  40,
	})
	span.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	requireIntAttr(t, s, AttrTokensPrompt, 120)
	requireIntAttr(t, s, AttrTokensCompletion, 80)
	requireIntAttr(t, s, AttrTokensReasoning, 40)
}

func TestStartToolSpan(t *testing.T) {
	tracer, exp := newTestTracer(t)
	_, span := StartToolSpan(context.Background(), tracer, "web_search")
	span.End()

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name != "inferglow.tool.call" {
		t.Errorf("name = %q, want %q", s.Name, "inferglow.tool.call")
	}
	requireStrAttr(t, s, AttrToolName, "web_search")
}

func TestStartSpanPauseResumeNames(t *testing.T) {
	tracer, exp := newTestTracer(t)
	_, p := tracer.StartSpan(context.Background(), SpanPause, "")
	p.End()
	_, r := tracer.StartSpan(context.Background(), SpanResume, "")
	r.End()

	spans := exp.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	if spans[0].Name != "inferglow.flow.pause" {
		t.Errorf("pause name = %q", spans[0].Name)
	}
	if spans[1].Name != "inferglow.flow.resume" {
		t.Errorf("resume name = %q", spans[1].Name)
	}
}

func TestWithModelAttrsSetsAttributes(t *testing.T) {
	tracer, exp := newTestTracer(t)
	_, span := tracer.StartSpan(context.Background(), SpanLLMCall, "", WithModelAttrs("claude", "anthropic"))
	span.End()
	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	requireStrAttr(t, spans[0], AttrModelName, "claude")
	requireStrAttr(t, spans[0], AttrModelProvider, "anthropic")
}

func TestSpansConcurrent(t *testing.T) {
	tracer, exp := newTestTracer(t)
	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			kind := SpanKind(i % 4)
			_, span := tracer.StartSpan(context.Background(), kind, fmt.Sprintf("span-%d", i))
			span.End()
		}(i)
	}
	wg.Wait()

	if got := len(exp.GetSpans()); got != n {
		t.Errorf("expected %d spans, got %d", n, got)
	}
}

func TestRecordLLMUsageOnNoopSpan(t *testing.T) {
	// A non-recording (noop) span must not panic when recording usage.
	noop := trace.SpanFromContext(context.Background())
	RecordLLMUsage(noop, LLMUsage{PromptTokens: 1, CompletionTokens: 2, ReasoningTokens: 3})
}
