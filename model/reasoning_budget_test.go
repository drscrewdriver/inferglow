package model

import (
	"context"
	"testing"
)

// G1-05 + G1-06 测试：推理预算控制 + 推理 token 单独计费

// TestBroadcastResponseReasoningBudgetTruncation 验证 MaxReasoningTokens 预算
// 生效时，fullReasoning 会被截断且 ModelResponse.ReasoningTruncated=true。
func TestBroadcastResponseReasoningBudgetTruncation(t *testing.T) {
	// 构造一个 stream：先发若干 reasoning chunk，再发 done。
	// 每 chunk 的 reasoning 长度均小于预算，但累计超过。
	stream := make(chan *StreamChunk, 4)
	stream <- &StreamChunk{Reasoning: "abcdefgh"} // 8 bytes
	stream <- &StreamChunk{Reasoning: "ijklmnop"} // 8 bytes，累计 16 bytes
	stream <- &StreamChunk{Reasoning: "qrstuvwxyz"} // 12 bytes，累计 28 bytes
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	// 预算 = 5 tokens * 4 bytes = 20 bytes
	p := &OpenAICompatibleProvider{MaxReasoningTokens: 5}
	events, err := p.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var done *ModelResponse
	for ev := range events {
		if ev.EventType == EventDone {
			if mr, ok := ev.Payload.(*ModelResponse); ok {
				done = mr
			}
		}
	}
	if done == nil {
		t.Fatal("expected EventDone")
	}
	if !done.ReasoningTruncated {
		t.Errorf("ReasoningTruncated = false, want true (budget 20 bytes, got %d bytes of reasoning)", len(done.Reasoning))
	}
	// 截断后 reasoning 长度不应超过预算 20 bytes。
	if len(done.Reasoning) > 20 {
		t.Errorf("Reasoning len = %d, want <= 20", len(done.Reasoning))
	}
	// 第一段 8 bytes 完整保留。
	if got := done.Reasoning[:8]; got != "abcdefgh" {
		t.Errorf("first 8 bytes = %q, want %q", got, "abcdefgh")
	}
}

// TestBroadcastResponseReasoningBudgetWithinBudget 验证预算未超时
// ReasoningTruncated=false。
func TestBroadcastResponseReasoningBudgetWithinBudget(t *testing.T) {
	stream := make(chan *StreamChunk, 3)
	stream <- &StreamChunk{Reasoning: "abcdefgh"} // 8 bytes
	stream <- &StreamChunk{Reasoning: "ijklmnop"} // 8 bytes，累计 16 bytes
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	// 预算 = 10 tokens * 4 bytes = 40 bytes，大于 16 bytes
	p := &OpenAICompatibleProvider{MaxReasoningTokens: 10}
	events, err := p.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var done *ModelResponse
	for ev := range events {
		if ev.EventType == EventDone {
			if mr, ok := ev.Payload.(*ModelResponse); ok {
				done = mr
			}
		}
	}
	if done == nil {
		t.Fatal("expected EventDone")
	}
	if done.ReasoningTruncated {
		t.Errorf("ReasoningTruncated = true, want false (within budget)")
	}
	if done.Reasoning != "abcdefghijklmnop" {
		t.Errorf("Reasoning = %q, want %q", done.Reasoning, "abcdefghijklmnop")
	}
}

// TestBroadcastResponseReasoningBudgetZero 验证 MaxReasoningTokens=0 时
// 不做限制（向后兼容）。
func TestBroadcastResponseReasoningBudgetZero(t *testing.T) {
	stream := make(chan *StreamChunk, 3)
	stream <- &StreamChunk{Reasoning: "abcdefgh"}
	stream <- &StreamChunk{Reasoning: "ijklmnop"}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	p := &OpenAICompatibleProvider{MaxReasoningTokens: 0} // 无限制
	events, err := p.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var done *ModelResponse
	for ev := range events {
		if ev.EventType == EventDone {
			if mr, ok := ev.Payload.(*ModelResponse); ok {
				done = mr
			}
		}
	}
	if done == nil {
		t.Fatal("expected EventDone")
	}
	if done.ReasoningTruncated {
		t.Errorf("ReasoningTruncated = true, want false (no budget)")
	}
	if done.Reasoning != "abcdefghijklmnop" {
		t.Errorf("Reasoning = %q, want %q", done.Reasoning, "abcdefghijklmnop")
	}
}

// TestBroadcastResponseReasoningTruncateUTF8 验证截断按 rune 边界对齐，
// 不会切断多字节 UTF-8 字符（如中文）。
func TestBroadcastResponseReasoningTruncateUTF8(t *testing.T) {
	// "你好世界" 共 4 个汉字，UTF-8 编码为 12 bytes（每字 3 bytes）。
	stream := make(chan *StreamChunk, 2)
	stream <- &StreamChunk{Reasoning: "你好世界你好世界"} // 24 bytes
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	// 预算 = 2 tokens * 4 bytes = 8 bytes。
	// 截断到 8 bytes 边界，但因 UTF-8 rune 对齐，实际保留 6 bytes（2 个汉字）。
	p := &OpenAICompatibleProvider{MaxReasoningTokens: 2}
	events, err := p.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var done *ModelResponse
	for ev := range events {
		if ev.EventType == EventDone {
			if mr, ok := ev.Payload.(*ModelResponse); ok {
				done = mr
			}
		}
	}
	if done == nil {
		t.Fatal("expected EventDone")
	}
	if !done.ReasoningTruncated {
		t.Errorf("ReasoningTruncated = false, want true")
	}
	// 应保留 2 个完整汉字 "你好"（6 bytes），不超过 8 bytes 预算。
	if done.Reasoning != "你好" {
		t.Errorf("Reasoning = %q, want %q (rune-aligned truncation)", done.Reasoning, "你好")
	}
}

// TestBroadcastResponseReasoningTokensFromUsage 验证 G1-06：当 Provider
// 在 usage.completion_tokens_details.reasoning_tokens 中报告推理 token 计数时，
// ModelResponse.ReasoningTokens 被正确填充。
func TestBroadcastResponseReasoningTokensFromUsage(t *testing.T) {
	stream := make(chan *StreamChunk, 2)
	usage := &UsageInfo{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		CompletionTokensDetails: map[string]int{
			"reasoning_tokens": 30,
		},
	}
	stream <- &StreamChunk{Delta: "answer", Usage: usage}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	p := &OpenAICompatibleProvider{}
	events, err := p.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var done *ModelResponse
	var sawReasoningMeta bool
	var metaCount int
	for ev := range events {
		if ev.EventType == EventDone {
			if mr, ok := ev.Payload.(*ModelResponse); ok {
				done = mr
			}
		}
		// G1-06: 额外的 ReasoningTokenMeta 事件
		if ev.EventType == MetaEvent {
			if rtm, ok := ev.Payload.(*ReasoningTokenMeta); ok {
				sawReasoningMeta = true
				metaCount = rtm.Count
			}
		}
	}
	if done == nil {
		t.Fatal("expected EventDone")
	}
	if done.ReasoningTokens != 30 {
		t.Errorf("ReasoningTokens = %d, want 30", done.ReasoningTokens)
	}
	if !sawReasoningMeta {
		t.Error("expected ReasoningTokenMeta MetaEvent to be emitted")
	}
	if metaCount != 30 {
		t.Errorf("ReasoningTokenMeta.Count = %d, want 30", metaCount)
	}
}

// TestBroadcastResponseReasoningTokensAbsent 验证 Provider 未返回
// reasoning_tokens 时，ModelResponse.ReasoningTokens=0 且不发 ReasoningTokenMeta。
func TestBroadcastResponseReasoningTokensAbsent(t *testing.T) {
	stream := make(chan *StreamChunk, 2)
	usage := &UsageInfo{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		// 无 CompletionTokensDetails
	}
	stream <- &StreamChunk{Delta: "answer", Usage: usage}
	stream <- &StreamChunk{IsDone: true}
	close(stream)

	p := &OpenAICompatibleProvider{}
	events, err := p.BroadcastResponse(context.Background(), stream)
	if err != nil {
		t.Fatalf("BroadcastResponse failed: %v", err)
	}

	var done *ModelResponse
	sawReasoningMeta := false
	for ev := range events {
		if ev.EventType == EventDone {
			if mr, ok := ev.Payload.(*ModelResponse); ok {
				done = mr
			}
		}
		if ev.EventType == MetaEvent {
			if _, ok := ev.Payload.(*ReasoningTokenMeta); ok {
				sawReasoningMeta = true
			}
		}
	}
	if done == nil {
		t.Fatal("expected EventDone")
	}
	if done.ReasoningTokens != 0 {
		t.Errorf("ReasoningTokens = %d, want 0", done.ReasoningTokens)
	}
	if sawReasoningMeta {
		t.Error("did not expect ReasoningTokenMeta MetaEvent")
	}
}

// TestUsageInfoReasoningTokens 验证 UsageInfo.ReasoningTokens() 辅助方法。
func TestUsageInfoReasoningTokens(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		u := &UsageInfo{
			CompletionTokensDetails: map[string]int{"reasoning_tokens": 42},
		}
		if got := u.ReasoningTokens(); got != 42 {
			t.Errorf("ReasoningTokens() = %d, want 42", got)
		}
	})
	t.Run("absent", func(t *testing.T) {
		u := &UsageInfo{}
		if got := u.ReasoningTokens(); got != 0 {
			t.Errorf("ReasoningTokens() = %d, want 0", got)
		}
	})
	t.Run("nil_receiver", func(t *testing.T) {
		var u *UsageInfo
		if got := u.ReasoningTokens(); got != 0 {
			t.Errorf("ReasoningTokens() = %d, want 0 (nil receiver)", got)
		}
	})
	t.Run("nil_map", func(t *testing.T) {
		u := &UsageInfo{CompletionTokensDetails: nil}
		if got := u.ReasoningTokens(); got != 0 {
			t.Errorf("ReasoningTokens() = %d, want 0 (nil map)", got)
		}
	})
}

// TestProcessOpenAILineParsesReasoningTokens 验证 SSE 中的
// usage.completion_tokens_details.reasoning_tokens 能被反序列化到 UsageInfo。
func TestProcessOpenAILineParsesReasoningTokens(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	var usage *UsageInfo
	toolStates := map[int]*openAIToolState{}
	var got *StreamChunk
	emit := func(c *StreamChunk) { got = c }

	// 一条同时携带 usage 与 reasoning_tokens 详情的 SSE 行。
	line := `data: {"choices":[{"delta":{"content":"hi"},"finish_reason":null}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"completion_tokens_details":{"reasoning_tokens":3}}}`
	if u := p.processOpenAILine(line, usage, toolStates, emit); u != nil {
		usage = u
	}
	if usage == nil {
		t.Fatal("expected usage to be parsed")
	}
	if usage.CompletionTokens != 5 {
		t.Errorf("CompletionTokens = %d, want 5", usage.CompletionTokens)
	}
	if got == nil {
		t.Fatal("expected chunk")
	}
	if got.Usage == nil {
		t.Fatal("expected chunk.Usage")
	}
	if got.Usage.ReasoningTokens() != 3 {
		t.Errorf("ReasoningTokens() = %d, want 3", got.Usage.ReasoningTokens())
	}
}
