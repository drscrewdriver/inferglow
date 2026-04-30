package schema

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ContractEngine 核心验证引擎
type ContractEngine struct {
	InputSchema  map[string]any
	ResultSchema map[string]any
	EnsureKeys   map[string]EnsurePolicy
	EnsureAll    bool
}

// ValidateWithRetry 在校验失败时调用 retryFn 重新获取结果并再次校验，
// 最多重试 maxRetries 次（指数退避 base=1s）。retryFn 为 nil 时退化为单次校验。
// 行为：
//   - 首次校验通过：直接返回 result，不调用 retryFn。
//   - retryFn 返回 error：立即返回该 error（包装）。
//   - 重试期间 ctx 取消：返回 ctx.Err()。
//   - 所有重试均失败：返回 "ensure_all failed after N retries" 错误。
func (ce *ContractEngine) ValidateWithRetry(
	ctx context.Context,
	result any,
	maxRetries int,
	retryFn func() (any, error),
) (any, error) {
	// 首次校验
	if err := ce.ValidateResult(result); err == nil {
		return result, nil
	}

	// 无重试函数 → 直接返回首次校验失败
	if retryFn == nil || maxRetries <= 0 {
		return nil, fmt.Errorf("ensure_all failed: validation error (no retry)")
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// 指数退避：1s, 2s, 4s ...
		backoff := time.Duration(1<<(attempt-1)) * time.Second
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}

		// 重新获取结果
		newResult, err := retryFn()
		if err != nil {
			return nil, fmt.Errorf("ensure_all retry %d/%d failed: %w", attempt, maxRetries, err)
		}

		if err := ce.ValidateResult(newResult); err == nil {
			return newResult, nil
		} else {
			lastErr = err
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("ensure_all failed after %d retries: %w", maxRetries, lastErr)
	}
	return nil, fmt.Errorf("ensure_all failed after %d retries", maxRetries)
}

// GenerateJSONSchema 生成 JSON Schema
func (ce *ContractEngine) GenerateJSONSchema() map[string]any {
	if ce.ResultSchema != nil {
		return ce.ResultSchema
	}
	return map[string]any{
		"type": "object",
	}
}

// ValidateInput 校验输入数据
func (ce *ContractEngine) ValidateInput(data any) error {
	return ce.validateData(data, ce.EnsureKeys, ce.EnsureAll)
}

// ValidateResult 校验输出结果
func (ce *ContractEngine) ValidateResult(data any) error {
	return ce.validateData(data, ce.EnsureKeys, ce.EnsureAll)
}

// validateData 内部验证逻辑
func (ce *ContractEngine) validateData(data any, ensureKeys map[string]EnsurePolicy, ensureAll bool) error {
	if data == nil {
		return nil
	}

	dict, ok := data.(map[string]any)
	if !ok {
		// 非 map 类型，跳过校验
		return nil
	}

	if len(ensureKeys) == 0 {
		return nil
	}

	var missingFields []string
	for path, policy := range ensureKeys {
		switch policy {
		case EnsurePresence:
			if !ensurePathExists(dict, path) {
				missingFields = append(missingFields, path)
			}
		case EnsureNotNull:
			if !checkNotNull(dict, path) {
				missingFields = append(missingFields, path)
			}
		}
	}

	if ensureAll && len(missingFields) > 0 {
		return fmt.Errorf("ensure all failed: missing fields: %s", strings.Join(missingFields, ", "))
	}

	if !ensureAll && len(missingFields) > 0 {
		return fmt.Errorf("validation failed: missing fields: %s", strings.Join(missingFields, ", "))
	}

	return nil
}

// ensurePathExists 检查路径是否存在（叶节点值可为 nil）。
// 支持通配符 [*]：要求所有数组元素都满足剩余路径，空数组视为不满足。
func ensurePathExists(data map[string]any, path string) bool {
	parts := ParsePath(path)
	return pathSatisfies(data, parts, false)
}

// checkNotNull 检查路径上的值是否为非 nil。
// 支持通配符 [*]：要求所有数组元素的叶节点值非 nil。
func checkNotNull(data map[string]any, path string) bool {
	parts := ParsePath(path)
	return pathSatisfies(data, parts, true)
}

// pathSatisfies 递归检查路径。
//   - notNull=true 时叶节点必须非 nil（EnsureNotNull 语义）。
//   - notNull=false 时只要求路径存在（EnsurePresence 语义）。
//
// wildcard 语义：所有数组元素都必须满足剩余路径，任一不满足则返回 false。
func pathSatisfies(data any, parts []PathPart, notNull bool) bool {
	if len(parts) == 0 {
		if notNull {
			return data != nil
		}
		// EnsurePresence：只要路径走到这里就算存在（包括 nil 值）
		return true
	}

	part := parts[0]
	remaining := parts[1:]

	if part.Wild {
		arr, ok := data.([]any)
		if !ok || len(arr) == 0 {
			return false
		}
		for _, item := range arr {
			if !pathSatisfies(item, remaining, notNull) {
				return false
			}
		}
		return true
	}

	switch v := data.(type) {
	case map[string]any:
		child, ok := v[part.Key]
		if !ok {
			return false
		}
		return pathSatisfies(child, remaining, notNull)
	case []any:
		if part.Index < 0 || part.Index >= len(v) {
			return false
		}
		return pathSatisfies(v[part.Index], remaining, notNull)
	default:
		return false
	}
}
