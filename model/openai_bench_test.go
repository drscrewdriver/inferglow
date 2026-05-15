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

// G1-07.3b: 流式解析（processOpenAILine）与 usage 解析 Benchmark。
// 构造模拟 SSE 行反复解析，覆盖 content delta / reasoning / tool_call / usage 场景。

// benchSSELines 预构造的模拟 SSE 行，覆盖主要解析路径。
var benchSSELines = []string{
	// content delta
	`data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"Hello, how can I help you today?"},"finish_reason":""}]}` + "\n",
	// reasoning_content delta (MiMo/Spark 风格)
	`data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"reasoning_content":"Let me think about this problem step by step."},"finish_reason":""}]}` + "\n",
	// tool_call 首块（id + name + 部分 arguments）
	`data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"calculate","arguments":"{\"expr\":"}}]},"finish_reason":""}]}` + "\n",
	// tool_call 续块（arguments 增量）
	`data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"1+1\"}"}}]},"finish_reason":""}]}` + "\n",
	// tool_calls finish
	`data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n",
	// usage-only chunk（empty choices，含 reasoning_tokens）
	`data: {"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":128,"completion_tokens":64,"total_tokens":192,"completion_tokens_details":{"reasoning_tokens":32}}}` + "\n",
	// stop finish
	`data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n",
	// [DONE]
	"data: [DONE]\n",
	// 非 data 行（应被跳过）
	": keepalive\n",
}

// noopEmit 是一个空操作的 emit 函数，避免分配 channel。
func noopEmit(*StreamChunk) {}

// BenchmarkProcessOpenAILineSingle 测试单行反复解析的性能。
func BenchmarkProcessOpenAILineSingle(b *testing.B) {
	line := benchSSELines[0]
	toolStates := make(map[int]*openAIToolState)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = benchProvider.processOpenAILine(line, nil, toolStates, noopEmit)
	}
}

// BenchmarkProcessOpenAILineMixed 测试混合 SSE 行序列解析（含 content/reasoning/tool/usage/done）。
func BenchmarkProcessOpenAILineMixed(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toolStates := make(map[int]*openAIToolState)
		var usage *UsageInfo
		for _, line := range benchSSELines {
			if u := benchProvider.processOpenAILine(line, usage, toolStates, noopEmit); u != nil {
				usage = u
			}
		}
	}
}

// BenchmarkProcessOpenAILineToolCalls 测试 tool_call 累积场景（首块+续块+finish）。
func BenchmarkProcessOpenAILineToolCalls(b *testing.B) {
	lines := benchSSELines[2:5] // tool_call 首块、续块、finish
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toolStates := make(map[int]*openAIToolState)
		for _, line := range lines {
			benchProvider.processOpenAILine(line, nil, toolStates, noopEmit)
		}
	}
}

// BenchmarkProcessOpenAILineUsage 测试含 usage 的 chunk 解析（JSON 反序列化 UsageInfo）。
func BenchmarkProcessOpenAILineUsage(b *testing.B) {
	line := benchSSELines[5]
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = benchProvider.processOpenAILine(line, nil, make(map[int]*openAIToolState), noopEmit)
	}
}

// BenchmarkUsageInfoReasoningTokens 测试 ReasoningTokens() 方法的调用开销。
func BenchmarkUsageInfoReasoningTokens(b *testing.B) {
	u := &UsageInfo{
		PromptTokens:           128,
		CompletionTokens:       64,
		TotalTokens:            192,
		CompletionTokensDetails: map[string]int{"reasoning_tokens": 32},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = u.ReasoningTokens()
	}
}

// BenchmarkUsageInfoReasoningTokensNil 测试 nil details 场景的开销。
func BenchmarkUsageInfoReasoningTokensNil(b *testing.B) {
	u := &UsageInfo{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = u.ReasoningTokens()
	}
}

// BenchmarkNormalizeThinkingTags 测试 <think> 标签归一化的开销。
func BenchmarkNormalizeThinkingTags(b *testing.B) {
	content := "Hello <think>this is reasoning content that should be extracted</think> world"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = normalizeThinkingTags(content)
	}
}
