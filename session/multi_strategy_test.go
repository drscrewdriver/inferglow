package session

import (
	"reflect"
	"strings"
	"testing"
)

// Task 5.6.1: TestRegisterResizeHandler
// 注册 2 个策略 + ListResizeHandlers 返回排序后的列表
func TestRegisterResizeHandler(t *testing.T) {
	s := NewSession("register-test", 1000)
	s.RegisterResizeHandler("simple_cut", SimpleCutResizeHandler)
	s.RegisterResizeHandler("summary_first", SummaryFirstResizeHandler)

	handlers := s.ListResizeHandlers()
	if len(handlers) != 2 {
		t.Fatalf("ListResizeHandlers returned %d handlers, want 2", len(handlers))
	}

	// 应按字母升序排序
	expected := []string{"simple_cut", "summary_first"}
	if !reflect.DeepEqual(handlers, expected) {
		t.Errorf("ListResizeHandlers = %v, want %v", handlers, expected)
	}
}

// Task 5.6.2: TestSetDefaultResizeHandler
// 设置默认策略 / 不存在的策略 → error
func TestSetDefaultResizeHandler(t *testing.T) {
	s := NewSession("default-test", 1000)
	s.RegisterResizeHandler("simple_cut", SimpleCutResizeHandler)
	s.RegisterResizeHandler("summary_first", SummaryFirstResizeHandler)

	// 设置存在的策略应成功
	if err := s.SetDefaultResizeHandler("summary_first"); err != nil {
		t.Errorf("SetDefaultResizeHandler(summary_first) returned error: %v", err)
	}

	// 设置不存在的策略应返回 error
	err := s.SetDefaultResizeHandler("nonexistent")
	if err == nil {
		t.Fatal("SetDefaultResizeHandler(nonexistent) should return error, got nil")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("error message = %q, should contain 'not registered'", err.Error())
	}
}

// Task 5.6.3: TestRegisterAnalysisHandler
// 注册 AnalysisHandler + AddMessage 触发对应策略
func TestRegisterAnalysisHandler(t *testing.T) {
	s := NewSession("analyzer-test", 100)
	s.AutoResize = true
	s.RegisterResizeHandler("simple_cut", SimpleCutResizeHandler)

	analyzerCalled := false
	s.RegisterAnalysisHandler(func(full []ChatMessage, window []ChatMessage, memo map[string]any) (string, error) {
		analyzerCalled = true
		return "simple_cut", nil
	})

	s.AddMessage("user", "hello world this is a test message that should trigger resize", "")

	if !analyzerCalled {
		t.Error("AnalysisHandler should have been called when AutoResize=true and analysisHandlers non-empty")
	}

	// SimpleCutResizeHandler 会把窗口裁剪到只剩最后一条消息
	if len(s.ContextWindow) != 1 {
		t.Errorf("ContextWindow len = %d, want 1 (SimpleCut keeps last message)", len(s.ContextWindow))
	}
}

// Task 5.6.4: TestAnalysisHandlerEmptyStringNotTrigger
// AnalysisHandler 返回 "" → 不触发 resize
func TestAnalysisHandlerEmptyStringNotTrigger(t *testing.T) {
	s := NewSession("no-trigger-test", 1000)
	s.AutoResize = true
	s.RegisterResizeHandler("simple_cut", SimpleCutResizeHandler)

	analyzerCalled := false
	s.RegisterAnalysisHandler(func(full []ChatMessage, window []ChatMessage, memo map[string]any) (string, error) {
		analyzerCalled = true
		return "", nil // 空字符串，不触发 resize
	})

	s.AddMessage("user", "hello", "")
	s.AddMessage("assistant", "world", "")

	if !analyzerCalled {
		t.Error("AnalysisHandler should have been called at least once")
	}

	// 没有触发 resize，ContextWindow 应保留所有消息
	if len(s.ContextWindow) != 2 {
		t.Errorf("ContextWindow len = %d, want 2 (no resize should happen on empty strategy name)", len(s.ContextWindow))
	}
}

// Task 5.6.5: TestOldPathCompat
// 仅设置 ResizeHandler（旧 API） → 行为与旧版本一致
func TestOldPathCompat(t *testing.T) {
	s := NewSession("old-path-test", 50)
	s.AutoResize = true

	triggered := false
	s.ResizeHandler = func(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error) {
		triggered = true
		return SimpleCutResizeHandler(fullContext, contextWindow)
	}

	// 短消息，不触发
	s.AddMessage("user", "hi", "")
	s.AddMessage("assistant", "hello", "")
	if triggered {
		t.Error("ResizeHandler should not be triggered for messages within limit")
	}

	// 长消息，超过 50 字节，触发
	s.AddMessage("user", "This is a very long message that should definitely exceed the 50 byte limit we set", "")
	if !triggered {
		t.Error("ResizeHandler should be triggered when message exceeds MaxLength")
	}

	// FullContext 保留全部 3 条
	if len(s.FullContext) != 3 {
		t.Errorf("FullContext len = %d, want 3", len(s.FullContext))
	}

	// ContextWindow 应被裁剪（SimpleCut 保留最后一条）
	if len(s.ContextWindow) != 1 {
		t.Errorf("ContextWindow len = %d, want 1 (SimpleCut keeps last message)", len(s.ContextWindow))
	}
}

// Task 5.6.6: TestSummaryFirstResizeHandler
// window = 5 条消息 → resize 后 [first, summary, last2] 共 4 条
// summary content 以 "[summary:" 开头
// window 长度 <= 2 时原样返回
func TestSummaryFirstResizeHandler(t *testing.T) {
	window := []ChatMessage{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "msg1 content"},
		{Role: "assistant", Content: "msg2 content"},
		{Role: "user", Content: "msg3 content"},
		{Role: "assistant", Content: "msg4 content"},
	}

	resized, err := SummaryFirstResizeHandler(nil, window)
	if err != nil {
		t.Fatalf("SummaryFirstResizeHandler failed: %v", err)
	}

	// 期望: [first, summary, last 2] = 4 条
	if len(resized) != 4 {
		t.Fatalf("len(resized) = %d, want 4", len(resized))
	}

	// 第一条应是原首条消息
	if !reflect.DeepEqual(resized[0], window[0]) {
		t.Errorf("resized[0] = %v, want %v (first message)", resized[0], window[0])
	}

	// 第二条应是 summary system 消息
	if resized[1].Role != "system" {
		t.Errorf("resized[1].Role = %q, want %q", resized[1].Role, "system")
	}
	if c, ok := resized[1].Content.(string); ok && !strings.HasPrefix(c, "[summary:") {
		t.Errorf("resized[1].Content = %q, should start with '[summary:'", resized[1].Content)
	}

	// 末尾两条应是原 window 的 lastTwo
	if !reflect.DeepEqual(resized[2], window[3]) {
		t.Errorf("resized[2] = %v, want %v (window[3])", resized[2], window[3])
	}
	if !reflect.DeepEqual(resized[3], window[4]) {
		t.Errorf("resized[3] = %v, want %v (window[4])", resized[3], window[4])
	}

	// 边界: window 长度 <= 2 时原样返回
	shortWindow := []ChatMessage{
		{Role: "user", Content: "only one"},
		{Role: "assistant", Content: "two"},
	}
	resizedShort, err := SummaryFirstResizeHandler(nil, shortWindow)
	if err != nil {
		t.Fatalf("SummaryFirstResizeHandler with short window failed: %v", err)
	}
	if !reflect.DeepEqual(resizedShort, shortWindow) {
		t.Errorf("short window (len=2) should be returned as-is, got %v", resizedShort)
	}

	// 边界: 单条消息原样返回
	singleWindow := []ChatMessage{{Role: "user", Content: "alone"}}
	resizedSingle, err := SummaryFirstResizeHandler(nil, singleWindow)
	if err != nil {
		t.Fatalf("SummaryFirstResizeHandler with single window failed: %v", err)
	}
	if !reflect.DeepEqual(resizedSingle, singleWindow) {
		t.Errorf("single window should be returned as-is, got %v", resizedSingle)
	}
}

// Task 5.6.7: TestTokenAwareResizeHandlerWithMax
// maxLength = 100（约 25 tokens）
// window = 5 条消息，每条约 50 字符（约 12 tokens，共 60 tokens）
// resize 后 → 总 tokens <= 25，至少保留 1 条
func TestTokenAwareResizeHandlerWithMax(t *testing.T) {
	// maxLength = 100 -> maxTokens = 25
	handler := TokenAwareResizeHandlerWithMax(100)

	// 5 条消息，每条 50 字符 -> 每条约 12 tokens (50/4=12)，总 60 tokens
	window := make([]ChatMessage, 5)
	for i := range window {
		window[i] = ChatMessage{
			Role:    "user",
			Content: strings.Repeat("a", 50),
		}
	}

	resized, err := handler(nil, window)
	if err != nil {
		t.Fatalf("TokenAwareResizeHandlerWithMax failed: %v", err)
	}

	// 至少保留 1 条
	if len(resized) < 1 {
		t.Fatalf("should keep at least 1 message, got %d", len(resized))
	}

	// 总 tokens 应 <= 25
	totalTokens := 0
	for _, m := range resized {
		totalTokens += len(ContentToString(m.Content)) / 4
	}
	if totalTokens > 25 {
		t.Errorf("total tokens = %d, should be <= 25", totalTokens)
	}

	// 保留的应是末尾消息
	if !reflect.DeepEqual(resized[len(resized)-1], window[len(window)-1]) {
		t.Errorf("last message should be preserved, got %v", resized[len(resized)-1])
	}
}

// Task 5.6.8: TestNewPathTriggersStrategy
// 注册 simple_cut 和 summary_first
// 注册 AnalysisHandler 始终返回 "summary_first"
// AddMessage 后 ContextWindow 经过 summary_first 处理
func TestNewPathTriggersStrategy(t *testing.T) {
	s := NewSession("strategy-test", 10)
	s.AutoResize = true
	s.RegisterResizeHandler("simple_cut", SimpleCutResizeHandler)
	s.RegisterResizeHandler("summary_first", SummaryFirstResizeHandler)

	s.RegisterAnalysisHandler(func(full []ChatMessage, window []ChatMessage, memo map[string]any) (string, error) {
		return "summary_first", nil
	})

	// 添加 5 条消息
	s.AddMessage("user", "msg0 content here", "")
	s.AddMessage("user", "msg1 content here", "")
	s.AddMessage("user", "msg2 content here", "")
	s.AddMessage("user", "msg3 content here", "")
	s.AddMessage("user", "msg4 content here", "")

	// 经过 summary_first 处理后，ContextWindow 应为 [first, summary, last 2] = 4 条
	if len(s.ContextWindow) != 4 {
		t.Fatalf("ContextWindow len = %d, want 4 (after summary_first)", len(s.ContextWindow))
	}

	// 第一条应是首条消息
	if s.ContextWindow[0].Content != "msg0 content here" {
		t.Errorf("ContextWindow[0].Content = %q, want %q", s.ContextWindow[0].Content, "msg0 content here")
	}

	// 第二条应是 summary system 消息
	if s.ContextWindow[1].Role != "system" {
		t.Errorf("ContextWindow[1].Role = %q, want %q", s.ContextWindow[1].Role, "system")
	}
	if c, ok := s.ContextWindow[1].Content.(string); ok && !strings.HasPrefix(c, "[summary:") {
		t.Errorf("ContextWindow[1].Content = %q, should start with '[summary:'", s.ContextWindow[1].Content)
	}

	// 末尾两条应是 m3 和 m4
	if s.ContextWindow[2].Content != "msg3 content here" {
		t.Errorf("ContextWindow[2].Content = %q, want %q", s.ContextWindow[2].Content, "msg3 content here")
	}
	if s.ContextWindow[3].Content != "msg4 content here" {
		t.Errorf("ContextWindow[3].Content = %q, want %q", s.ContextWindow[3].Content, "msg4 content here")
	}

	// FullContext 应保留全部 5 条原始消息
	if len(s.FullContext) != 5 {
		t.Errorf("FullContext len = %d, want 5", len(s.FullContext))
	}
}
