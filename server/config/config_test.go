package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// loadBytes is a test helper that parses YAML bytes into a Config.
func loadBytes(t *testing.T, data []byte) (*Config, error) {
	t.Helper()
	cfg := &Config{}
	err := yaml.Unmarshal(data, cfg)
	return cfg, err
}

func TestConfig_LoadBytes(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    *Config
		wantErr bool
	}{
		{
			name: "basic config",
			yaml: `
llm:
  default: openai
  providers:
    openai:
      provider: openai
      api_key: test-key
      model: gpt-4
server:
  listen: ":8080"
  read_timeout: "30s"
security:
  pii_masking: true
  prompt_injection: false
flows:
  dir: "./flows"
  hot_reload: true
audit:
  enabled: true
  storage_backend: memory
  max_entries: 1000
`,
			want: &Config{
				LLM: MultiLLMConfig{
					Default: "openai",
					Providers: map[string]LLMConfig{
						"openai": {
							Provider: "openai",
							APIKey:   "test-key",
							Model:    "gpt-4",
						},
					},
				},
				Server: ServerConfig{
					Listen:      ":8080",
					ReadTimeout: "30s",
				},
				Security: SecurityConfig{
					PIIMasking:      true,
					PromptInjection: false,
				},
				Flows: FlowsConfig{
					Dir:       "./flows",
					HotReload: true,
				},
				Audit: AuditConfig{
					Enabled:        true,
					StorageBackend: "memory",
					MaxEntries:     1000,
				},
			},
		},
		{
			name: "minimal config",
			yaml: `
llm:
  provider: openai
  api_key: minimal-key
server:
  listen: ":3000"
`,
			want: &Config{
				LLM: MultiLLMConfig{
					Default: "default",
					Providers: map[string]LLMConfig{
						"default": {
							Provider: "openai",
							APIKey:   "minimal-key",
						},
					},
				},
				Server: ServerConfig{
					Listen: ":3000",
				},
			},
		},
		{
			name: "empty config",
			yaml: `{}`,
			want: &Config{},
		},
		{
			name: "api key from env var",
			yaml: `
llm:
  provider: anthropic
  api_key_env: ANTHROPIC_API_KEY
server:
  api_key_env: MY_API_KEY
`,
			want: &Config{
				LLM: MultiLLMConfig{
					Default: "default",
					Providers: map[string]LLMConfig{
						"default": {
							Provider:  "anthropic",
							APIKeyEnv: "ANTHROPIC_API_KEY",
						},
					},
				},
				Server: ServerConfig{
					APIKeyEnv: "MY_API_KEY",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadBytes(t, []byte(tt.yaml))
			if (err != nil) != tt.wantErr {
				t.Errorf("loadBytes() error = %v, wantErr = %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			assertConfigEqual(t, got, tt.want)
		})
	}
}

func TestConfig_DefaultValues(t *testing.T) {
	t.Run("zero value config", func(t *testing.T) {
		cfg := &Config{}
		if cfg.LLM.Default != "" {
			t.Errorf("expected empty default provider, got %q", cfg.LLM.Default)
		}
		if cfg.LLM.Providers != nil {
			t.Errorf("expected nil providers, got %v", cfg.LLM.Providers)
		}
		if cfg.Server.Listen != "" {
			t.Errorf("expected empty listen, got %q", cfg.Server.Listen)
		}
		if cfg.Security.PIIMasking != false {
			t.Errorf("expected PIIMasking false, got %v", cfg.Security.PIIMasking)
		}
		if cfg.Flows.HotReload != false {
			t.Errorf("expected HotReload false, got %v", cfg.Flows.HotReload)
		}
		if cfg.Audit.Enabled != false {
			t.Errorf("expected Audit.Enabled false, got %v", cfg.Audit.Enabled)
		}
	})

	t.Run("resolved default provider", func(t *testing.T) {
		m := MultiLLMConfig{
			Providers: map[string]LLMConfig{
				"b": {Provider: "b"},
				"a": {Provider: "a"},
			},
		}
		if got := m.ResolveDefault(); got != "a" {
			t.Errorf("ResolveDefault() = %q, want %q (first alphabetically)", got, "a")
		}
	})

	t.Run("explicit default provider", func(t *testing.T) {
		m := MultiLLMConfig{
			Default: "b",
			Providers: map[string]LLMConfig{
				"b": {Provider: "b"},
				"a": {Provider: "a"},
			},
		}
		if got := m.ResolveDefault(); got != "b" {
			t.Errorf("ResolveDefault() = %q, want %q", got, "b")
		}
	})

	t.Run("empty providers returns empty default", func(t *testing.T) {
		m := MultiLLMConfig{}
		if got := m.ResolveDefault(); got != "" {
			t.Errorf("ResolveDefault() = %q, want empty", got)
		}
	})
}

func TestConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name:    "invalid yaml",
			yaml:    `llm: [unclosed`,
			wantErr: true,
		},
		{
			name:    "nonsense yaml type mismatch",
			yaml:    `llm: 123`,
			wantErr: true, // yaml.v3 cannot unmarshal int into LLMConfig
		},
		{
			name:    "server as list",
			yaml:    `server: ["invalid"]`,
			wantErr: true, // yaml.v3 cannot unmarshal seq into ServerConfig
		},
		{
			name:    "completely invalid yaml",
			yaml:    "{{{{}}}}",
			wantErr: true,
		},
		{
			name:    "empty string",
			yaml:    "",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadBytes(t, []byte(tt.yaml))
			if (err != nil) != tt.wantErr {
				t.Errorf("loadBytes() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_SingleLLMBackwardCompat(t *testing.T) {
	yaml := `
llm:
  provider: openai
  api_key: legacy-key
  model: gpt-3.5-turbo
`
	cfg, err := loadBytes(t, []byte(yaml))
	if err != nil {
		t.Fatalf("loadBytes() unexpected error: %v", err)
	}

	if cfg.LLM.Default != "default" {
		t.Errorf("expected default provider name 'default', got %q", cfg.LLM.Default)
	}
	if len(cfg.LLM.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(cfg.LLM.Providers))
	}
	p := cfg.LLM.Providers["default"]
	if p.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %q", p.Provider)
	}
	if p.APIKey != "legacy-key" {
		t.Errorf("expected api_key 'legacy-key', got %q", p.APIKey)
	}
	if p.Model != "gpt-3.5-turbo" {
		t.Errorf("expected model 'gpt-3.5-turbo', got %q", p.Model)
	}
}

func TestConfig_ResolveAPIKey(t *testing.T) {
	t.Run("direct value takes precedence", func(t *testing.T) {
		s := ServerConfig{APIKey: "direct-key", APIKeyEnv: "SOME_ENV"}
		if got := s.ResolveAPIKey(); got != "direct-key" {
			t.Errorf("ResolveAPIKey() = %q, want %q", got, "direct-key")
		}
	})

	t.Run("env var fallback", func(t *testing.T) {
		t.Setenv("TEST_API_KEY", "env-key-value")
		s := ServerConfig{APIKeyEnv: "TEST_API_KEY"}
		if got := s.ResolveAPIKey(); got != "env-key-value" {
			t.Errorf("ResolveAPIKey() = %q, want %q", got, "env-key-value")
		}
	})

	t.Run("empty returns empty", func(t *testing.T) {
		s := ServerConfig{}
		if got := s.ResolveAPIKey(); got != "" {
			t.Errorf("ResolveAPIKey() = %q, want empty", got)
		}
	})

	t.Run("LLMConfig direct value", func(t *testing.T) {
		l := LLMConfig{APIKey: "llm-direct-key"}
		if got := l.ResolveAPIKey(); got != "llm-direct-key" {
			t.Errorf("ResolveAPIKey() = %q, want %q", got, "llm-direct-key")
		}
	})

	t.Run("LLMConfig env var", func(t *testing.T) {
		t.Setenv("LLM_API_KEY", "llm-env-key")
		l := LLMConfig{APIKeyEnv: "LLM_API_KEY"}
		if got := l.ResolveAPIKey(); got != "llm-env-key" {
			t.Errorf("ResolveAPIKey() = %q, want %q", got, "llm-env-key")
		}
	})
}

func TestLoader_NewLoader(t *testing.T) {
	l := NewLoader("/nonexistent/path.yaml")
	if l == nil {
		t.Fatal("NewLoader() returned nil")
	}
}

func TestLoader_Load(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := []byte(`
llm:
  default: test-provider
  providers:
    test-provider:
      provider: test
      api_key: test-key
server:
  listen: ":9999"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(path)
	cfg, err := l.Load()
	if err != nil {
		t.Fatalf("Loader.Load() unexpected error: %v", err)
	}

	if cfg.LLM.Default != "test-provider" {
		t.Errorf("expected default 'test-provider', got %q", cfg.LLM.Default)
	}
	if cfg.Server.Listen != ":9999" {
		t.Errorf("expected listen ':9999', got %q", cfg.Server.Listen)
	}

	// Verify Get() returns the same snapshot
	got := l.Get()
	if got != cfg {
		t.Error("Loader.Get() did not return the same config as Load()")
	}
}

func TestLoader_Load_FileNotFound(t *testing.T) {
	l := NewLoader("/nonexistent/path/that/does/not/exist.yaml")
	_, err := l.Load()
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestLoader_Load_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{{{{invalid}}}}}"), 0644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader(path)
	_, err := l.Load()
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

// assertConfigEqual deep-compares two Config structs for test readability.
func assertConfigEqual(t *testing.T, got, want *Config) {
	t.Helper()

	if got.LLM.Default != want.LLM.Default {
		t.Errorf("LLM.Default = %q, want %q", got.LLM.Default, want.LLM.Default)
	}

	if len(got.LLM.Providers) != len(want.LLM.Providers) {
		t.Errorf("LLM.Providers len = %d, want %d", len(got.LLM.Providers), len(want.LLM.Providers))
	} else {
		for name, wantP := range want.LLM.Providers {
			gotP, ok := got.LLM.Providers[name]
			if !ok {
				t.Errorf("missing provider %q", name)
				continue
			}
			if gotP != wantP {
				t.Errorf("LLM.Providers[%q] = %+v, want %+v", name, gotP, wantP)
			}
		}
	}

	if len(got.LLM.FallbackChain) != len(want.LLM.FallbackChain) {
		t.Errorf("LLM.FallbackChain len = %d, want %d", len(got.LLM.FallbackChain), len(want.LLM.FallbackChain))
	}

	if got.Server.Listen != want.Server.Listen {
		t.Errorf("Server.Listen = %q, want %q", got.Server.Listen, want.Server.Listen)
	}
	if got.Server.ReadTimeout != want.Server.ReadTimeout {
		t.Errorf("Server.ReadTimeout = %q, want %q", got.Server.ReadTimeout, want.Server.ReadTimeout)
	}
	if got.Server.WriteTimeout != want.Server.WriteTimeout {
		t.Errorf("Server.WriteTimeout = %q, want %q", got.Server.WriteTimeout, want.Server.WriteTimeout)
	}
	if got.Server.IdleTimeout != want.Server.IdleTimeout {
		t.Errorf("Server.IdleTimeout = %q, want %q", got.Server.IdleTimeout, want.Server.IdleTimeout)
	}
	if got.Server.APIKey != want.Server.APIKey {
		t.Errorf("Server.APIKey = %q, want %q", got.Server.APIKey, want.Server.APIKey)
	}
	if got.Server.APIKeyEnv != want.Server.APIKeyEnv {
		t.Errorf("Server.APIKeyEnv = %q, want %q", got.Server.APIKeyEnv, want.Server.APIKeyEnv)
	}
	if len(got.Server.CORSOrigins) != len(want.Server.CORSOrigins) {
		t.Errorf("Server.CORSOrigins len = %d, want %d", len(got.Server.CORSOrigins), len(want.Server.CORSOrigins))
	} else {
		for i := range got.Server.CORSOrigins {
			if got.Server.CORSOrigins[i] != want.Server.CORSOrigins[i] {
				t.Errorf("Server.CORSOrigins[%d] = %q, want %q", i, got.Server.CORSOrigins[i], want.Server.CORSOrigins[i])
			}
		}
	}
	if got.Security != want.Security {
		t.Errorf("Security = %+v, want %+v", got.Security, want.Security)
	}
	if got.Flows != want.Flows {
		t.Errorf("Flows = %+v, want %+v", got.Flows, want.Flows)
	}
	if got.Audit != want.Audit {
		t.Errorf("Audit = %+v, want %+v", got.Audit, want.Audit)
	}
}
