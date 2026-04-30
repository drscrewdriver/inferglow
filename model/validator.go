package model

import (
	"context"
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

// ValidateAndRetry 校验输出是否符合 Schema，失败时自动重试
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

	return nil
}
