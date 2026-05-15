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
