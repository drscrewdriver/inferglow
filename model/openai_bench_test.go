package model

import (
	"context"
	"testing"
)

// G1-07.3: OpenAI Request 构建 Benchmark
// 覆盖：有无 tools、不同历史消息数。

// benchProvider 共享一个 Provider 实例避免重复构造。
var benchProvider = &OpenAICompatibleProvider{
	BaseURL: "https://api.openai.com/v1",
	Model:   "gpt-4",
}

// BenchmarkGenerateRequestDataBaseline 基线场景：纯文本、无历史、无工具。
func BenchmarkGenerateRequestDataBaseline(b *testing.B) {
	req := &ModelRequest{
		System:   "You are a helpful assistant.",
		Instruct: "Hello world",
		Input:    "What is 2+2?",
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = benchProvider.GenerateRequestData(ctx, req)
	}
}

// BenchmarkGenerateRequestDataWithTools 带 tools 场景。
func BenchmarkGenerateRequestDataWithTools(b *testing.B) {
	req := &ModelRequest{
		System:   "You are a helpful assistant.",
		Instruct: "Search for Go benchmarks",
		Tools: []ToolDefinition{
			{Name: "search", Description: "Search the web", Parameters: map[string]any{"type": "object"}},
			{Name: "calc", Description: "Calculator", Parameters: map[string]any{"type": "object"}},
			{Name: "fetch", Description: "Fetch URL", Parameters: map[string]any{"type": "object"}},
		},
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = benchProvider.GenerateRequestData(ctx, req)
	}
}

// BenchmarkGenerateRequestDataWithHistory 不同历史消息数。
func BenchmarkGenerateRequestDataWithHistory(b *testing.B) {
	cases := []struct {
		name string
		size int
	}{
		{"history_0", 0},
		{"history_10", 10},
		{"history_50", 50},
		{"history_200", 200},
	}
	ctx := context.Background()
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			history := make([]ChatMessage, c.size)
			for i := 0; i < c.size; i++ {
				role := "user"
				if i%2 == 1 {
					role = "assistant"
				}
				history[i] = ChatMessage{
					Role:    role,
					Content: "message-" + itoaBench(i),
				}
			}
			req := &ModelRequest{
				System:      "You are a helpful assistant.",
				Instruct:    "Continue the conversation",
				ChatHistory: history,
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = benchProvider.GenerateRequestData(ctx, req)
			}
		})
	}
}

// BenchmarkGenerateRequestDataWithOptions 含 Options（thinking 参数透传场景）。
func BenchmarkGenerateRequestDataWithOptions(b *testing.B) {
	req := &ModelRequest{
		System:   "You are MiMo.",
		Instruct: "Explain quantum computing",
		Options: map[string]any{
			"thinking":          map[string]string{"type": "enabled"},
			"top_p":             0.95,
			"reasoning_effort":  "high",
		},
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = benchProvider.GenerateRequestData(ctx, req)
	}
}

// BenchmarkGenerateRequestDataAllCombined 综合场景：system + developer + history + tools + options
func BenchmarkGenerateRequestDataAllCombined(b *testing.B) {
	history := make([]ChatMessage, 20)
	for i := 0; i < 20; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		history[i] = ChatMessage{Role: role, Content: "history-message-" + itoaBench(i)}
	}
	req := &ModelRequest{
		System:    "You are a helpful assistant.",
		Developer: "Always respond in JSON.",
		Instruct:  "Process the request",
		Input:     "complex payload",
		ChatHistory: history,
		Tools: []ToolDefinition{
			{Name: "search", Description: "Search the web", Parameters: map[string]any{"type": "object"}},
			{Name: "calc", Description: "Calculator", Parameters: map[string]any{"type": "object"}},
		},
		Options: map[string]any{
			"thinking": map[string]string{"type": "enabled"},
			"top_p":    0.95,
		},
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = benchProvider.GenerateRequestData(ctx, req)
	}
}

// itoaBench 是为 benchmark 文件准备的简易 int→string，避免与 pool_test.go 中的 itoa 冲突。
func itoaBench(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
