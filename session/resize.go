package session

import (
	"strings"
)

// DefaultAnalysisHandler checks if the contextWindow total byte size exceeds maxLength.
func DefaultAnalysisHandler(fullContext []ChatMessage, contextWindow []ChatMessage, maxLength int) bool {
	totalBytes := 0
	for _, m := range contextWindow {
		totalBytes += len(ContentToString(m.Content))
	}
	return totalBytes > maxLength
}

// SimpleCutResizeHandler trims messages from the front of the window until
// the total content byte size fits within the session's MaxLength.  The most
// recent messages are kept.
func SimpleCutResizeHandler(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error) {
	if len(contextWindow) == 0 {
		return contextWindow, nil
	}

	// Calculate total bytes in contextWindow
	totalBytes := 0
	for _, m := range contextWindow {
		totalBytes += len(ContentToString(m.Content))
	}

	if totalBytes <= 0 {
		return contextWindow, nil
	}

	// Trim from the front until we fit
	resized := make([]ChatMessage, len(contextWindow))
	copy(resized, contextWindow)

	for totalBytes > 0 && len(resized) > 0 {
		totalBytes -= len(ContentToString(resized[0].Content))
		resized = resized[1:]
	}

	if len(resized) == 0 {
		// Keep at least the most recent message even if it exceeds the limit
		resized = []ChatMessage{contextWindow[len(contextWindow)-1]}
	}

	return resized, nil
}

// SummaryFirstResizeHandler 保留窗口首条消息 + 末尾 2 条消息，中间被丢弃的消息
// 替换为单条 system 摘要消息（"[summary: <first 100 chars of joined dropped content>]"）。
//
// 示例: window=[m0, m1, m2, m3, m4] -> result=[m0, summary(m1+m2), m3, m4]
//
// 边界情况:
//   - len(window) <= 2: 原样返回 copy
//   - len(window) == 3: 首条 + 末尾 2 条覆盖全部，无中间消息可摘要，原样返回 copy
func SummaryFirstResizeHandler(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error) {
	if len(contextWindow) <= 2 {
		result := make([]ChatMessage, len(contextWindow))
		copy(result, contextWindow)
		return result, nil
	}

	first := contextWindow[0]
	lastTwo := contextWindow[len(contextWindow)-2:]
	middle := contextWindow[1 : len(contextWindow)-2]

	if len(middle) == 0 {
		// 无中间消息可摘要，原样返回 copy
		result := make([]ChatMessage, len(contextWindow))
		copy(result, contextWindow)
		return result, nil
	}

	var dropped strings.Builder
	for _, m := range middle {
		dropped.WriteString(ContentToString(m.Content))
	}
	droppedStr := dropped.String()
	if len(droppedStr) > 100 {
		droppedStr = droppedStr[:100]
	}

	summary := ChatMessage{
		Role:    "system",
		Content: "[summary: " + droppedStr + "]",
	}

	result := make([]ChatMessage, 0, 1+1+len(lastTwo))
	result = append(result, first)
	result = append(result, summary)
	result = append(result, lastTwo...)
	return result, nil
}

// TokenAwareResizeHandlerWithMax 返回一个绑定了 maxLength 的 TokenAware resize handler。
//
// 估算规则: 每条消息 token 数 ≈ len(content) / 4，从前往后丢弃消息直到总 token 数
// ≤ maxLength/4，至少保留末尾 1 条消息。
func TokenAwareResizeHandlerWithMax(maxLength int) ResizeHandler {
	return func(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error) {
		maxTokens := maxLength / 4
		if maxTokens <= 0 {
			maxTokens = 500 // 默认值
		}

		tokens := make([]int, len(contextWindow))
		totalTokens := 0
		for i, m := range contextWindow {
			tokens[i] = len(ContentToString(m.Content)) / 4
			totalTokens += tokens[i]
		}

		if totalTokens <= maxTokens {
			result := make([]ChatMessage, len(contextWindow))
			copy(result, contextWindow)
			return result, nil
		}

		// 从前往后丢弃，直到 token 数 <= maxTokens 或只剩 1 条
		dropUntil := 0
		for dropUntil < len(contextWindow)-1 && totalTokens > maxTokens {
			totalTokens -= tokens[dropUntil]
			dropUntil++
		}

		result := make([]ChatMessage, len(contextWindow)-dropUntil)
		copy(result, contextWindow[dropUntil:])
		return result, nil
	}
}

// TokenAwareResizeHandler 默认版本，使用 8000 字符作为 maxLength（约 2000 tokens）。
var TokenAwareResizeHandler = TokenAwareResizeHandlerWithMax(8000)
