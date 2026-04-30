package session

import (
	"encoding/json"
	"os"

	"gopkg.in/yaml.v3"
)

type SessionData struct {
	ID            string         `json:"id" yaml:"id"`
	FullContext   []ChatMessage  `json:"full_context" yaml:"full_context"`
	ContextWindow []ChatMessage  `json:"context_window" yaml:"context_window"`
	Memo          map[string]any `json:"memo" yaml:"memo"`
	MaxLength     int            `json:"max_length" yaml:"max_length"`
	AutoResize    bool           `json:"auto_resize" yaml:"auto_resize"`
}

func (s *Session) ToJSON() (string, error) {
	data := s.toSessionData()
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (s *Session) ToYAML() (string, error) {
	data := s.toSessionData()
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
	return nil
}

func (s *Session) toSessionData() SessionData {
	return SessionData{
		ID:            s.ID,
		FullContext:   s.FullContext,
		ContextWindow: s.ContextWindow,
		Memo:          s.Memo,
		MaxLength:     s.MaxLength,
		AutoResize:    s.AutoResize,
	}
}
