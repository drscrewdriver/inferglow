// Shared provider-config loading for the server.
//
// The CLI/TUI keep their provider configuration in a JSON file (the
// ~/.inferglow/config.json schema). This file mirrors the provider-relevant
// subset of that schema so the server, the webui agent picker and the TUI all
// resolve the SAME provider list during the test phase:
//
//	etc/config.json             (project first-level, shared test config)
//	~/.inferglow/config.json    (the TUI's own config; fallback)
//
// Precedence is home < project etc/ < server YAML: YAML stays authoritative
// for server-only concerns (listen, security, workspaces), while provider
// entries absent from YAML flow in from the shared JSON.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// CLIJSONLLM is one provider route in the CLI JSON schema (cli.LLMConfig).
// EnableThinking is a server-side extension (not in the CLI schema): the
// CLI/TUI ignore it, the server uses it for chat_template_kwargs injection.
type CLIJSONLLM struct {
	Endpoint       string `json:"endpoint"`
	Model          string `json:"model"`
	APIKey         string `json:"api_key,omitempty"`
	Provider       string `json:"provider,omitempty"`
	EnableThinking bool   `json:"enable_thinking,omitempty"`
}

// CLIJSONProviders mirrors cli.ProvidersConfig (RF-1 multi-provider routes).
type CLIJSONProviders struct {
	Active string                `json:"active,omitempty"`
	List   map[string]CLIJSONLLM `json:"list,omitempty"`
}

// CLIJSONConfig is the provider-relevant subset of cli.CLIConfig. Unknown
// fields (features, tui, audit, ...) are ignored so the same file serves the
// CLI, the TUI and the server without a module dependency on cli.
type CLIJSONConfig struct {
	LLM        CLIJSONLLM        `json:"llm"`
	Providers  *CLIJSONProviders `json:"providers,omitempty"`
	Workspaces map[string]string `json:"workspaces,omitempty"`
}

// DefaultSharedConfigPaths returns the candidate shared provider-config
// locations in ascending precedence: the TUI's home config first, then the
// project first-level etc/config.json (searched upward from the working
// directory so the server can run from server/ or the repo root).
func DefaultSharedConfigPaths() []string {
	// etc/config.json candidates, most-distant ancestor first so the closest
	// one (last) wins; home comes before all of them.
	etc := []string{}
	if dir, err := os.Getwd(); err == nil {
		for i := 0; i < 6; i++ {
			etc = append(etc, filepath.Join(dir, "etc", "config.json"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	for i, j := 0, len(etc)-1; i < j; i, j = i+1, j-1 {
		etc[i], etc[j] = etc[j], etc[i]
	}
	paths := []string{}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".inferglow", "config.json"))
	}
	return append(paths, etc...)
}

// LoadSharedProviderConfig reads the shared provider config. explicit "" means
// "try every default path"; the last existing file wins (project etc/ over
// home). A missing file is not an error (nil, ""). An explicit path that
// cannot be read or parsed IS an error.
func LoadSharedProviderConfig(explicit string) (*CLIJSONConfig, string, error) {
	var candidates []string
	if explicit != "" {
		candidates = []string{explicit}
	} else {
		for _, p := range DefaultSharedConfigPaths() {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				candidates = append(candidates, p)
			}
		}
		if len(candidates) == 0 {
			return nil, "", nil
		}
	}
	path := candidates[len(candidates)-1]
	data, err := os.ReadFile(path)
	if err != nil {
		if explicit == "" {
			return nil, "", nil
		}
		return nil, "", err
	}
	cfg := &CLIJSONConfig{}
	if err := json.Unmarshal(data, cfg); err != nil {
		if explicit == "" {
			return nil, "", nil
		}
		return nil, path, err
	}
	return cfg, path, nil
}

// ToMultiLLM converts the CLI JSON provider config onto the server YAML
// MultiLLMConfig shape so agent wiring stays single-path. Mirrors the CLI
// semantics: a non-empty providers.list takes precedence over the single llm
// route.
func (c *CLIJSONConfig) ToMultiLLM() MultiLLMConfig {
	conv := func(in CLIJSONLLM) LLMConfig {
		return LLMConfig{
			Provider:       in.Provider,
			BaseURL:        in.Endpoint,
			Model:          in.Model,
			APIKey:         in.APIKey,
			EnableThinking: in.EnableThinking,
		}
	}
	if c.Providers != nil && len(c.Providers.List) > 0 {
		providers := make(map[string]LLMConfig, len(c.Providers.List))
		for name, lc := range c.Providers.List {
			providers[name] = conv(lc)
		}
		return MultiLLMConfig{Default: c.Providers.Active, Providers: providers}
	}
	if c.LLM.Endpoint == "" && c.LLM.Provider == "" {
		return MultiLLMConfig{}
	}
	name := c.LLM.Provider
	if name == "" {
		name = "default"
	}
	return MultiLLMConfig{
		Default:   name,
		Providers: map[string]LLMConfig{name: conv(c.LLM)},
	}
}
