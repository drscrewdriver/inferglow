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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Semantic attribute keys exported by InferGlow spans.
const (
	AttrModelName        = "llm.model_name"
	AttrModelProvider    = "llm.provider_name"
	AttrToolName         = "tool.name"
	AttrSessionID        = "inferglow.session_id"
	AttrRunID            = "inferglow.run_id"
	AttrTokensPrompt     = "llm.usage.prompt_tokens"
	AttrTokensCompletion = "llm.usage.completion_tokens"
	AttrTokensReasoning  = "llm.usage.reasoning_tokens"
)

// WithModelAttrs returns a SpanStartOption that records the model and provider.
func WithModelAttrs(modelName, provider string) trace.SpanStartOption {
	return trace.WithAttributes(
		attribute.String(AttrModelName, modelName),
		attribute.String(AttrModelProvider, provider),
	)
}

// WithToolAttrs returns a SpanStartOption that records the tool name.
func WithToolAttrs(toolName string) trace.SpanStartOption {
	return trace.WithAttributes(
		attribute.String(AttrToolName, toolName),
	)
}

// WithSessionAttrs returns a SpanStartOption that records the session and run IDs.
func WithSessionAttrs(sessionID, runID string) trace.SpanStartOption {
	return trace.WithAttributes(
		attribute.String(AttrSessionID, sessionID),
		attribute.String(AttrRunID, runID),
	)
}
