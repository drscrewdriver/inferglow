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
	"sync/atomic"
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
// The pii.Masker type in the security module's pii subpackage satisfies
// this interface and serves as the PII hook; other security modules may
// provide their own implementation. SessionOption is declared in
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

// MessageStore is the capability interface for appending messages to and
// retrieving the current prompt from a session backend. Both Session and
// ThreeZoneSession implement it.
type MessageStore interface {
	// AddMessage appends a message to the conversation history.
	AddMessage(role string, content any, name string)
	// AddMessageWithMeta appends a message carrying extra metadata
	// (e.g. tool_calls, tool_call_id) to the history.
	AddMessageWithMeta(role string, content any, name string, meta map[string]any)
	// PreparePrompt returns the current context window as a ChatMessage
	// slice suitable for consumption by SessionExtension.PreparePrompt.
	PreparePrompt() []ChatMessage
}

// SessionPersistor is the capability interface for persisting session
// state to disk. Both Session and ThreeZoneSession implement SaveJSON.
//
// LoadJSON is intentionally not part of this interface because the two
// backends have incompatible LoadJSON signatures: Session.LoadJSON
// accepts a path-or-content string, while ThreeZoneSession.LoadJSON
// accepts only a file path. Callers that need crash recovery should
// depend on the concrete type or a backend-specific interface.
type SessionPersistor interface {
	// SaveJSON persists the session state to a JSON file at path.
	SaveJSON(path string) error
}

// ZoneManager is the capability interface for ThreeZone-style context
// management: Zone 1 (immutable prefix), Zone 2 (append-only history,
// populated via MessageStore), and Zone 3 (volatile scratch).
// ThreeZoneSession implements these natively; Session provides no-op
// stubs so it satisfies SessionBackend. The no-op stubs preserve the
// previous type-assertion behavior where non-supporting backends
// silently ignored zone operations.
type ZoneManager interface {
	// SetImmutablePrefix sets Zone 1 (system prompt + tool definitions).
	// Backends that do not support an immutable prefix return nil without
	// side effect.
	SetImmutablePrefix(systemPrompt string, tools []any) error
	// ClearVolatileScratch clears Zone 3 (per-round scratchpad). No-op on
	// backends without a scratchpad.
	ClearVolatileScratch()
	// BuildPrompt returns the full prompt assembled from all zones. On
	// backends without zones this is equivalent to PreparePrompt.
	BuildPrompt() []ChatMessage
}

// MaskableStore is the capability interface for installing a PII/security
// masker consulted by AddMessage. Session implements this natively;
// ThreeZoneSession provides a no-op stub so it satisfies SessionBackend,
// preserving the previous type-assertion behavior where non-supporting
// backends silently ignored the masker.
type MaskableStore interface {
	// SetMessageMasker installs (or clears, when m is nil) the masker.
	SetMessageMasker(m MessageMasker)
}

// RewritableBackend is an optional interface for session backends that
// support rewriting their active message window. ModeSummary uses this
// to replace the context window with a compacted summary + tail.
// FullContext (audit trail) is never modified by Rewrite.
type RewritableBackend interface {
	// Rewrite replaces the active message window with msgs.
	// It returns the original messages for archival.
	Rewrite(msgs []ChatMessage) []ChatMessage
}

// SessionBackend is the union of the four capability interfaces above.
// It is the interface that both Session and ThreeZoneSession implement so
// that SessionExtension can work with either backend without type
// assertions. Backends that do not natively support a capability provide
// a no-op stub (see the method docs on each split interface).
type SessionBackend interface {
	MessageStore
	SessionPersistor
	ZoneManager
	MaskableStore
}

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

	// PromptVersion 记录当前使用的 prompt template 版本。
	// 用于回放测试（F3）时将 golden session 与 prompt 版本关联。
	PromptVersion string

	// ephemeral 标记会话为进程内存态（R2）：不产生任何持久化文件。
	// 通过 NewEphemeralSession 构造；为 true 时 SaveJSON/SaveYAML
	// 成为 no-op（即使上层误配了持久化路径也不会落盘）。
	ephemeral bool
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

// forkSeq 是进程内单调递增的 fork 序号发生器，与纳秒时间戳组合，
// 保证并发 Fork 与多次 Fork 生成的新 ID 互不冲突。
var forkSeq atomic.Uint64

// newForkID 基于原会话 ID 生成新的 fork 会话 ID。
func newForkID(origID string) string {
	if origID == "" {
		origID = "session"
	}
	return fmt.Sprintf("%s-fork-%d-%d", origID, time.Now().UnixNano(), forkSeq.Add(1))
}

// Fork 深拷贝当前会话状态并生成新 ID，返回 fork 出的新会话；原会话不受影响。
//
// 深拷贝范围：
//   - FullContext / ContextWindow：逐条复制消息；Content 为 []ContentBlock 时
//     连块切片与块内 Meta 一并复制；
//   - 每条消息的 Meta 与 Memo：独立 map 副本（改 B 不影响 A）；
//   - MaxLength / AutoResize / PromptVersion：值复制；
//   - resizeHandlers / analysisHandlers / defaultResizeName：注册表独立副本
//     （handler 函数本身按引用共享，函数无会话态）；
//   - securityHook / masker：按引用共享（接口实现要求并发安全、无会话态）；
//   - ephemeral 标记随原会话继承（内存态会话 fork 出的仍是内存态）。
//
// 持久化说明：Session 本身不持有持久化句柄——JSON 落盘由调用方显式调用
// SaveJSON(path) 指定路径，JSONL usage 记录由 UsageRecorder（独立于 Session
// 构造）挂接。因此 Fork 采用与 NewSession 构造一致的最小方案：fork 出的
// 会话不自动注册任何持久化 sink，需要落盘时由调用方显式调用 SaveJSON。
func (s *Session) Fork() *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fork := &Session{
		ID:                newForkID(s.ID),
		FullContext:       deepCopyMessages(s.FullContext),
		ContextWindow:     deepCopyMessages(s.ContextWindow),
		Memo:              deepCopyMetaMap(s.Memo),
		MaxLength:         s.MaxLength,
		AutoResize:        s.AutoResize,
		PromptVersion:     s.PromptVersion,
		resizeHandlers:    make(map[string]ResizeHandler, len(s.resizeHandlers)),
		analysisHandlers:  append([]AnalysisHandler(nil), s.analysisHandlers...),
		defaultResizeName: s.defaultResizeName,
		securityHook:      s.securityHook,
		masker:            s.masker,
		ephemeral:         s.ephemeral,
	}
	for name, handler := range s.resizeHandlers {
		fork.resizeHandlers[name] = handler
	}
	return fork
}

// NewEphemeralSession 创建一个 ephemeral（进程内存态）会话（R2）。
// 签名与 NewSession 完全对齐，差异仅在于：
//   - 会话被标记为 ephemeral（可通过 IsEphemeral 查询）；
//   - SaveJSON / SaveYAML 成为 no-op——即使上层误挂了持久化路径
//     （如 SessionExtension 的 persistPath），也不会产生任何
//     JSON/JSONL/YAML 持久化文件；
//   - 上层组件（如 UsageRecorder 的 usage.jsonl 落盘）应通过
//     IsEphemeral() 识别并跳过对 ephemeral 会话的持久化。
//
// 会话状态全程留在进程内存中，进程退出即消失。
func NewEphemeralSession(id string, maxLength int) *Session {
	s := NewSession(id, maxLength)
	s.ephemeral = true
	return s
}

// IsEphemeral 报告会话是否为 ephemeral（进程内存态、不落盘）。
func (s *Session) IsEphemeral() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ephemeral
}

// deepCopyMessages 深拷贝消息切片：逐条复制消息结构，Content 为
// []ContentBlock 时连块切片与块内 Meta 一并复制，确保修改副本
// 不会波及原会话。
func deepCopyMessages(msgs []ChatMessage) []ChatMessage {
	if msgs == nil {
		return nil
	}
	out := make([]ChatMessage, len(msgs))
	for i, m := range msgs {
		cm := m
		cm.Content = deepCopyContent(m.Content)
		cm.Meta = deepCopyMetaMap(m.Meta)
		out[i] = cm
	}
	return out
}

// deepCopyContent 深拷贝消息 Content：仅处理 []ContentBlock 复合类型；
// string 等不可变标量直接复用原值。
func deepCopyContent(c any) any {
	blocks, ok := c.([]ContentBlock)
	if !ok {
		return c
	}
	out := make([]ContentBlock, len(blocks))
	for i, b := range blocks {
		cb := b
		cb.Meta = deepCopyMetaMap(b.Meta)
		out[i] = cb
	}
	return out
}

// deepCopyMetaMap 复制 map[string]any：返回新 map，键集合独立。
// 值通常为 string/int 等标量或调用方自有结构（如 tool_calls），
// 按键级独立已满足 fork 的隔离需求；对任意 any 值做通用深拷贝
// 既不可行也无必要。
func deepCopyMetaMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
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
// hook's rejection error (e.g. sessionhook.ErrPromptInjectionBlocked from
// the security module's sessionhook subpackage) instead of silently
// dropping the message. When no hook is configured the behavior is
// identical to the legacy AddMessage.
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

// AddMessageWithMeta appends a message with an explicit Meta map.
// Used by the agent loop to store tool_calls and tool_call_id alongside
// messages so PreparePrompt can forward them to the model.
//
// Like AddMessageChecked, this runs the configured security hook (prompt
// injection detection) and PII masker before appending. This closes the
// indirect-injection gap where tool results (role="tool") and MCP output
// — which flow through AddToolResult → AddMessageWithMeta — previously
// bypassed detection and masking. When the security hook rejects the
// message it is silently dropped, consistent with AddMessage's behavior;
// the signature has no error return for backward compatibility.
func (s *Session) AddMessageWithMeta(role string, content any, name string, meta map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.securityHook != nil {
		if err := s.securityHook.BeforeAddMessage(role, content, name); err != nil {
			// Blocked by the security hook; drop silently to match AddMessage.
			return
		}
	}
	// Apply PII/security masking to string content after the block check.
	if s.masker != nil {
		if str, ok := content.(string); ok {
			content = s.masker.MaskInput(str)
		}
	}
	msg := ChatMessage{
		Role:      role,
		Content:   content,
		Name:      name,
		Meta:      meta,
		Timestamp: time.Now(),
	}
	s.FullContext = append(s.FullContext, msg)
	s.ContextWindow = append(s.ContextWindow, msg)
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

	// 旧路径：analysisHandlers 为空 + ResizeHandler 非空 → 按旧逻辑。
	// 触发条件：字节数 > MaxLength。注意：MaxLength 在此路径下按字节计。
	// 新部署应优先使用 ThreeZoneSession（maxHistoryBytes 语义明确）。
	if s.ResizeHandler != nil {
		if TotalContentBytes(s.ContextWindow) > s.MaxLength {
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

// SetImmutablePrefix implements ZoneManager. A plain Session has no
// immutable-prefix zone, so this is a no-op that returns nil to preserve
// the previous type-assertion behavior (where Session silently ignored
// the call). Callers that need real prefix caching should use
// ThreeZoneSession.
func (s *Session) SetImmutablePrefix(systemPrompt string, tools []any) error {
	return nil
}

// ClearVolatileScratch implements ZoneManager. A plain Session has no
// volatile-scratch zone, so this is a no-op.
func (s *Session) ClearVolatileScratch() {}

// BuildPrompt implements ZoneManager. For a plain Session the prompt is
// just the context window, so this delegates to PreparePrompt.
func (s *Session) BuildPrompt() []ChatMessage {
	return s.PreparePrompt()
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

// Rewrite implements RewritableBackend. It replaces ContextWindow with
// the given messages and returns the original window for archival.
// FullContext is NOT modified — the audit trail is preserved.
func (s *Session) Rewrite(msgs []ChatMessage) []ChatMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.ContextWindow
	s.ContextWindow = msgs
	return old
}

// copyMessages creates a shallow copy of a ChatMessage slice.
// Caller must hold the appropriate lock (RLock or Lock) on s.mu.
func (s *Session) copyMessages(msgs []ChatMessage) []ChatMessage {
	result := make([]ChatMessage, len(msgs))
	copy(result, msgs)
	return result
}
