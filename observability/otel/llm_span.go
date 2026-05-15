package otel

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// LLMUsage is the token-usage subset relevant to tracing. It mirrors the
// fields of model.UsageInfo that are recorded as span attributes, without
// importing the model package (which would create a module cycle).
type LLMUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	ReasoningTokens  int
}

// StartLLMSpan starts a SpanLLMCall span tagged with the model and provider.
// Callers must call span.End() when the LLM call completes.
func StartLLMSpan(ctx context.Context, tracer *Tracer, modelName, providerName string) (context.Context, trace.Span) {
	return tracer.StartSpan(ctx, SpanLLMCall, "", WithModelAttrs(modelName, providerName))
}

// RecordLLMUsage records token-usage attributes on span. Safe to call with a
// non-recording span; noop spans ignore SetAttributes.
func RecordLLMUsage(span trace.Span, usage LLMUsage) {
	span.SetAttributes(
		attribute.Int(AttrTokensPrompt, usage.PromptTokens),
		attribute.Int(AttrTokensCompletion, usage.CompletionTokens),
		attribute.Int(AttrTokensReasoning, usage.ReasoningTokens),
	)
}
