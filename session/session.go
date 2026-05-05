package session

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// ContentBlock 多模态内容块
type ContentBlock struct {
	Type string         `json:"type"` // "text" | "image" | "video" | "audio" | "file"
	Data any            `json:"data"` // string (text) / []byte (binary) / URL
	Meta map[string]any `json:"meta,omitempty"`
}

// ChatMessage represents a single message in the conversation
type ChatMessage struct {
	Role      string         `json:"role"`
	Content   any            `json:"content"`  // string | []ContentBlock
	Name      string         `json:"name,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

type ResizeHandler func(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error)

// AnalysisHandler inspects the current full context and context window (with
// access to the session Memo) and returns the name of the resize strategy to
// apply. Returning an empty string means "do not trigger resize".
type AnalysisHandler func(full []ChatMessage, window []ChatMessage, memo map[string]any) (string, error)

type Session struct {
	mu sync.RWMutex

	ID            string
	FullContext   []ChatMessage
	ContextWindow []ChatMessage
	Memo          map[string]any
	MaxLength     int
	AutoResize    bool
	ResizeHandler ResizeHandler

	// 多策略注册：name -> handler
	resizeHandlers map[string]ResizeHandler
	// 分析器列表：按注册顺序调用，首个返回非空策略名的生效
	analysisHandlers []AnalysisHandler
	// 默认策略名：当 AnalysisHandler 返回未注册的策略名时回退使用
	defaultResizeName string
}

func NewSession(id string, maxLength int) *Session {
	return &Session{
		ID:             id,
		FullContext:    make([]ChatMessage, 0),
		ContextWindow:  make([]ChatMessage, 0),
		Memo:           make(map[string]any),
		MaxLength:      maxLength,
		AutoResize:     false,
		resizeHandlers: make(map[string]ResizeHandler),
	}
}

// RegisterResizeHandler 注册一个命名 resize 策略。
func (s *Session) RegisterResizeHandler(name string, handler ResizeHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resizeHandlers[name] = handler
}

// RegisterAnalysisHandler 追加一个 AnalysisHandler，按注册顺序调用。
func (s *Session) RegisterAnalysisHandler(handler AnalysisHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.analysisHandlers = append(s.analysisHandlers, handler)
}

// ListResizeHandlers 返回已注册策略名（按字母升序排序）。
func (s *Session) ListResizeHandlers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.resizeHandlers))
	for name := range s.resizeHandlers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// SetDefaultResizeHandler 设置默认 resize 策略。若 name 未注册则返回错误。
func (s *Session) SetDefaultResizeHandler(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.resizeHandlers[name]; !ok {
		return fmt.Errorf("resize handler %q not registered", name)
	}
	s.defaultResizeName = name
	return nil
}

// ContentToString 将 content 字段转换为字符串用于 byte 计算
func ContentToString(c any) string {
	switch v := c.(type) {
	case string:
		return v
	case []ContentBlock:
		var parts []string
		for _, b := range v {
			if b.Type == "text" {
				if s, ok := b.Data.(string); ok {
					parts = append(parts, s)
				}
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func (s *Session) AddMessage(role string, content any, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msg := ChatMessage{
		Role:      role,
		Content:   content,
		Name:      name,
		Timestamp: time.Now(),
	}
	s.FullContext = append(s.FullContext, msg)
	s.ContextWindow = append(s.ContextWindow, msg)

	if !s.AutoResize {
		return
	}
	s.applyResizeLocked()
}

// AddChatHistory appends multiple messages to both FullContext and ContextWindow
func (s *Session) AddChatHistory(messages []ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, msg := range messages {
		s.FullContext = append(s.FullContext, ChatMessage{
			Role:      msg.Role,
			Content:   msg.Content,
			Name:      msg.Name,
			Timestamp: msg.Timestamp,
		})
	}
	s.ContextWindow = append(s.ContextWindow, s.copyMessages(messages)...)

	if !s.AutoResize {
		return
	}
	s.applyResizeLocked()
}

// SetChatHistory replaces the current history in ContextWindow only
func (s *Session) SetChatHistory(messages []ChatMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ContextWindow = s.copyMessages(messages)
}

// applyResizeLocked 在持有写锁的前提下执行 resize 调度。
// 调用方必须已持有 s.mu。
// ResizeHandler 返回 error 时记录日志并保留原始 ContextWindow（回退）。
func (s *Session) applyResizeLocked() {
	// 新路径：analysisHandlers 非空时优先走多策略调度
	if len(s.analysisHandlers) > 0 {
		// 注入 memo["max_length"] 供策略使用
		if s.Memo == nil {
			s.Memo = make(map[string]any)
		}
		s.Memo["max_length"] = s.MaxLength

		for _, analyzer := range s.analysisHandlers {
			strategyName, err := analyzer(s.FullContext, s.ContextWindow, s.Memo)
			if err != nil {
				continue
			}
			if strategyName == "" {
				continue
			}
			// 找到策略，执行 resize
			if handler, ok := s.resizeHandlers[strategyName]; ok {
				resized, err := handler(s.FullContext, s.ContextWindow)
				if err != nil {
					log.Printf("session resize handler %q returned error: %v (falling back to original window)", strategyName, err)
					return
				}
				s.ContextWindow = resized
				return
			}
			// 策略名未注册，回退到 defaultResizeName
			if s.defaultResizeName != "" {
				if handler, ok := s.resizeHandlers[s.defaultResizeName]; ok {
					resized, err := handler(s.FullContext, s.ContextWindow)
					if err != nil {
						log.Printf("session default resize handler %q returned error: %v (falling back to original window)", s.defaultResizeName, err)
						return
					}
					s.ContextWindow = resized
					return
				}
			}
		}
		return
	}

	// 旧路径：analysisHandlers 为空 + ResizeHandler 非空 → 按旧逻辑
	if s.ResizeHandler != nil {
		totalBytes := 0
		for _, m := range s.ContextWindow {
			totalBytes += len(ContentToString(m.Content))
		}
		if totalBytes > s.MaxLength {
			resized, err := s.ResizeHandler(s.FullContext, s.ContextWindow)
			if err != nil {
				log.Printf("session resize handler returned error: %v (falling back to original window)", err)
				return
			}
			s.ContextWindow = resized
		}
	}
}

// contentToPromptString 将单个消息的 content 字段转换为 prompt 字符串
func contentToPromptString(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []ContentBlock:
		var parts []string
		for _, block := range v {
			switch block.Type {
			case "text":
				if s, ok := block.Data.(string); ok {
					parts = append(parts, s)
				}
			case "image", "video", "audio", "file":
				parts = append(parts, fmt.Sprintf("[%s referenced]", block.Type))
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

// PreparePrompt returns a copy of ContextWindow, serializing ContentBlock to string
func (s *Session) PreparePrompt() []ChatMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prompt := make([]ChatMessage, len(s.ContextWindow))
	for i, msg := range s.ContextWindow {
		prompt[i] = msg
		if blocks, ok := msg.Content.([]ContentBlock); ok {
			var parts []string
			for _, block := range blocks {
				if block.Type == "text" {
					if s, ok := block.Data.(string); ok {
						parts = append(parts, s)
					}
				}
			}
			prompt[i].Content = strings.Join(parts, "")
		}
	}
	return prompt
}

// GetFullContext returns the full history as a copy
func (s *Session) GetFullContext() []ChatMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.copyMessages(s.FullContext)
}

// GetContextWindow returns the current window as a copy
func (s *Session) GetContextWindow() []ChatMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.copyMessages(s.ContextWindow)
}

// copyMessages creates a shallow copy of a ChatMessage slice.
// Caller must hold the appropriate lock (RLock or Lock) on s.mu.
func (s *Session) copyMessages(msgs []ChatMessage) []ChatMessage {
	result := make([]ChatMessage, len(msgs))
	copy(result, msgs)
	return result
}
