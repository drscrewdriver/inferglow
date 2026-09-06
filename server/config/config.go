// Package config loads InferGlow's YAML configuration and supports hot reload.
//
// Recycled from the inferflow project and adapted for the inferglow server
// module. Provides multi-provider LLM config, security toggles, and
// fsnotify-based hot reload.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration parsed from a YAML file.
type Config struct {
	LLM MultiLLMConfig `yaml:"llm"`
	// Workspaces seeds the server's workspace registry at startup
	// (name → absolute root directory), e.g. for the webui workspace selector.
	Workspaces map[string]string `yaml:"workspaces"`
	Server     ServerConfig      `yaml:"server"`
	Security   SecurityConfig    `yaml:"security"`
	Flows      FlowsConfig       `yaml:"flows"`
	Audit      AuditConfig       `yaml:"audit"`
}

// AuditConfig configures the audit chain for request/response logging
// with cryptographic integrity verification.
type AuditConfig struct {
	// Enabled toggles audit chain creation. When false, no audit overhead.
	Enabled bool `yaml:"enabled"`
	// StorageBackend is "memory" (default) or "json_file".
	StorageBackend string `yaml:"storage_backend"`
	// StoragePath is the directory for JSONL audit logs (json_file backend).
	StoragePath string `yaml:"storage_path"`
	// SignatureKey is an optional HMAC-SHA256 key for entry signing.
	SignatureKey string `yaml:"signature_key"`
	// MaxEntries limits in-memory storage (0 = unlimited).
	MaxEntries int `yaml:"max_entries"`
}

// ServerConfig configures the HTTP server.
type ServerConfig struct {
	Listen       string   `yaml:"listen"`        // e.g. ":8080"
	ReadTimeout  string   `yaml:"read_timeout"`  // e.g. "30s"
	WriteTimeout string   `yaml:"write_timeout"` // e.g. "60s"
	IdleTimeout  string   `yaml:"idle_timeout"`  // e.g. "120s"
	APIKey       string   `yaml:"api_key"`       // Bearer token (empty = disabled)
	APIKeyEnv    string   `yaml:"api_key_env"`   // Env var for API key
	CORSOrigins  []string `yaml:"cors_origins"`
}

// ResolveAPIKey returns the API key: direct value first, then env var.
func (c ServerConfig) ResolveAPIKey() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	if c.APIKeyEnv != "" {
		return os.Getenv(c.APIKeyEnv)
	}
	return ""
}

// MultiLLMConfig supports multiple named LLM providers with a default
// and an optional fallback chain.
type MultiLLMConfig struct {
	Default       string               `yaml:"default"`
	Providers     map[string]LLMConfig `yaml:"providers"`
	FallbackChain []string             `yaml:"fallback_chain"`
}

// ResolveDefault returns the default provider name.
func (m MultiLLMConfig) ResolveDefault() string {
	if m.Default != "" {
		return m.Default
	}
	if len(m.Providers) == 0 {
		return ""
	}
	first := ""
	for name := range m.Providers {
		if first == "" || name < first {
			first = name
		}
	}
	return first
}

// UnmarshalYAML implements custom unmarshaling for backward compatibility.
func (m *MultiLLMConfig) UnmarshalYAML(unmarshal func(any) error) error {
	type multiAlias MultiLLMConfig
	var multi multiAlias
	if err := unmarshal(&multi); err == nil && len(multi.Providers) > 0 {
		*m = MultiLLMConfig(multi)
		return nil
	}
	var single LLMConfig
	if err := unmarshal(&single); err != nil {
		return err
	}
	if single.Provider == "" {
		*m = MultiLLMConfig{}
		return nil
	}
	*m = MultiLLMConfig{
		Default:   "default",
		Providers: map[string]LLMConfig{"default": single},
	}
	return nil
}

// LLMConfig configures a single LLM provider.
type LLMConfig struct {
	Provider  string `yaml:"provider"`
	BaseURL   string `yaml:"base_url"`
	Model     string `yaml:"model"`
	APIKey    string `yaml:"api_key"`
	APIKeyEnv string `yaml:"api_key_env"`
	Timeout   string `yaml:"timeout"`
	ForceJSON bool   `yaml:"force_json"`
	// EnableThinking injects chat_template_kwargs.enable_thinking=true into
	// every request (vLLM-hosted Qwen3-family models gate their reasoning
	// on this flag; --reasoning-parser then splits it into the reasoning
	// field). Opt-in: other OpenAI-compatible servers may reject the
	// unknown body field.
	EnableThinking bool `yaml:"enable_thinking"`
}

// ResolveAPIKey returns the API key: direct value first, then env var.
func (c LLMConfig) ResolveAPIKey() string {
	if c.APIKey != "" {
		return c.APIKey
	}
	if c.APIKeyEnv != "" {
		return os.Getenv(c.APIKeyEnv)
	}
	return ""
}

// SecurityConfig toggles PII masking and prompt-injection scanning.
type SecurityConfig struct {
	PIIMasking      bool `yaml:"pii_masking"`
	PromptInjection bool `yaml:"prompt_injection"`
}

// FlowsConfig configures flow loading behaviour.
type FlowsConfig struct {
	Dir       string `yaml:"dir"`        // directory to auto-load *.yaml flows
	HotReload bool   `yaml:"hot_reload"` // watch dir for changes
}

// Loader reads and watches the config file. Safe for concurrent use.
type Loader struct {
	path string

	mu      sync.RWMutex
	cur     *Config
	watcher *fsnotify.Watcher
}

// NewLoader returns a Loader bound to path.
func NewLoader(path string) *Loader {
	return &Loader{path: path}
}

// Load reads and parses the YAML config file.
func (l *Loader) Load() (*Config, error) {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", l.path, err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", l.path, err)
	}
	l.mu.Lock()
	l.cur = cfg
	l.mu.Unlock()
	return cfg, nil
}

// Get returns the current config snapshot.
func (l *Loader) Get() *Config {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.cur
}

// Watch starts watching the config file for changes and invokes onChange
// with the new config. The parent directory is watched so atomic
// replacements are observed across platforms.
func (l *Loader) Watch(onChange func(*Config)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	dir := filepath.Dir(l.path)
	if err := w.Add(dir); err != nil {
		w.Close()
		return fmt.Errorf("watch dir %s: %w", dir, err)
	}

	l.mu.Lock()
	l.watcher = w
	l.mu.Unlock()

	target := filepath.Base(l.path)
	go func() {
		for {
			select {
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) != target {
					continue
				}
				if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}
				// Brief debounce for partial writes.
				time.Sleep(50 * time.Millisecond)
				cfg, err := l.Load()
				if err != nil {
					continue
				}
				onChange(cfg)
			case _, ok := <-w.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return nil
}

// StopWatch stops the watch goroutine.
func (l *Loader) StopWatch() {
	l.mu.Lock()
	w := l.watcher
	l.watcher = nil
	l.mu.Unlock()
	if w != nil {
		w.Close()
	}
}
