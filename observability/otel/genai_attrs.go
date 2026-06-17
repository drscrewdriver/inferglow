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

// GenAI semantic attribute keys following OTel GenAI conventions.
// See: https://opentelemetry.io/docs/specs/semconv/gen-ai/
const (
	// Operation attributes
	GenAIOperationName = "gen_ai.operation.name"
	GenAIProviderName  = "gen_ai.provider.name"

	// Request attributes
	GenAIRequestModel       = "gen_ai.request.model"
	GenAIRequestMaxTokens   = "gen_ai.request.max_tokens"
	GenAIRequestTemperature = "gen_ai.request.temperature"
	GenAIRequestTopP        = "gen_ai.request.top_p"
	GenAIRequestStream      = "gen_ai.request.stream"
	GenAIRequestReasonLevel = "gen_ai.request.reasoning.level"

	// Response attributes
	GenAIResponseID            = "gen_ai.response.id"
	GenAIResponseModel         = "gen_ai.response.model"
	GenAIResponseFinishReasons = "gen_ai.response.finish_reasons"
	GenAIResponseTimeToFirst   = "gen_ai.response.time_to_first_chunk"

	// Usage attributes
	GenAIUsageInputTokens       = "gen_ai.usage.input_tokens"
	GenAIUsageOutputTokens      = "gen_ai.usage.output_tokens"
	GenAIUsageReasoningTokens   = "gen_ai.usage.reasoning_tokens"
	GenAIUsageCacheReadTokens   = "gen_ai.usage.cache_read.input_tokens"
	GenAIUsageCacheCreationTok  = "gen_ai.usage.cache_creation.input_tokens"

	// Agent attributes
	GenAIAgentName        = "gen_ai.agent.name"
	GenAIAgentID          = "gen_ai.agent.id"
	GenAIAgentDescription = "gen_ai.agent.description"

	// Tool attributes
	GenAIToolName        = "gen_ai.tool.name"
	GenAIToolCallID      = "gen_ai.tool.call.id"
	GenAIToolDescription = "gen_ai.tool.description"
	GenAIToolType        = "gen_ai.tool.type"
	GenAIToolCallArgs    = "gen_ai.tool.call.arguments"
	GenAIToolCallResult  = "gen_ai.tool.call.result"

	// Conversation attributes
	GenAIConversationID        = "gen_ai.conversation.id"
	GenAIConversationCompacted = "gen_ai.conversation.compacted"

	// Error attributes
	GenAIErrorType = "error.type"
)

// GenAIOperation constants for gen_ai.operation.name.
const (
	GenAIOpChat            = "chat"
	GenAIOpTextCompletion  = "text_completion"
	GenAIOpGenerateContent = "generate_content"
	GenAIOpEmbeddings      = "embeddings"
	GenAIOpInvokeAgent     = "invoke_agent"
	GenAIOpCreateAgent     = "create_agent"
	GenAIOpPlan            = "plan"
	GenAIOpExecuteTool     = "execute_tool"
	GenAIOpSearchMemory    = "search_memory"
	GenAIOpCreateMemory    = "create_memory"
	GenAIOpUpdateMemory    = "update_memory"
	GenAIOpDeleteMemory    = "delete_memory"
	GenAIOpInvokeWorkflow  = "invoke_workflow"
	GenAIOpRetrieval       = "retrieval"
)

// WithGenAIInferenceAttrs returns span options for an LLM inference call.
func WithGenAIInferenceAttrs(operation, provider, model string) trace.SpanStartOption {
	return trace.WithAttributes(
		attribute.String(GenAIOperationName, operation),
		attribute.String(GenAIProviderName, provider),
		attribute.String(GenAIRequestModel, model),
	)
}

// WithGenAIAgentAttrs returns span options for an agent invocation.
func WithGenAIAgentAttrs(agentName, agentID string) trace.SpanStartOption {
	return trace.WithAttributes(
		attribute.String(GenAIOperationName, GenAIOpInvokeAgent),
		attribute.String(GenAIAgentName, agentName),
		attribute.String(GenAIAgentID, agentID),
	)
}

// WithGenAIToolAttrs returns span options for a tool execution.
func WithGenAIToolAttrs(toolName, toolType, callID string) trace.SpanStartOption {
	return trace.WithAttributes(
		attribute.String(GenAIOperationName, GenAIOpExecuteTool),
		attribute.String(GenAIToolName, toolName),
		attribute.String(GenAIToolType, toolType),
		attribute.String(GenAIToolCallID, callID),
	)
}

// InferenceUsage holds usage data returned from an LLM call.
type InferenceUsage struct {
	InputTokens      int
	OutputTokens     int
	ReasoningTokens  int
	CacheReadTokens  int
	CacheCreateTok   int
	FinishReasons    []string
	ResponseID       string
	ResponseModel    string
}

// InferenceUsageAttrs returns span attributes for recording inference usage.
func InferenceUsageAttrs(u InferenceUsage) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.Int(GenAIUsageInputTokens, u.InputTokens),
		attribute.Int(GenAIUsageOutputTokens, u.OutputTokens),
	}
	if u.ReasoningTokens > 0 {
		attrs = append(attrs, attribute.Int(GenAIUsageReasoningTokens, u.ReasoningTokens))
	}
	if u.CacheReadTokens > 0 {
		attrs = append(attrs, attribute.Int(GenAIUsageCacheReadTokens, u.CacheReadTokens))
	}
	if u.CacheCreateTok > 0 {
		attrs = append(attrs, attribute.Int(GenAIUsageCacheCreationTok, u.CacheCreateTok))
	}
	if len(u.FinishReasons) > 0 {
		attrs = append(attrs, attribute.StringSlice(GenAIResponseFinishReasons, u.FinishReasons))
	}
	if u.ResponseID != "" {
		attrs = append(attrs, attribute.String(GenAIResponseID, u.ResponseID))
	}
	if u.ResponseModel != "" {
		attrs = append(attrs, attribute.String(GenAIResponseModel, u.ResponseModel))
	}
	return attrs
}
