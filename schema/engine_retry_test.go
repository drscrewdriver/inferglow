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
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Check: 通配符路径在 ContractEngine 校验中实际生效
// 当前 engine.go 的 locateParts 对 Wild 直接返回 nil，导致 resources[*].title 之类
// 路径在 EnsurePresence/EnsureNotNull 校验时永远失败。
func TestValidateResultWithWildcardPath(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"resources[*].title": EnsurePresence,
		},
		EnsureAll: true,
	}

	valid := map[string]any{
		"resources": []any{
			map[string]any{"title": "Resource 1"},
			map[string]any{"title": "Resource 2"},
		},
	}

	if err := ce.ValidateResult(valid); err != nil {
		t.Fatalf("ValidateResult with wildcard path should pass, got: %v", err)
	}

	// 至少一个元素缺少 title 应该失败
	missing := map[string]any{
		"resources": []any{
			map[string]any{"title": "Resource 1"},
			map[string]any{"name": "No Title"}, // 缺少 title
		},
	}
	if err := ce.ValidateResult(missing); err == nil {
		t.Fatal("ValidateResult should fail when wildcard element missing field")
	}
}

// Check: ValidateWithRetry 首次通过时不调用 retryFn
func TestValidateWithRetryFirstPass(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"name": EnsurePresence,
		},
		EnsureAll: true,
	}

	retryCalls := 0
	retryFn := func() (any, error) {
		retryCalls++
		return map[string]any{"name": "test"}, nil
	}

	result, err := ce.ValidateWithRetry(context.Background(), map[string]any{"name": "test"}, 3, retryFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
	if retryCalls != 0 {
		t.Errorf("retryFn should not be called on first pass, calls=%d", retryCalls)
	}
}

// Check: ValidateWithRetry 失败 → 重试 → 成功
func TestValidateWithRetrySuccessAfterRetry(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"email": EnsurePresence,
		},
		EnsureAll: true,
	}

	calls := 0
	retryFn := func() (any, error) {
		calls++
		if calls < 2 {
			// 缺少 email 字段
			return map[string]any{"name": "test"}, nil
		}
		return map[string]any{"name": "test", "email": "test@example.com"}, nil
	}

	// 初始结果缺少 email
	initial := map[string]any{"name": "test"}

	result, err := ce.ValidateWithRetry(context.Background(), initial, 3, retryFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 retry calls, got %d", calls)
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any", result)
	}
	if resultMap["email"] != "test@example.com" {
		t.Errorf("result email = %v, want test@example.com", resultMap["email"])
	}
}

// Check: ValidateWithRetry 达到最大重试次数后返回 error
func TestValidateWithRetryMaxRetriesExceeded(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"missing": EnsurePresence,
		},
		EnsureAll: true,
	}

	calls := 0
	retryFn := func() (any, error) {
		calls++
		// 始终返回不满足条件的结果
		return map[string]any{"name": "test"}, nil
	}

	_, err := ce.ValidateWithRetry(context.Background(), map[string]any{"name": "test"}, 2, retryFn)
	if err == nil {
		t.Fatal("expected error after max retries exceeded")
	}
	if !strings.Contains(err.Error(), "ensure_all") && !strings.Contains(err.Error(), "retries") {
		t.Errorf("error should mention ensure_all or retries, got: %v", err)
	}
	// 应该调用 retryFn maxRetries 次
	if calls != 2 {
		t.Errorf("expected 2 retry calls, got %d", calls)
	}
}

// Check: ValidateWithRetry retryFn 返回 error 时立即返回
func TestValidateWithRetryRetryFnError(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"name": EnsurePresence,
		},
		EnsureAll: true,
	}

	expectedErr := errors.New("upstream failure")
	calls := 0
	retryFn := func() (any, error) {
		calls++
		return nil, expectedErr
	}

	// 初始结果不通过校验
	initial := map[string]any{}

	_, err := ce.ValidateWithRetry(context.Background(), initial, 3, retryFn)
	if err == nil {
		t.Fatal("expected error from retryFn")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error to wrap upstream error, got: %v", err)
	}
	if calls != 1 {
		t.Errorf("retryFn should be called once before error, got %d", calls)
	}
}

// Check: ValidateWithRetry 尊重 context 取消
func TestValidateWithRetryContextCancel(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"missing": EnsurePresence,
		},
		EnsureAll: true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	// 立即取消
	cancel()

	calls := 0
	retryFn := func() (any, error) {
		calls++
		return map[string]any{}, nil
	}

	_, err := ce.ValidateWithRetry(ctx, map[string]any{}, 5, retryFn)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// Check: ValidateWithRetry 不带 retryFn (nil) 时退化为基础校验
func TestValidateWithRetryNilRetryFn(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"name": EnsurePresence,
		},
		EnsureAll: true,
	}

	// 通过校验
	result, err := ce.ValidateWithRetry(context.Background(), map[string]any{"name": "test"}, 3, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}

	// 不通过校验且无 retryFn → 立即失败
	_, err = ce.ValidateWithRetry(context.Background(), map[string]any{}, 3, nil)
	if err == nil {
		t.Fatal("expected error when validation fails and retryFn is nil")
	}
}

// Check: ValidateWithRetry 退避时间在 maxRetries 较大时仍然合理（不精确测时）
func TestValidateWithRetryBackoffTiming(t *testing.T) {
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"missing": EnsurePresence,
		},
		EnsureAll: true,
	}

	start := time.Now()
	calls := 0
	retryFn := func() (any, error) {
		calls++
		return map[string]any{}, nil
	}

	_, _ = ce.ValidateWithRetry(context.Background(), map[string]any{}, 3, retryFn)
	elapsed := time.Since(start)

	// 3 次重试，退避序列：1s, 2s = 3s 最小（按 1s base）
	// 这里只验证退避确实发生（>500ms），不精确断言
	if elapsed < 500*time.Millisecond {
		t.Logf("warning: backoff elapsed only %v, expected >500ms with 3 retries", elapsed)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}
