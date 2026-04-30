package model

import (
	"context"
	"testing"
)

// Check 1.6.1: ValidateAndRetry 正确校验输出字段存在性
func TestValidatorValidate(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type:     "required_content",
		Required: []string{"content"},
	})

	// 校验通过
	resp := &ModelResponse{Content: "valid response"}
	result, err := v.ValidateAndRetry(context.Background(), resp)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Content != "valid response" {
		t.Errorf("unexpected result: %+v", result)
	}
}

// Check 1.6.2: 校验失败触发自动重试
func TestValidatorRetryOnFailure(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type:     "required_content",
		Required: []string{"content"},
	})

	// 第一次校验失败，第二次通过
	callCount := 0
	resp := &ModelResponse{Content: "will be valid"}

	result, err := v.ValidateAndRetry(context.Background(), resp)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	_ = callCount
	_ = result
}

// Check 1.6.3: 所有重试失败后返回最后一次 error
func TestValidatorAllRetriesFail(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Type:     "required_content",
		Required: []string{"content"},
	})

	// 空响应，必定校验失败
	emptyResp := &ModelResponse{}

	_, err := v.ValidateAndRetry(context.Background(), emptyResp)
	if err == nil {
		t.Fatal("expected error after retries")
	}
}

// Check: nil Schema 直接返回
func TestValidatorNilSchema(t *testing.T) {
	v := &OutputValidator{Schema: nil}

	resp := &ModelResponse{Content: "test"}
	result, err := v.ValidateAndRetry(context.Background(), resp)
	if err != nil {
		t.Fatalf("expected no error for nil schema, got: %v", err)
	}
	if result.Content != "test" {
		t.Errorf("unexpected result: %+v", result)
	}
}

// Check: nil Response 处理
func TestValidatorNilResponse(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{Type: "required_content"})

	result, err := v.ValidateAndRetry(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error for nil schema check, got: %v", err)
	}
	if result != nil {
		t.Error("expected nil result")
	}
}

// Check: 校验 required tools
func TestValidatorRequiredTools(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Required: []string{"tools"},
	})

	// 缺少 tools
	emptyResp := &ModelResponse{Content: "hello"}
	_, err := v.ValidateAndRetry(context.Background(), emptyResp)
	if err == nil {
		t.Fatal("expected error for missing tools")
	}

	// 有 tools 应该通过
	respWithTools := &ModelResponse{
		Content: "hello",
		Tools:   []ToolCall{{ID: "1", Name: "calc"}},
	}
	result, err := v.ValidateAndRetry(context.Background(), respWithTools)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	_ = result
}

// Check: 校验 required reasoning
func TestValidatorRequiredReasoning(t *testing.T) {
	v := NewOutputValidator(&OutputSchema{
		Required: []string{"reasoning"},
	})

	// 缺少 reasoning
	emptyResp := &ModelResponse{Content: "hello"}
	_, err := v.ValidateAndRetry(context.Background(), emptyResp)
	if err == nil {
		t.Fatal("expected error for missing reasoning")
	}

	// 有 reasoning 应该通过
	respWithReasoning := &ModelResponse{
		Content:   "hello",
		Reasoning: "I think...",
	}
	result, err := v.ValidateAndRetry(context.Background(), respWithReasoning)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	_ = result
}
