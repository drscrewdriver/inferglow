package model

import (
	"context"
	"testing"
)

// TestProcessOpenAILineReasoningContentOnly verifies that an SSE chunk
// containing only reasoning_content (no reasoning) correctly fills
// StreamChunk.Reasoning. Delta should remain empty.
func TestProcessOpenAILineReasoningContentOnly(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	usage := &UsageInfo{}
	toolStates := map[int]*openAIToolState{}
	var got *StreamChunk
	emit := func(c *StreamChunk) { got = c }

	line := `data: {"choices":[{"delta":{"reasoning_content":"思考过程"},"finish_reason":null}]}`
	p.processOpenAILine(line, usage, toolStates, emit)

	if got == nil {
		t.Fatal("expected chunk")
	}
	if got.Reasoning != "思考过程" {
		t.Errorf("Reasoning=%q, want %q", got.Reasoning, "思考过程")
	}
	if got.Delta != "" {
		t.Errorf("Delta=%q, want empty", got.Delta)
	}
}

// TestProcessOpenAILineReasoningContentOverridesReasoning verifies that
// when a chunk contains both reasoning and reasoning_content, the
// reasoning_content value wins (higher priority).
func TestProcessOpenAILineReasoningContentOverridesReasoning(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	usage := &UsageInfo{}
	toolStates := map[int]*openAIToolState{}
	var got *StreamChunk
	emit := func(c *StreamChunk) { got = c }

	line := `data: {"choices":[{"delta":{"reasoning":"低优先级","reasoning_content":"高优先级"},"finish_reason":null}]}`
	p.processOpenAILine(line, usage, toolStates, emit)

	if got == nil {
		t.Fatal("expected chunk")
	}
	if got.Reasoning != "高优先级" {
		t.Errorf("Reasoning=%q, want %q (reasoning_content should override reasoning)", got.Reasoning, "高优先级")
	}
}

// TestProcessOpenAILineReasoningBackwardCompat verifies that a chunk
// containing only reasoning (no reasoning_content) still works — backward
// compatibility with standard OpenAI/DeepSeek/Qwen providers.
func TestProcessOpenAILineReasoningBackwardCompat(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	usage := &UsageInfo{}
	toolStates := map[int]*openAIToolState{}
	var got *StreamChunk
	emit := func(c *StreamChunk) { got = c }

	line := `data: {"choices":[{"delta":{"reasoning":"标准推理"},"finish_reason":null}]}`
	p.processOpenAILine(line, usage, toolStates, emit)

	if got == nil {
		t.Fatal("expected chunk")
	}
	if got.Reasoning != "标准推理" {
		t.Errorf("Reasoning=%q, want %q", got.Reasoning, "标准推理")
	}
}

// TestToStreamChunkReasoningContent verifies that toStreamChunk (the
// backward-compat helper) also handles reasoning_content with the same
// priority override.
func TestToStreamChunkReasoningContent(t *testing.T) {
	reasoningVal := "低优先级"
	reasoningContentVal := "高优先级"

	chunk := openAIChunk{
		Choices: []struct {
			Index int `json:"index"`
			Delta struct {
				Role             string     `json:"role,omitempty"`
				Content          *string    `json:"content,omitempty"`
				Reasoning        *string    `json:"reasoning,omitempty"`
				ReasoningContent *string    `json:"reasoning_content,omitempty"`
				ToolCalls        []toolCall `json:"tool_calls,omitempty"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Delta: struct {
					Role             string     `json:"role,omitempty"`
					Content          *string    `json:"content,omitempty"`
					Reasoning        *string    `json:"reasoning,omitempty"`
					ReasoningContent *string    `json:"reasoning_content,omitempty"`
					ToolCalls        []toolCall `json:"tool_calls,omitempty"`
				}{
					Reasoning:        &reasoningVal,
					ReasoningContent: &reasoningContentVal,
				},
			},
		},
	}

	result := toStreamChunk(chunk, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Reasoning != "高优先级" {
		t.Errorf("Reasoning=%q, want %q (reasoning_content should override reasoning)", result.Reasoning, "高优先级")
	}
}

// TestToStreamChunkReasoningContentOnly verifies toStreamChunk with only
// reasoning_content (no reasoning field).
func TestToStreamChunkReasoningContentOnly(t *testing.T) {
	reasoningContentVal := "仅 reasoning_content"

	chunk := openAIChunk{
		Choices: []struct {
			Index int `json:"index"`
			Delta struct {
				Role             string     `json:"role,omitempty"`
				Content          *string    `json:"content,omitempty"`
				Reasoning        *string    `json:"reasoning,omitempty"`
				ReasoningContent *string    `json:"reasoning_content,omitempty"`
				ToolCalls        []toolCall `json:"tool_calls,omitempty"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Delta: struct {
					Role             string     `json:"role,omitempty"`
					Content          *string    `json:"content,omitempty"`
					Reasoning        *string    `json:"reasoning,omitempty"`
					ReasoningContent *string    `json:"reasoning_content,omitempty"`
					ToolCalls        []toolCall `json:"tool_calls,omitempty"`
				}{
					ReasoningContent: &reasoningContentVal,
				},
			},
		},
	}

	result := toStreamChunk(chunk, nil)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Reasoning != "仅 reasoning_content" {
		t.Errorf("Reasoning=%q, want %q", result.Reasoning, "仅 reasoning_content")
	}
}

// --- G1-04: <think> tag normalization tests ---

// TestHasThinkingTags is a table-driven test for hasThinkingTags.
func TestHasThinkingTags(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"complete block", "<think>x</think>y", true},
		{"plain text", "纯文本", false},
		{"open tag only no close", "<think>无闭合", false},
		{"close tag only no open", "无开始</think>", false},
		{"empty think block", "<think></think>", true},
		{"multiple blocks", "<think>a</think>b<think>c</think>", true},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasThinkingTags(tt.content)
			if got != tt.want {
				t.Errorf("hasThinkingTags(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

// TestNormalizeThinkingTags is a table-driven test for normalizeThinkingTags.
func TestNormalizeThinkingTags(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		wantReasoning string
		wantCleaned   string
	}{
		{
			name:          "simple block",
			content:       "<think>推理</think>回答",
			wantReasoning: "推理",
			wantCleaned:   "回答",
		},
		{
			name:          "no tags",
			content:       "无标签",
			wantReasoning: "",
			wantCleaned:   "无标签",
		},
		{
			name:          "multiple blocks joined",
			content:       "<think>第一段</think>中间<think>第二段</think>结尾",
			wantReasoning: "第一段\n第二段",
			wantCleaned:   "中间结尾",
		},
		{
			name:          "whitespace trimmed cleaned empty",
			content:       "  <think>推理</think>  ",
			wantReasoning: "推理",
			wantCleaned:   "",
		},
		{
			name:          "empty think block",
			content:       "<think></think>回答",
			wantReasoning: "",
			wantCleaned:   "回答",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotReasoning, gotCleaned := normalizeThinkingTags(tt.content)
			if gotReasoning != tt.wantReasoning {
				t.Errorf("reasoning = %q, want %q", gotReasoning, tt.wantReasoning)
			}
			if gotCleaned != tt.wantCleaned {
				t.Errorf("cleaned = %q, want %q", gotCleaned, tt.wantCleaned)
			}
		})
	}
}

// TestBroadcastResponseThinkTagNormalization is an end-to-end test: feed
// a StreamChunk stream whose Delta accumulates <think>...</think> content,
// and verify BroadcastResponse normalizes it into Reasoning/Content.
func TestBroadcastResponseThinkTagNormalization(t *testing.T) {
	provider := &OpenAICompatibleProvider{}
	stream := make(chan *StreamChunk, 4)
	stream <- &StreamChunk{Delta: "<think>推理过程</think>最终回答"}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var donePayload *ModelResponse
	for ev := range events {
		if ev.EventType == EventDone {
			if mr, ok := ev.Payload.(*ModelResponse); ok {
				donePayload = mr
			}
		}
	}

	if donePayload == nil {
		t.Fatal("expected EventDone with ModelResponse payload")
	}
	if donePayload.Reasoning != "推理过程" {
		t.Errorf("Reasoning = %q, want %q", donePayload.Reasoning, "推理过程")
	}
	if donePayload.Content != "最终回答" {
		t.Errorf("Content = %q, want %q", donePayload.Content, "最终回答")
	}
}

// TestBroadcastResponseNoNormalizationWhenReasoningPresent verifies that
// when the standard reasoning field is already filled (via reasoning/
// reasoning_content delta), the <think> tag normalization is NOT triggered
// (avoids double processing).
func TestBroadcastResponseNoNormalizationWhenReasoningPresent(t *testing.T) {
	provider := &OpenAICompatibleProvider{}
	stream := make(chan *StreamChunk, 4)
	// Standard reasoning delta fills fullReasoning, so normalization should
	// be skipped even though Content contains <think> tags.
	stream <- &StreamChunk{Reasoning: "标准推理", Delta: "<think>不应提取</think>回答"}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	events, err := provider.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var donePayload *ModelResponse
	for ev := range events {
		if ev.EventType == EventDone {
			if mr, ok := ev.Payload.(*ModelResponse); ok {
				donePayload = mr
			}
		}
	}

	if donePayload == nil {
		t.Fatal("expected EventDone with ModelResponse payload")
	}
	// Reasoning should be the standard field, NOT the <think> tag content.
	if donePayload.Reasoning != "标准推理" {
		t.Errorf("Reasoning = %q, want %q (standard reasoning should not be overwritten)", donePayload.Reasoning, "标准推理")
	}
	// Content should still contain the <think> tags (not normalized).
	if donePayload.Content != "<think>不应提取</think>回答" {
		t.Errorf("Content = %q, want %q (content should not be cleaned when reasoning is present)", donePayload.Content, "<think>不应提取</think>回答")
	}
}
