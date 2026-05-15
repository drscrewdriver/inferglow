package session

import (
	"fmt"
	"testing"
	"time"
)

// G1-07.4: ThreeZone Session Benchmark
// 覆盖：不同消息长度、不同 resize 策略。

// makeMsg 构造一条指定内容长度的 ChatMessage。
func makeMsg(role, content string) ChatMessage {
	return ChatMessage{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
	}
}

// BenchmarkThreeZoneAppend 不触发 resize 的纯 append 性能。
func BenchmarkThreeZoneAppend(b *testing.B) {
	cases := []struct {
		name      string
		msgBytes  int
		maxBytes  int
	}{
		{"small_unbounded", 100, 1 << 30},  // 100B/条，max 设很大不触发 resize
		{"medium_unbounded", 1024, 1 << 30},
		{"large_unbounded", 8192, 1 << 30},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			content := string(make([]byte, c.msgBytes))
			// 用相同字符填充以避免每条消息重新分配内容
			for i := range content {
				content = content[:i] + "x" + content[i+1:]
			}
			s := NewThreeZoneSession("bench", c.maxBytes)
			_ = s.SetImmutablePrefix("system prompt", nil)
			msg := makeMsg("user", content)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.AddToHistory(msg)
			}
		})
	}
}

// BenchmarkThreeZoneBuildPrompt 测试 BuildPrompt（拼接三区）开销。
func BenchmarkThreeZoneBuildPrompt(b *testing.B) {
	cases := []struct {
		name    string
		history int
		scratch int
	}{
		{"history_0_scratch_0", 0, 0},
		{"history_10_scratch_2", 10, 2},
		{"history_100_scratch_5", 100, 5},
		{"history_500_scratch_10", 500, 10},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			s := NewThreeZoneSession("bench", 1<<30)
			_ = s.SetImmutablePrefix("system prompt", nil)
			for i := 0; i < c.history; i++ {
				s.AddToHistory(makeMsg("user", fmt.Sprintf("msg-%d", i)))
			}
			scratch := make([]ChatMessage, c.scratch)
			for i := 0; i < c.scratch; i++ {
				scratch[i] = makeMsg("assistant", fmt.Sprintf("scratch-%d", i))
			}
			s.SetVolatileScratch(scratch)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = s.BuildPrompt()
			}
		})
	}
}

// BenchmarkThreeZoneResize 触发 resize 的 append 性能（不同策略）。
func BenchmarkThreeZoneResize(b *testing.B) {
	// snip 策略：从头部移除 N 条
	b.Run("snip", func(b *testing.B) {
		s := NewThreeZoneSession("bench", 1024) // 1KB 上限
		_ = s.SetImmutablePrefix("sys", nil)
		s.SetResizeStrategies(SnipFromHead(1), nil, nil)
		msg := makeMsg("user", "xxxxxxxxxxxx") // 12 bytes
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s.AddToHistory(msg)
		}
	})

	// prune 策略：剔除短消息
	b.Run("prune", func(b *testing.B) {
		s := NewThreeZoneSession("bench", 1024)
		_ = s.SetImmutablePrefix("sys", nil)
		s.SetResizeStrategies(nil, PruneLowValue(5), nil)
		msg := makeMsg("user", "xxxx") // 4 bytes < 5 → 会被 prune
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s.AddToHistory(msg)
		}
	})

	// snip → prune 链式
	b.Run("snip_then_prune", func(b *testing.B) {
		s := NewThreeZoneSession("bench", 1024)
		_ = s.SetImmutablePrefix("sys", nil)
		s.SetResizeStrategies(SnipFromHead(1), PruneLowValue(5), nil)
		msg := makeMsg("user", "xxxxxxxxxxxx")
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s.AddToHistory(msg)
		}
	})
}

// BenchmarkThreeZoneSetImmutablePrefix 测试 Zone 1 设置开销（含 hash 计算）。
func BenchmarkThreeZoneSetImmutablePrefix(b *testing.B) {
	cases := []struct {
		name    string
		prompt  string
		tools   []any
	}{
		{"short_no_tools", "You are helpful.", nil},
		{"long_no_tools", string(make([]byte, 4096)), nil}, // 4KB prompt
		{"short_with_tools", "You are helpful.", []any{
			map[string]any{"name": "tool1", "description": "desc1"},
			map[string]any{"name": "tool2", "description": "desc2"},
			map[string]any{"name": "tool3", "description": "desc3"},
		}},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				s := NewThreeZoneSession("bench", 1<<30)
				_ = s.SetImmutablePrefix(c.prompt, c.tools)
			}
		})
	}
}

// BenchmarkThreeZoneScratchOnly 测试 Zone 3 单独操作的吞吐。
func BenchmarkThreeZoneScratchOnly(b *testing.B) {
	s := NewThreeZoneSession("bench", 1<<30)
	_ = s.SetImmutablePrefix("sys", nil)
	scratch := make([]ChatMessage, 10)
	for i := 0; i < 10; i++ {
		scratch[i] = makeMsg("assistant", fmt.Sprintf("scratch-%d", i))
	}
	b.Run("set_scratch", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			s.SetVolatileScratch(scratch)
		}
	})
	b.Run("clear_scratch", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			s.ClearVolatileScratch()
		}
	})
}

// Sanity check 确保 benchmark 数据生成正确。
func TestBenchThreeZoneSanity(t *testing.T) {
	s := NewThreeZoneSession("bench", 1<<30)
	if err := s.SetImmutablePrefix("sys", nil); err != nil {
		t.Fatalf("SetImmutablePrefix: %v", err)
	}
	for i := 0; i < 5; i++ {
		s.AddToHistory(makeMsg("user", fmt.Sprintf("msg-%d", i)))
	}
	prompt := s.BuildPrompt()
	// 1 (system) + 5 (history) = 6
	if len(prompt) != 6 {
		t.Errorf("prompt length = %d, want 6", len(prompt))
	}
}
