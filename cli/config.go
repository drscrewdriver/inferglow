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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ProvidersConfig holds multiple provider endpoint configurations (RF-1).
// When List is non-empty, multi-provider routing takes precedence over the
// legacy single-route `llm` field.
type ProvidersConfig struct {
	// Active is the currently active provider key (omitted = fall back to
	// ~/.inferglow/model.json or the single-route llm).
	Active string `json:"active,omitempty"`
	// List maps a provider key to its full LLM configuration. The key is
	// also used as the route's provider name.
	List map[string]LLMConfig `json:"list,omitempty"`
}

// CLIConfig holds the full configuration for the CLI agent.
type CLIConfig struct {
	LLM          LLMConfig      `json:"llm"`
	Providers    ProvidersConfig `json:"providers,omitempty"` // RF-1: multi-provider routes
	DataDir      string         `json:"data_dir"`
	WorkspaceDir string       `json:"workspace_dir"`
	Constitutional string     `json:"constitutional,omitempty"`
	WindowTokens int          `json:"window_tokens"`
	TopK         int          `json:"top_k"`
	UnsafeMode   bool         `json:"unsafe_mode"`
	SandboxMode  string       `json:"sandbox_mode,omitempty"` // "trusted_local", "local", "docker", "gvisor", "auto"
	Features     FeatureFlags `json:"features"`
	Audit        AuditConfig  `json:"audit,omitempty"`
	TUI          TUIConfig    `json:"tui,omitempty"`
	// MC-1: context management mode (passthrough/three_zone/summary/hybrid).
	ContextMode string `json:"context_mode,omitempty"`
	// MC-2: dedicated compression model; nil = fallback to main LLM.
	CompressModel *LLMConfig `json:"compress_model,omitempty"`
}

// AuditConfig controls the audit trail for the CLI agent.
type AuditConfig struct {
	Enabled      bool   `json:"enabled"`
	StoragePath  string `json:"storage_path,omitempty"`
	SignatureKey string `json:"signature_key,omitempty"`
}

// TUIConfig holds TUI-specific display and behavior settings.
type TUIConfig struct {
	Theme         string `json:"theme,omitempty"`          // "dark", "light", "auto"
	ShowReasoning bool   `json:"show_reasoning"`           // display LLM reasoning steps
	MaxScrollback int    `json:"max_scrollback,omitempty"` // max transcript lines (0=unlimited)
	// ReasoningEffort is the /effort level: "low"|"medium"|"high"|"" (RF-2).
	// "" = provider default (also used as the fallback when effort.json is absent).
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	// HealthCheckInterval is the API health-check period in seconds (RF-10).
	// Defaults to 60 when zero; clamped to [10,600].
	HealthCheckInterval int `json:"health_check_interval,omitempty"`
	// HealthProbeMode selects the health-check probe strategy (RF-10):
	// "tcp" (default, TCP dial) | "http" (GET {endpoint}/models) | "off".
	HealthProbeMode string `json:"health_probe_mode,omitempty"`
	// EffortScales overrides the per-model reasoning-effort scales (RF-2
	// extension). Keys are "provider" or "provider/model"; each value maps a
	// level name ("off"|"low"|"medium"|"high"|"max"|...) to its injection
	// params (empty = no injection). Exact provider/model wins over provider,
	// which wins over the built-in defaults.
	EffortScales map[string]map[string]EffortScaleLevelCfg `json:"effort_scales,omitempty"`
}

// EffortScaleLevelCfg configures one effort level in tui.effort_scales.
type EffortScaleLevelCfg struct {
	Label  string         `json:"label,omitempty"`
	Params map[string]any `json:"params,omitempty"` // injected into Options; empty = none
}

// LLMConfig holds LLM endpoint configuration.
type LLMConfig struct {
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key,omitempty"`
	Provider string `json:"provider,omitempty"` // "openai", "deepseek", "anthropic", etc.
}

// FeatureFlags controls optional features.
type FeatureFlags struct {
	MemoryInjection  bool   `json:"memory_injection"`  // Per-turn auto recall
	MemoryStorage    bool   `json:"memory_storage"`    // Tool result auto-ingest
	Constitutional   bool   `json:"constitutional"`    // Load constitutional zone
	MetaInstructions bool   `json:"meta_instructions"` // CM-3: inject tool/background/compression guidance into Zone 0.5
	Compression      bool   `json:"compression"`       // Auto compression
	ProactiveRecall  bool   `json:"proactive_recall"`  // Auto recall on session start
	RuntimeModeSwitch bool  `json:"runtime_mode_switch"` // MC-3: enable /mode TUI command
	TUIMode          bool   `json:"tui_mode"`          // Enable full-screen TUI mode
	AutoBackground   bool   `json:"auto_background"`   // CM-2: auto-trigger /rebackground when Zone 1 (head buffer) is empty; false disables the auto project-analysis tool loop
	OutputMode       string `json:"output_mode"`       // "tui", "cli", or "oneshot"; mirrors CLI flag dispatch
	SlashCompat      bool   `json:"slash_compat"`      // SC-1: accept claude/pi/opencode/codex slash commands via alias catalog (default true)
	SlashPopup       bool   `json:"slash_popup"`       // SC-2: IME-style "/" prefix autocomplete popup (default true)
	TaskPanel        bool   `json:"task_panel"`        // SC-3: right-side task list panel (default true)
	MessageActions   bool   `json:"message_actions"`   // SC-4: history message action menu (default true)
	WorkspaceSwitch  bool   `json:"workspace_switch"`  // SC-5: workspace directory switching (default true)
	SkillLoader      bool   `json:"skill_loader"`      // SC-6: load ~/.agents/skills as slash commands (default true)
	ModelSwitch      bool   `json:"model_switch"`      // RF-1: runtime multi-provider/model switching (default true)
	EffortControl    bool   `json:"effort"`            // RF-2: /effort reasoning-level control (default true)
	ThemeSwitch      bool   `json:"theme_switch"`      // RF-3: /theme real switching (default true)
	InputHistory     bool   `json:"input_history"`     // RF-5: persisted input history (default true)
	TurnStats        bool   `json:"turn_stats"`        // RF-6: per-turn stats (thinking/tool durations) (default true)
	TPS              bool   `json:"tps"`               // RF-7: TPS output efficiency (default true)
	CacheHit         bool   `json:"cache_hit"`         // RF-8: cache hit rate (default true)
	Welcome          bool   `json:"welcome"`           // RF-9: startup welcome page (default true)
	HealthCheck      bool   `json:"health_check"`      // RF-10: API health check (default true)
}

// DefaultCLIConfig returns a CLIConfig with sensible defaults.
func DefaultCLIConfig() CLIConfig {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".inferglow")
	return CLIConfig{
		DataDir:      dataDir,
		WorkspaceDir: ".",
		WindowTokens: 32000,
		TopK:         5,
		SandboxMode:  "trusted_local",
		Constitutional: filepath.Join(dataDir, "constitutional", "rules.md"),
		Audit: AuditConfig{
			Enabled: false,
		},
		Features: FeatureFlags{
			MemoryInjection:   true,
			MemoryStorage:     true,
			Compression:       true,
			Constitutional:    true,
			MetaInstructions:  true,
			RuntimeModeSwitch: true,
			TUIMode:           true,
			AutoBackground:    true,
			SlashCompat:       true,
			SlashPopup:        true,
			TaskPanel:         true,
			MessageActions:    true,
			WorkspaceSwitch:   true,
			SkillLoader:       true,
			ModelSwitch:       true,
			EffortControl:     true,
			ThemeSwitch:       true,
			InputHistory:      true,
			TurnStats:         true,
			TPS:               true,
			CacheHit:          true,
			Welcome:           true,
			HealthCheck:       true,
		},
		ContextMode:  "hybrid",
		TUI: TUIConfig{
			Theme:              "dark",
			ShowReasoning:      false,
			HealthCheckInterval: 60,
			HealthProbeMode:     "tcp",
		},
	}
}

// LoadConfig reads a JSON config file and returns the parsed CLIConfig.
func LoadConfig(path string) (CLIConfig, error) {
	cfg := DefaultCLIConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// DefaultConfigPath returns the default config file location.
func DefaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".inferglow", "config.json")
}

// SaveConfig writes the config to the given path as JSON.
func SaveConfig(cfg CLIConfig, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadOrDefaultConfig tries to load from the given path, then the default
// location (~/.inferglow/config.json). If neither exists, it returns the
// default config and persists it to disk for future use.
func LoadOrDefaultConfig(explicitPath string) (CLIConfig, string, error) {
	cfg := DefaultCLIConfig()

	// Try explicit path first.
	if explicitPath != "" {
		loaded, err := LoadConfig(explicitPath)
		return loaded, explicitPath, err
	}

	// Try default location.
	defaultPath := DefaultConfigPath()
	if _, err := os.Stat(defaultPath); err == nil {
		loaded, err := LoadConfig(defaultPath)
		return loaded, defaultPath, err
	}

	// No config file found: persist defaults.
	if err := SaveConfig(cfg, defaultPath); err != nil {
		return cfg, defaultPath, nil // non-fatal: just use in-memory defaults
	}
	return cfg, defaultPath, nil
}

// EnsureDataDirs creates the full directory structure under DataDir.
// Called once at startup to guarantee all required subdirectories exist.
//
// Directory layout:
//
//	~/.inferglow/
//	├── config.json                  # long-term config (auto-created by SaveConfig)
//	├── constitutional/              # Zone 0.5 rules & meta-instructions
//	│   └── rules.md
//	├── sessions/                    # session JSONL files (L0 + refs)
//	│   └── index.jsonl              # session index
//	├── audit/                      # audit trail JSONL files
//	├── memory/                      # long-term memory store
//	├── skills/                      # global skill store
//	│   └── global/
//	└── projects/                    # per-project data
//	    └── default/
//	        └── skills/
func EnsureDataDirs(dataDir string) error {
	dirs := []string{
		filepath.Join(dataDir, "constitutional"),
		filepath.Join(dataDir, "sessions"),
		filepath.Join(dataDir, "memory"),
		filepath.Join(dataDir, "skills", "global"),
		filepath.Join(dataDir, "projects", "default", "skills"),
		filepath.Join(dataDir, "audit"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("ensure data dir %s: %w", d, err)
		}
	}
	return nil
}

// ApplyEnvOverrides applies environment variable overrides to the config.
// Supported environment variables:
//   - LLM_ENDPOINT: API endpoint
//   - LLM_MODEL: Model name
//   - LLM_API_KEY: API key
//   - LLM_PROVIDER: Provider type (openai, deepseek, anthropic, etc.)
func ApplyEnvOverrides(cfg *CLIConfig) {
	if v := os.Getenv("LLM_ENDPOINT"); v != "" {
		cfg.LLM.Endpoint = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
	if v := os.Getenv("LLM_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("LLM_PROVIDER"); v != "" {
		cfg.LLM.Provider = v
	}
	// MC-2: dedicated compression model override.
	if v := os.Getenv("COMPRESS_MODEL"); v != "" {
		if cfg.CompressModel == nil {
			cfg.CompressModel = &LLMConfig{}
		}
		cfg.CompressModel.Model = v
	}
}
