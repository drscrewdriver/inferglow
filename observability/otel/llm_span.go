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
