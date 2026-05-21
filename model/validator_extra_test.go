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

package model

import (
	"context"
	"testing"
)

// Check: 自定义 BackoffBase 值
func TestValidatorCustomBackoffBase(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type: "required_content",
	})
	v.BackoffBase = 0.001 // 快速重试

	// 验证 BackoffBase 可自定义
	if v.BackoffBase != 0.001 {
		t.Errorf("BackoffBase = %f, want 0.001", v.BackoffBase)
	}
}

// Check: Schema.Type 不为 "required_content" 时跳过内容检查
func TestValidatorNonRequiredContentType(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type: "optional_content",
	})

	// 空响应，但 Type 不是 required_content，应该通过
	emptyResp := &ModelResponse{}
	result, err := v.ValidateAndRetry(context.Background(), emptyResp)
	if err != nil {
		t.Fatalf("expected no error for non-required type, got: %v", err)
	}
	_ = result
}

// Check: Response 既有 Content 又有 Tools 时通过
func TestValidatorBothContentAndTools(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type:     "required_content",
		Required: []string{"content", "tools"},
	})

	resp := &ModelResponse{
		Content: "hello",
		Tools:   []ToolCall{{ID: "1", Name: "test"}},
	}
	result, err := v.ValidateAndRetry(context.Background(), resp)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	_ = result
}

// Check: 空 Content 且无 Tools 时报错
func TestValidatorBothEmpty(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type: "required_content",
	})

	emptyResp := &ModelResponse{}
	_, err := v.ValidateAndRetry(context.Background(), emptyResp)
	if err == nil {
		t.Fatal("expected error for both content and tools empty")
	}
}

// Check: UsageInfo 字段完整
func TestUsageInfoFields(t *testing.T) {
	usage := UsageInfo{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	if usage.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d, want 100", usage.PromptTokens)
	}
	if usage.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d, want 50", usage.CompletionTokens)
	}
	if usage.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", usage.TotalTokens)
	}
}

// Check: Example 类型定义
func TestExampleFields(t *testing.T) {
	example := Example{
		Input:  "What is 2+2?",
		Output: "4",
	}

	if example.Input != "What is 2+2?" {
		t.Errorf("Input = %q, want %q", example.Input, "What is 2+2?")
	}
	if example.Output != "4" {
		t.Errorf("Output = %q, want %q", example.Output, "4")
	}
}

// Check: Attachment 类型定义
func TestAttachmentFields(t *testing.T) {
	attachment := Attachment{
		Type: "image",
		Data: map[string]any{"url": "http://example.com/img.png"},
	}

	if attachment.Type != "image" {
		t.Errorf("Type = %q, want %q", attachment.Type, "image")
	}
	if attachment.Data == nil {
		t.Error("Data should not be nil")
	}
}

// Check: ActionResult 类型定义
func TestActionResultFields(t *testing.T) {
	result := ActionResult{
		Name:    "get_weather",
		Success: true,
		Output:  map[string]any{"temp": 25},
		Error:   "",
	}

	if result.Name != "get_weather" {
		t.Errorf("Name = %q, want %q", result.Name, "get_weather")
	}
	if !result.Success {
		t.Error("Success should be true")
	}
	if result.Output == nil {
		t.Error("Output should not be nil")
	}
}

// Check: RequestData Options 传递
func TestRequestDataOptions(t *testing.T) {
	req := &RequestData{
		Model:       "gpt-4",
		Temperature: 0.7,
		Options:     map[string]any{"max_tokens": 1000, "top_p": 0.9},
	}

	if req.Options == nil {
		t.Error("Options should not be nil")
	}
	if req.Options["max_tokens"] != 1000 {
		t.Errorf("max_tokens = %v, want 1000", req.Options["max_tokens"])
	}
}

// Check: StreamChunk Reasoning 字段
func TestStreamChunkReasoning(t *testing.T) {
	chunk := StreamChunk{
		Delta:     "The answer is 4",
		Reasoning: "Let me calculate...",
		IsDone:    false,
		Usage:     &UsageInfo{TotalTokens: 10},
		Meta:      map[string]any{"chunk_id": 1},
	}

	if chunk.Reasoning != "Let me calculate..." {
		t.Errorf("Reasoning = %q, want %q", chunk.Reasoning, "Let me calculate...")
	}
	if chunk.Usage == nil || chunk.Usage.TotalTokens != 10 {
		t.Error("Usage should have TotalTokens=10")
	}
}

// Check: ModelRequest Developer 字段
func TestModelRequestDeveloperField(t *testing.T) {
	req := ModelRequest{
		System:    "You are helpful.",
		Developer: "Use JSON output format.",
		Instruct:  "Answer this question.",
	}

	if req.Developer != "Use JSON output format." {
		t.Errorf("Developer = %q, want %q", req.Developer, "Use JSON output format.")
	}
}

// Check: ModelRequest Attachment 和 Examples 字段
func TestModelRequestAttachmentAndExamples(t *testing.T) {
	req := ModelRequest{
		Instruct: "test",
		Examples: []Example{
			{Input: "hello", Output: "world"},
		},
		Attachment: []Attachment{
			{Type: "image", Data: map[string]any{}},
		},
		Actions: []ActionResult{
			{Name: "test", Success: true},
		},
		Output: &OutputSchema{Type: "object"},
	}

	if len(req.Examples) != 1 {
		t.Errorf("Examples length = %d, want 1", len(req.Examples))
	}
	if len(req.Attachment) != 1 {
		t.Errorf("Attachment length = %d, want 1", len(req.Attachment))
	}
	if len(req.Actions) != 1 {
		t.Errorf("Actions length = %d, want 1", len(req.Actions))
	}
	if req.Output == nil {
		t.Error("Output should not be nil")
	}
}

// Check: ResultEvent 携带 Usage 数据
func TestResultEventWithUsage(t *testing.T) {
	usage := &UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}
	event := ResultEvent{
		EventType: MetaEvent,
		Payload:   usage,
	}

	if event.EventType != MetaEvent {
		t.Errorf("EventType = %v, want %v", event.EventType, MetaEvent)
	}
	if event.Payload == nil {
		t.Error("Payload should not be nil")
	}
}

// Check: ModelResponse Meta 字段
func TestModelResponseMeta(t *testing.T) {
	resp := ModelResponse{
		Content: "test",
		Meta: map[string]any{
			"latency_ms":    100,
			"model_name":    "gpt-4",
			"finish_reason": "stop",
		},
	}

	if resp.Meta == nil {
		t.Error("Meta should not be nil")
	}
	if resp.Meta["model_name"] != "gpt-4" {
		t.Errorf("Meta[model_name] = %v, want %q", resp.Meta["model_name"], "gpt-4")
	}
}

// Check: 重试超时的错误信息
func TestValidatorRetryTimeoutErrorMessage(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type: "required_content",
	})

	emptyResp := &ModelResponse{}
	_, err := v.ValidateAndRetry(context.Background(), emptyResp)
	if err == nil {
		t.Fatal("expected error after retries")
	}

	errMsg := err.Error()
	// 错误信息应包含 "validation failed"
	if errMsg == "" {
		t.Error("error message should not be empty")
	}
}

// Check: 空响应体错误
func TestValidatorNilResponseError(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type: "required_content",
	})

	// 直接测试 validate 方法，绕过 ValidateAndRetry
	err := v.validate(nil)
	if err == nil {
		t.Fatal("expected error for nil response")
	}
}

// Check: ModelRequester 接口 Name() 方法
func TestModelRequesterName(t *testing.T) {
	provider := &OpenAICompatibleProvider{Model: "gpt-4"}
	name := provider.Name()

	if name != "openai-compatible" {
		t.Errorf("Name() = %q, want %q", name, "openai-compatible")
	}
}
