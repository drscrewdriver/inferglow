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

package schema

import (
	"testing"
)

// Check 2.2.1: ValidateInput 正确校验输入数据
func TestValidateInputValid(t *testing.T) {
	ce := &ContractEngine{
		EnsureAll: false,
	}

	// 空数据应该通过
	err := ce.ValidateInput(nil)
	if err != nil {
		t.Errorf("ValidateInput(nil) = %v, want nil", err)
	}

	// 有效数据应该通过
	validData := map[string]any{
		"name": "test",
		"age":  25,
	}
	err = ce.ValidateInput(validData)
	if err != nil {
		t.Errorf("ValidateInput(valid) = %v, want nil", err)
	}
}

// Check 2.2.2: ValidateResult 正确校验输出数据
func TestValidateResultWithRequiredFields(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"name":  EnsurePresence,
			"email": EnsureNotNull,
		},
		EnsureAll: false,
	}

	validResult := map[string]any{
		"name":  "test",
		"email": "test@example.com",
	}
	err := ce.ValidateResult(validResult)
	if err != nil {
		t.Errorf("ValidateResult(valid) = %v, want nil", err)
	}

	missingEmail := map[string]any{
		"name": "test",
	}
	err = ce.ValidateResult(missingEmail)
	if err == nil {
		t.Error("ValidateResult(missing email) = nil, want error")
	}
}

// Check 2.2.3: EnsureAll=true 时强制所有字段存在
func TestEnsureAllTrue(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"name":  EnsurePresence,
			"age":   EnsurePresence,
			"email": EnsurePresence,
		},
		EnsureAll: true,
	}

	complete := map[string]any{
		"name":  "test",
		"age":   25,
		"email": "test@example.com",
	}
	err := ce.ValidateResult(complete)
	if err != nil {
		t.Errorf("ValidateResult(complete) = %v, want nil", err)
	}

	// 缺少 email 字段
	missingEmail := map[string]any{
		"name": "test",
		"age":  25,
	}
	err = ce.ValidateResult(missingEmail)
	if err == nil {
		t.Error("ValidateResult(missing field with EnsureAll=true) = nil, want error")
	}
}

// Check 2.2.4: 校验失败返回详细 error 信息
func TestValidationErrorDetails(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"name":  EnsurePresence,
			"email": EnsurePresence,
		},
		EnsureAll: true,
	}

	missingData := map[string]any{
		"name": "test",
		// email 缺失
	}

	err := ce.ValidateResult(missingData)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	if errMsg == "" {
		t.Error("error message should not be empty")
	}

	// Error 应该包含缺失字段的名称
	if !containsString(errMsg, "email") {
		t.Errorf("error message %q should mention 'email'", errMsg)
	}
}

// Test ValidateInput with path-based keys
func TestValidateInputWithPathKeys(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"metadata.title": EnsurePresence,
		},
		EnsureAll: false,
	}

	input := map[string]any{
		"metadata": map[string]any{
			"title": "Test Title",
		},
	}

	err := ce.ValidateInput(input)
	if err != nil {
		t.Errorf("ValidateInput with nested data = %v, want nil", err)
	}
}

// Test ValidateResult with nested EnsureAll
func TestValidateResultNestedEnsureAll(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"metadata.title":  EnsurePresence,
			"metadata.author": EnsurePresence,
		},
		EnsureAll: true,
	}

	// 缺少 author 字段
	input := map[string]any{
		"metadata": map[string]any{
			"title": "Test Title",
		},
	}

	err := ce.ValidateResult(input)
	if err == nil {
		t.Error("expected error for missing nested field, got nil")
	}
}

// Test ContractEngine with empty EnsureAll
func TestContractEngineEmptyEnsureAll(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys:   nil,
		EnsureAll:    false,
		InputSchema:  nil,
		ResultSchema: nil,
	}

	err := ce.ValidateResult(map[string]any{})
	if err != nil {
		t.Errorf("empty data should pass validation, got: %v", err)
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && s != "" && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Test EnsurePolicy values
func TestEnsurePolicyValidation(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"name": EnsureNotNull,
		},
	}

	// name 为 nil 应该失败
	data := map[string]any{
		"name": nil,
	}
	err := ce.ValidateResult(data)
	if err == nil {
		t.Error("expected error for null value with EnsureNotNull, got nil")
	}

	// name 为非 nil 应该通过
	data2 := map[string]any{
		"name": "value",
	}
	err = ce.ValidateResult(data2)
	if err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

// Test ContractEngine GenerateJSONSchema returns map
func TestContractEngineHasJSONSchemaMethod(t *testing.T) {
	ce := &ContractEngine{
		InputSchema:  map[string]any{"type": "object"},
		ResultSchema: map[string]any{"type": "object"},
	}

	schema := ce.GenerateJSONSchema()
	if schema == nil {
		t.Error("GenerateJSONSchema() should not return nil")
	}
}

// TestValidateData_NonMapReturnsError is a regression test for SC-HIGH-1:
// validateData previously returned nil for any non-map data, so string,
// array, or number responses silently passed validation. The fix returns
// an error describing the actual type received.
func TestValidateData_NonMapReturnsError(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"name": EnsurePresence,
		},
		EnsureAll: true,
	}

	cases := []struct {
		name string
		data any
	}{
		{"string", "not a map"},
		{"int", 42},
		{"slice", []any{"a", "b"}},
		{"struct", struct{ X int }{X: 1}},
		{"bool", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ce.ValidateResult(tc.data); err == nil {
				t.Errorf("ValidateResult(%v) = nil, want error for %s", tc.data, tc.name)
			} else if !findSubstring(err.Error(), "expected map[string]any") {
				t.Errorf("ValidateResult(%v) error = %q, want it to contain 'expected map[string]any'", tc.data, err.Error())
			}
			// ValidateInput shares the same code path.
			if err := ce.ValidateInput(tc.data); err == nil {
				t.Errorf("ValidateInput(%v) = nil, want error for %s", tc.data, tc.name)
			}
		})
	}
}

// TestValidateData_NilStillPasses confirms the SC-HIGH-1 fix preserves the
// documented "nil data short-circuits to nil" behavior, which existing
// callers (e.g. optional response fields) rely on.
func TestValidateData_NilStillPasses(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"name": EnsurePresence,
		},
		EnsureAll: true,
	}
	if err := ce.ValidateResult(nil); err != nil {
		t.Errorf("ValidateResult(nil) = %v, want nil", err)
	}
	if err := ce.ValidateInput(nil); err != nil {
		t.Errorf("ValidateInput(nil) = %v, want nil", err)
	}
}
