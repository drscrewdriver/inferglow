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

// MessageMasker is the hook interface used by Session.AddMessage to redact
// sensitive content before it is appended to the history. Unlike
// MessageHook (which can block a message), a masker transforms the content
// in place. Implementations are expected to inspect their own
// configuration and return the text unchanged when masking is disabled
// for the relevant side. A nil masker (the default) means no masking is
// performed, preserving backward compatibility.
//
// The pii.Masker type in github.com/inferglow/security/pii satisfies this
// interface and serves as the PII hook; other security modules may provide
// their own implementation. SessionOption is declared in
// security_hook.go and is reused here so PII masking and prompt-injection
// blocking share a single option pipeline.
type MessageMasker interface {
	// MaskInput transforms a message before it enters the session
	// history. Returning the input unchanged disables input masking.
	MaskInput(text string) string
	// MaskOutput transforms the final response before it is returned to
	// the caller. Returning the input unchanged disables output
	// masking. The session itself does not call MaskOutput; the agent
	// layer is responsible for invoking it on the final response.
	MaskOutput(text string) string
}

// WithMessageMasker returns a SessionOption that installs m as the
// session's PII/security masker. Pass nil to clear a previously set
// masker. This option is the session-level injection point used by the
// agent's WithPIIMasker option.
func WithMessageMasker(m MessageMasker) SessionOption {
	return func(s *Session) {
		s.masker = m
	}
}

// ChatMessage represents a single message in the conversation
type ChatMessage struct {
	Role      string         `json:"role"`
	Content   any            `json:"content"` // string | []ContentBlock
	Name      string         `json:"name,omitempty"`
	Meta      map[string]any `json:"meta,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// ResizeHandler adapts the context window when it exceeds the configured length limit.
type ResizeHandler func(fullContext []ChatMessage, contextWindow []ChatMessage) ([]ChatMessage, error)

// AnalysisHandler inspects the current full context and context window (with
// access to the session Memo) and returns the name of the resize strategy to
// apply. Returning an empty string means "do not trigger resize".
type AnalysisHandler func(full []ChatMessage, window []ChatMessage, memo map[string]any) (string, error)

// Session holds the conversation state, including the full context history and the active context window.
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
	// securityHook 在 AddMessage 前对输入做安全检测（可选）。
	// 通过 WithSecurityHook Option 注入；为 nil 时 AddMessage 行为
	// 与原始实现完全一致（向后兼容）。
	securityHook MessageHook
	// masker 在 AddMessage 前对内容做脱敏变换（可选）。
	// 通过 WithMessageMasker Option 注入；为 nil 时不做任何变换，
	// 保持向后兼容。与 securityHook 互不干扰：hook 先判断是否拦截，
	// masker 再对通过检查的内容做脱敏。
	masker MessageMasker
}

// NewSession creates a Session with the given id and maximum context length.
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

// AddMessage appends a message to both FullContext and ContextWindow.
// When a security hook is configured (via WithSecurityHook) and it
// rejects the message, the message is NOT appended and the method
// returns silently — the signature has no error return for backward
// compatibility. Use AddMessageChecked to obtain the rejection error.
func (s *Session) AddMessage(role string, content any, name string) {
	_ = s.AddMessageChecked(role, content, name)
}

// AddMessageChecked behaves like AddMessage but returns the security
// hook's rejection error (e.g. ErrPromptInjectionBlocked) instead of
// silently dropping the message. When no hook is configured the
// behavior is identical to the legacy AddMessage.
//
// When a MessageMasker is configured (via WithMessageMasker), string
// content is transformed by MaskInput after the security hook runs and
// before the message is appended. Non-string content (e.g. []ContentBlock)
// is passed through unchanged. Masking is applied only to the stored copy;
// the caller's argument is never mutated.
func (s *Session) AddMessageChecked(role string, content any, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.securityHook != nil {
		if err := s.securityHook.BeforeAddMessage(role, content, name); err != nil {
			return err
		}
	}
	// Apply PII/security masking to string content after the block check.
	// The masker itself decides (via its ApplyOn config) whether to
	// actually mask; MaskInput returns the text unchanged when input
	// masking is disabled.
	if s.masker != nil {
		if str, ok := content.(string); ok {
			content = s.masker.MaskInput(str)
		}
	}
	msg := ChatMessage{
		Role:      role,
		Content:   content,
		Name:      name,
		Timestamp: time.Now(),
	}
	s.FullContext = append(s.FullContext, msg)
	s.ContextWindow = append(s.ContextWindow, msg)

	if !s.AutoResize {
		return nil
	}
	s.applyResizeLocked()
	return nil
}

// SetMessageMasker installs (or clears, when m is nil) the PII/security
// masker consulted by AddMessageChecked. This is the imperative
// alternative to the WithMessageMasker SessionOption. The masker must
// be safe for concurrent use.
func (s *Session) SetMessageMasker(m MessageMasker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.masker = m
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
