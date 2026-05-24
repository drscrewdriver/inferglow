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
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// OutputValidator 输出验证器，校验 LLM 输出是否符合 OutputSchema
type OutputValidator struct {
	Schema      *OutputSchema
	MaxRetries  int
	BackoffBase float64
}

// NewOutputValidator 创建新的验证器
func NewOutputValidator(schema *OutputSchema) *OutputValidator {
	return &OutputValidator{
		Schema:     schema,
		MaxRetries: 3,
	}
}

// ResponseFetcher is a callback that produces a fresh ModelResponse on each
// call. It is used by ValidateAndRetryWithFetch to re-fetch the model output
// when validation fails, so each retry sees a freshly generated response
// rather than re-validating the same stale one.
type ResponseFetcher func(ctx context.Context) (*ModelResponse, error)

// ValidateAndRetry 校验输出是否符合 Schema，失败时自动重试
//
// Note: this method re-validates the SAME response object across retries.
// For most cases the caller actually wants a fresh response per retry — use
// ValidateAndRetryWithFetch for that.
func (v *OutputValidator) ValidateAndRetry(ctx context.Context, response *ModelResponse) (*ModelResponse, error) {
	if v.Schema == nil || response == nil {
		return response, nil
	}

	runner := NewAttemptRunner()
	runner.MaxAttempts = v.MaxRetries + 1 // +1 包含第一次尝试

	var lastErr error
	for i := 0; i < runner.MaxAttempts; i++ {
		err := v.validate(response)
		if err == nil {
			return response, nil
		}

		lastErr = err

		// 达到最大次数
		if i >= runner.MaxAttempts-1 {
			break
		}

		// 等待重试
		backoff := float64(v.BackoffBase) * math.Pow(2, float64(i))
		if backoff > float64(30) {
			backoff = 30
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(backoff) * time.Second):
			// 继续重试
		}
	}

	return nil, fmt.Errorf("validation failed after %d attempts: %w", v.MaxRetries+1, lastErr)
}

// ValidateAndRetryWithFetch validates a freshly fetched ModelResponse against
// the schema, re-invoking fetcher on validation failure. This fixes BUG-5:
// the original ValidateAndRetry only re-validated the same stale response,
// so a single bad generation could never recover.
//
// Behavior:
//   - If Schema is nil: invoke fetcher once, return the response unchanged.
//   - If fetcher returns an error: wrap and return immediately (no retry on
//     transport-level failures — that's the AttemptRunner's responsibility).
//   - If validation fails: wait (with exponential backoff), call fetcher
//     again, and re-validate. Repeat up to MaxRetries+1 attempts total.
//   - Respects ctx cancellation during both backoff and fetch.
func (v *OutputValidator) ValidateAndRetryWithFetch(ctx context.Context, fetcher ResponseFetcher) (*ModelResponse, error) {
	if fetcher == nil {
		return nil, fmt.Errorf("fetcher cannot be nil")
	}

	// nil Schema: still call fetcher once so the caller sees a response,
	// but skip validation entirely.
	if v.Schema == nil {
		return fetcher(ctx)
	}

	runner := NewAttemptRunner()
	runner.MaxAttempts = v.MaxRetries + 1 // +1 包含第一次尝试

	var lastErr error
	for i := 0; i < runner.MaxAttempts; i++ {
		// Re-fetch on every attempt so a retry sees fresh model output.
		resp, err := fetcher(ctx)
		if err != nil {
			return nil, fmt.Errorf("fetch response (attempt %d): %w", i+1, err)
		}
		if resp == nil {
			return nil, fmt.Errorf("fetch returned nil response (attempt %d)", i+1)
		}

		if vErr := v.validate(resp); vErr == nil {
			return resp, nil
		} else {
			lastErr = vErr
		}

		// 达到最大次数
		if i >= runner.MaxAttempts-1 {
			break
		}

		// 等待重试
		backoff := float64(v.BackoffBase) * math.Pow(2, float64(i))
		if backoff > float64(30) {
			backoff = 30
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(backoff) * time.Second):
			// 继续重试
		}
	}

	return nil, fmt.Errorf("validation failed after %d attempts: %w", v.MaxRetries+1, lastErr)
}

// validate 校验一次响应是否符合 Schema
func (v *OutputValidator) validate(response *ModelResponse) error {
	if response == nil {
		return fmt.Errorf("response is nil")
	}

	// 基础校验：检查内容是否存在
	if v.Schema.Type == "required_content" {
		if response.Content == "" && len(response.Tools) == 0 {
			return fmt.Errorf("response content and tools are both empty")
		}
	}

	// 检查 required 字段
	if len(v.Schema.Required) > 0 {
		for _, field := range v.Schema.Required {
			switch field {
			case "content":
				if response.Content == "" {
					return fmt.Errorf("missing required field: content")
				}
			case "reasoning":
				if response.Reasoning == "" {
					return fmt.Errorf("missing required field: reasoning")
				}
			case "tools":
				if len(response.Tools) == 0 {
					return fmt.Errorf("missing required field: tools")
				}
			}
		}
	}

	// L4: JSON structure validation (only when Properties is non-empty)
	if len(v.Schema.Properties) > 0 && response.Content != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(response.Content), &parsed); err != nil {
			return fmt.Errorf("L4 validation: content is not valid JSON: %w", err)
		}
		// Check Required field presence
		for _, field := range v.Schema.Required {
			if _, ok := parsed[field]; !ok {
				return fmt.Errorf("L4 validation: missing required field %q in JSON output", field)
			}
		}
		// Check field types (loose mode)
		for name, propDef := range v.Schema.Properties {
			if val, ok := parsed[name]; ok {
				if err := checkFieldType(name, val, propDef); err != nil {
					return fmt.Errorf("L4 validation: %w", err)
				}
			}
		}
	}

	return nil
}

// checkFieldType validates that a JSON value matches the expected type
// defined in the schema property definition.
func checkFieldType(name string, value any, propDef any) error {
	def, ok := propDef.(map[string]any)
	if !ok {
		return nil
	}
	expectedType, _ := def["type"].(string)
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("field %q: expected string, got %T", name, value)
		}
	case "integer", "number":
		if _, ok := value.(float64); !ok {
			return fmt.Errorf("field %q: expected number, got %T", name, value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("field %q: expected boolean, got %T", name, value)
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("field %q: expected object, got %T", name, value)
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("field %q: expected array, got %T", name, value)
		}
	}
	return nil
}
