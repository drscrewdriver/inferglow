package session

import (
	"encoding/json"
	"os"

	"gopkg.in/yaml.v3"
)

// 内置 ResizeHandler 注册表：name -> handler。
// LoadJSON 时通过此注册表恢复内置 handler。
var builtinResizeHandlers = map[string]ResizeHandler{}

func init() {
	// 注册内置 ResizeHandler，供 SaveJSON/LoadJSON 通过名字恢复
	builtinResizeHandlers["simple_cut"] = SimpleCutResizeHandler
	builtinResizeHandlers["summary_first"] = SummaryFirstResizeHandler
	builtinResizeHandlers["token_aware"] = TokenAwareResizeHandler
}

type SessionData struct {
	ID            string         `json:"id" yaml:"id"`
	FullContext   []ChatMessage  `json:"full_context" yaml:"full_context"`
	ContextWindow []ChatMessage  `json:"context_window" yaml:"context_window"`
	Memo          map[string]any `json:"memo" yaml:"memo"`
	MaxLength     int            `json:"max_length" yaml:"max_length"`
	AutoResize    bool           `json:"auto_resize" yaml:"auto_resize"`

	// ResizeHandlerNames 保存注册的 resize 策略名（仅内置策略可在 LoadJSON 时恢复）。
	// 自定义策略在 LoadJSON 后不会被恢复，需要调用方重新注册。
	ResizeHandlerNames []string `json:"resize_handlers,omitempty" yaml:"resize_handlers,omitempty"`
	// DefaultResizeName 保存默认 resize 策略名（用于 LoadJSON 恢复）。
	DefaultResizeName string `json:"default_resize_name,omitempty" yaml:"default_resize_name,omitempty"`
	// AnalysisHandlerCount 保存 AnalysisHandler 数量（仅作信息记录，函数无法序列化）。
	AnalysisHandlerCount int `json:"analysis_handler_count,omitempty" yaml:"analysis_handler_count,omitempty"`
}

func (s *Session) ToJSON() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data := s.toSessionDataLocked()
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (s *Session) ToYAML() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data := s.toSessionDataLocked()
	bytes, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (s *Session) SaveJSON(path string) error {
	data, err := s.ToJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(data), 0644)
}

func (s *Session) SaveYAML(path string) error {
	data, err := s.ToYAML()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(data), 0644)
}

func (s *Session) LoadJSON(pathOrContent string) error {
	var data SessionData
	content := []byte(pathOrContent)
	// Treat path ending with .json as a file path
	if len(pathOrContent) >= 5 && pathOrContent[len(pathOrContent)-5:] == ".json" {
		fileData, err := os.ReadFile(pathOrContent)
		if err == nil {
			content = fileData
		}
	}
	if err := json.Unmarshal(content, &data); err != nil {
		return err
	}
	return s.LoadFromData(data)
}

func (s *Session) LoadYAML(pathOrContent string) error {
	var data SessionData
	content := []byte(pathOrContent)
	// Treat path ending with .yaml as a file path
	if len(pathOrContent) >= 5 && pathOrContent[len(pathOrContent)-5:] == ".yaml" {
		fileData, err := os.ReadFile(pathOrContent)
		if err == nil {
			content = fileData
		}
	}
	if err := yaml.Unmarshal(content, &data); err != nil {
		return err
	}
	return s.LoadFromData(data)
}

func (s *Session) LoadFromData(data SessionData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ID = data.ID
	s.FullContext = make([]ChatMessage, len(data.FullContext))
	copy(s.FullContext, data.FullContext)
	s.ContextWindow = make([]ChatMessage, len(data.ContextWindow))
	copy(s.ContextWindow, data.ContextWindow)
	s.Memo = make(map[string]any, len(data.Memo))
	for k, v := range data.Memo {
		s.Memo[k] = v
	}
	s.MaxLength = data.MaxLength
	s.AutoResize = data.AutoResize

	// 恢复内置 resize handler
	if len(s.resizeHandlers) == 0 {
		s.resizeHandlers = make(map[string]ResizeHandler)
	}
	for _, name := range data.ResizeHandlerNames {
		if handler, ok := builtinResizeHandlers[name]; ok {
			s.resizeHandlers[name] = handler
		}
	}
	// 恢复默认策略名（仅当对应的 handler 已成功恢复时）
	if data.DefaultResizeName != "" {
		if _, ok := s.resizeHandlers[data.DefaultResizeName]; ok {
			s.defaultResizeName = data.DefaultResizeName
		} else {
			s.defaultResizeName = ""
		}
	}
	return nil
}

// toSessionDataLocked 在持有读锁的前提下构建 SessionData。
// 调用方必须已持有 s.mu（RLock 或 Lock）。
func (s *Session) toSessionDataLocked() SessionData {
	// 收集 resize handler 名字（仅内置可恢复，但保存所有名字以便诊断/迁移）
	handlerNames := make([]string, 0, len(s.resizeHandlers))
	for name := range s.resizeHandlers {
		handlerNames = append(handlerNames, name)
	}
	return SessionData{
		ID:                   s.ID,
		FullContext:          s.FullContext,
		ContextWindow:        s.ContextWindow,
		Memo:                 s.Memo,
		MaxLength:            s.MaxLength,
		AutoResize:           s.AutoResize,
		ResizeHandlerNames:   handlerNames,
		DefaultResizeName:    s.defaultResizeName,
		AnalysisHandlerCount: len(s.analysisHandlers),
	}
}
