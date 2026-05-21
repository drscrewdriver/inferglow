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

package session

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// Phase 1: BUG-9 / S-HIGH-1 — Session 线程安全
// =============================================================================

// TestSession_ConcurrentAddMessage 验证 10 个 goroutine 各执行 100 次 AddMessage
// 后总消息数为 1000，且 -race 不报警。
func TestSession_ConcurrentAddMessage(t *testing.T) {
	s := NewSession("concurrent-add", 1<<20) // 大 maxLength 避免触发 resize
	const goroutines = 10
	const perGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				s.AddMessage("user", "msg", "")
			}
		}(g)
	}
	wg.Wait()

	got := len(s.GetFullContext())
	want := goroutines * perGoroutine
	if got != want {
		t.Errorf("FullContext len = %d, want %d (race likely caused lost appends)", got, want)
	}
}

// TestSession_ConcurrentReadWrite 一个 goroutine 持续写，另一个持续读，
// 验证读期间不会触发 race / panic。
func TestSession_ConcurrentReadWrite(t *testing.T) {
	s := NewSession("concurrent-rw", 1<<20)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	// Writer
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				s.AddMessage("user", "concurrent write", "")
			}
		}
	}()

	// Reader
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = s.GetContextWindow()
				_ = s.GetFullContext()
				_ = s.PreparePrompt()
			}
		}
	}()

	// 让两个 goroutine 跑一会儿
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// =============================================================================
// Phase 2: S-HIGH-2 — AddChatHistory 触发 AutoResize
// =============================================================================

// TestSession_AddChatHistory_TriggersAutoResize 设置 maxLength=100，
// AddChatHistory 一条 200 字符消息，验证 ContextWindow 被裁剪。
func TestSession_AddChatHistory_TriggersAutoResize(t *testing.T) {
	s := NewSession("history-resize", 100)
	s.AutoResize = true
	s.ResizeHandler = SimpleCutResizeHandler

	longContent := strings.Repeat("a", 200)
	s.AddChatHistory([]ChatMessage{
		{Role: "user", Content: longContent, Timestamp: time.Now()},
	})

	windowBytes := 0
	for _, m := range s.ContextWindow {
		windowBytes += len(ContentToString(m.Content))
	}
	// SimpleCutResizeHandler 在 totalBytes > 0 时会从前往后裁剪到为 0，
	// 然后保留最后一条。即使最后一条 > maxLength 也会保留。
	// 修复前：AddChatHistory 不触发 resize，ContextWindow 保留 1 条 200 字符消息
	// 修复后：ContextWindow 仍保留 1 条（SimpleCut 至少保留最后一条），
	//        但触发 resize 路径后，window 应等于 SimpleCut 处理结果。
	// 用更明确的策略：custom handler 把窗口替换为空 slice，验证 resize 已被触发。
	customCalled := false
	s2 := NewSession("history-resize-custom", 100)
	s2.AutoResize = true
	s2.ResizeHandler = func(full, window []ChatMessage) ([]ChatMessage, error) {
		customCalled = true
		// 返回前两条消息（如果存在）
		if len(window) >= 2 {
			return window[:2], nil
		}
		return window, nil
	}

	// 准备 5 条消息，总长 > 100
	msgs := []ChatMessage{
		{Role: "user", Content: strings.Repeat("x", 30), Timestamp: time.Now()},
		{Role: "assistant", Content: strings.Repeat("y", 30), Timestamp: time.Now()},
		{Role: "user", Content: strings.Repeat("z", 30), Timestamp: time.Now()},
		{Role: "assistant", Content: strings.Repeat("w", 30), Timestamp: time.Now()},
		{Role: "user", Content: strings.Repeat("v", 30), Timestamp: time.Now()},
	}
	s2.AddChatHistory(msgs)

	if !customCalled {
		t.Error("AddChatHistory 应在 AutoResize=true 时触发 ResizeHandler，但未被调用")
	}
	if len(s2.ContextWindow) != 2 {
		t.Errorf("ContextWindow len = %d, want 2 (custom handler 应裁剪到 2 条)", len(s2.ContextWindow))
	}
}

// =============================================================================
// Phase 2: S-HIGH-3 — Resize 错误被吞掉
// =============================================================================

// TestSession_ResizeError_Fallback 注册一个返回 error 的 ResizeHandler，
// 验证 messages 保持原样（回退）且 error 被记录到日志。
func TestSession_ResizeError_Fallback(t *testing.T) {
	// 捕获 log 输出：替换默认 logger 的输出
	var logBuf bytes.Buffer
	oldOutput := log.Default().Writer()
	log.Default().SetOutput(&logBuf)
	defer log.Default().SetOutput(oldOutput)

	s := NewSession("resize-error", 5)
	s.AutoResize = true
	s.ResizeHandler = func(full, window []ChatMessage) ([]ChatMessage, error) {
		// 返回 error，且返回的 slice 不应被应用
		return nil, errSentinel
	}

	s.AddMessage("user", "this message is longer than 5 bytes", "")

	// 验证 messages 保持原样（未被替换为 nil）
	if len(s.ContextWindow) != 1 {
		t.Errorf("ContextWindow len = %d, want 1 (resize error 应回退到原消息)", len(s.ContextWindow))
	}

	// 验证 error 被记录到日志
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "resize") && !strings.Contains(logOutput, "Resize") {
		t.Errorf("resize error 应被记录到日志，但 log 输出为空或不含 resize: %q", logOutput)
	}
}

// errSentinel 用于 ResizeHandler 返回 error 的测试
var errSentinel = &testError{"resize failed for testing"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// =============================================================================
// Phase 2: BUG-19 / S-MEDIUM-1 — LoadJSON 恢复 handler
// =============================================================================

// TestPersistence_LoadRestoresHandlers SaveJSON 一个有 summary_first handler
// 的 session，LoadJSON 后验证 handler 已恢复。
func TestPersistence_LoadRestoresHandlers(t *testing.T) {
	tmpDir := t.TempDir()
	path := tmpDir + "/session_handlers.json"

	s := NewSession("handler-restore", 100)
	s.AutoResize = true
	s.RegisterResizeHandler("simple_cut", SimpleCutResizeHandler)
	s.RegisterResizeHandler("summary_first", SummaryFirstResizeHandler)
	if err := s.SetDefaultResizeHandler("summary_first"); err != nil {
		t.Fatalf("SetDefaultResizeHandler failed: %v", err)
	}
	s.AddMessage("user", "initial", "")

	if err := s.SaveJSON(path); err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}

	s2 := NewSession("loaded", 0)
	if err := s2.LoadJSON(path); err != nil {
		t.Fatalf("LoadJSON failed: %v", err)
	}

	// 验证 resizeHandlers 已恢复
	names := s2.ListResizeHandlers()
	if len(names) < 2 {
		t.Errorf("LoadJSON 后 resizeHandlers 数量 = %d, want >= 2 (simple_cut + summary_first)", len(names))
	}

	// 验证 defaultResizeName 已恢复
	// 通过 AddMessage 触发 resize 流程，验证 default 策略生效
	s2.AutoResize = true
	s2.RegisterAnalysisHandler(func(full, window []ChatMessage, memo map[string]any) (string, error) {
		return "nonexistent_strategy", nil // 故意返回未注册的，触发 default 回退
	})

	// 添加多条消息触发 resize。SummaryFirstResizeHandler 在 len(window) >= 4
	// 时会生成 [summary: 消息：[first, summary(middle), last 2]。
	s2.AddMessage("user", strings.Repeat("a", 200), "")
	s2.AddMessage("user", strings.Repeat("b", 200), "")
	s2.AddMessage("user", strings.Repeat("c", 200), "")

	// 如果 default handler 恢复了，ContextWindow 应被 summary_first 处理
	// 即至少包含一条 "[summary:" 开头的 system 消息
	foundSummary := false
	for _, m := range s2.ContextWindow {
		if c, ok := m.Content.(string); ok && strings.HasPrefix(c, "[summary:") {
			foundSummary = true
			break
		}
	}
	if !foundSummary {
		t.Errorf("LoadJSON 后默认 ResizeHandler 未生效（期望 summary_first 处理结果，含 [summary: 消息）；ContextWindow=%v", s2.ContextWindow)
	}
}
