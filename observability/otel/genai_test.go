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
)

func TestGenAIAttributeConstants(t *testing.T) {
	// Verify key attribute constants have expected values
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"operation_name", GenAIOperationName, "gen_ai.operation.name"},
		{"provider_name", GenAIProviderName, "gen_ai.provider.name"},
		{"request_model", GenAIRequestModel, "gen_ai.request.model"},
		{"usage_input", GenAIUsageInputTokens, "gen_ai.usage.input_tokens"},
		{"usage_output", GenAIUsageOutputTokens, "gen_ai.usage.output_tokens"},
		{"agent_name", GenAIAgentName, "gen_ai.agent.name"},
		{"tool_name", GenAIToolName, "gen_ai.tool.name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestGenAIOperationConstants(t *testing.T) {
	if GenAIOpChat != "chat" {
		t.Errorf("GenAIOpChat = %q, want chat", GenAIOpChat)
	}
	if GenAIOpInvokeAgent != "invoke_agent" {
		t.Errorf("GenAIOpInvokeAgent = %q, want invoke_agent", GenAIOpInvokeAgent)
	}
	if GenAIOpExecuteTool != "execute_tool" {
		t.Errorf("GenAIOpExecuteTool = %q, want execute_tool", GenAIOpExecuteTool)
	}
}

func TestWithGenAIInferenceAttrs(t *testing.T) {
	opt := WithGenAIInferenceAttrs("chat", "openai", "gpt-4")
	if opt == nil {
		t.Fatal("WithGenAIInferenceAttrs returned nil")
	}
}

func TestWithGenAIAgentAttrs(t *testing.T) {
	opt := WithGenAIAgentAttrs("MathTutor", "agent-123")
	if opt == nil {
		t.Fatal("WithGenAIAgentAttrs returned nil")
	}
}

func TestWithGenAIToolAttrs(t *testing.T) {
	opt := WithGenAIToolAttrs("calculator", "function", "call_abc")
	if opt == nil {
		t.Fatal("WithGenAIToolAttrs returned nil")
	}
}

func TestInferenceUsageAttrs(t *testing.T) {
	u := InferenceUsage{
		InputTokens:     100,
		OutputTokens:    50,
		ReasoningTokens: 10,
		FinishReasons:   []string{"stop"},
		ResponseID:      "resp-1",
		ResponseModel:   "gpt-4",
	}
	attrs := InferenceUsageAttrs(u)
	if len(attrs) < 4 {
		t.Errorf("want at least 4 attrs, got %d", len(attrs))
	}
}

func TestInferenceUsageAttrsMinimal(t *testing.T) {
	u := InferenceUsage{
		InputTokens:  10,
		OutputTokens: 5,
	}
	attrs := InferenceUsageAttrs(u)
	if len(attrs) != 2 {
		t.Errorf("want 2 attrs (input+output only), got %d", len(attrs))
	}
}

func TestNewMetricsNotNil(t *testing.T) {
	m := NewMetrics()
	if m == nil {
		t.Fatal("NewMetrics returned nil")
	}
	if m.InputTokenCounter == nil {
		t.Error("InputTokenCounter is nil")
	}
	if m.LLMCallDuration == nil {
		t.Error("LLMCallDuration is nil")
	}
}

func TestMetricsNilSafe(t *testing.T) {
	var m *Metrics
	ctx := context.Background()
	// These should not panic
	m.RecordTokenUsage(ctx, "gpt-4", "openai", 100, 50)
	m.RecordLLMCall(ctx, "gpt-4", "openai", 1.5, nil)
	m.RecordToolCall(ctx, "calc", 0.1, nil)
	m.RecordAgentRun(ctx, "agent", 2.0)
}

func TestMetricsRecordOps(t *testing.T) {
	m := NewMetrics()
	ctx := context.Background()
	// These should not panic
	m.RecordTokenUsage(ctx, "gpt-4", "openai", 100, 50)
	m.RecordLLMCall(ctx, "gpt-4", "openai", 1.5, nil)
	m.RecordLLMCall(ctx, "gpt-4", "openai", 0.5, context.DeadlineExceeded)
	m.RecordToolCall(ctx, "calculator", 0.1, nil)
	m.RecordToolCall(ctx, "bad_tool", 0.2, context.Canceled)
	m.RecordAgentRun(ctx, "MathTutor", 2.5)
}

func TestDefaultMetricsGlobal(t *testing.T) {
	if DefaultMetrics == nil {
		t.Fatal("DefaultMetrics is nil")
	}
}
