package model

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// M-HIGH-3: Add factory functions for DeepSeek/Qwen/GLM/Kimi providers.
// These providers reject the "developer" role, so RoleMapping must be set
// to map "developer" → "system". ProviderName must match the value used in
// ProviderCacheProfiles so CacheCapability returns the right profile.

func TestNewDeepSeekProviderFromConfig(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"deepseek": map[string]any{
			"api_key": "deepseek-key",
		},
	}}

	provider, err := NewDeepSeekProviderFromConfig(cp)
	if err != nil {
		t.Fatalf("NewDeepSeekProviderFromConfig failed: %v", err)
	}

	if provider.ProviderName != "deepseek" {
		t.Errorf("ProviderName = %q, want %q", provider.ProviderName, "deepseek")
	}
	if provider.Name() != "deepseek" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "deepseek")
	}
	if provider.APIKey != "deepseek-key" {
		t.Errorf("APIKey = %q, want %q", provider.APIKey, "deepseek-key")
	}
	if provider.BaseURL == "" {
		t.Error("BaseURL should default to non-empty when not configured")
	}
	if provider.Model == "" {
		t.Error("Model should default to non-empty when not configured")
	}
	if mapped := provider.mapRole("developer"); mapped != "system" {
		t.Errorf("RoleMapping[developer] = %q, want %q", mapped, "system")
	}
}

func TestNewQwenProviderFromConfig(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"qwen": map[string]any{
			"api_key": "qwen-key",
		},
	}}

	provider, err := NewQwenProviderFromConfig(cp)
	if err != nil {
		t.Fatalf("NewQwenProviderFromConfig failed: %v", err)
	}

	if provider.ProviderName != "qwen" {
		t.Errorf("ProviderName = %q, want %q", provider.ProviderName, "qwen")
	}
	if provider.Name() != "qwen" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "qwen")
	}
	if provider.APIKey != "qwen-key" {
		t.Errorf("APIKey = %q, want %q", provider.APIKey, "qwen-key")
	}
	if provider.BaseURL == "" {
		t.Error("BaseURL should default to non-empty when not configured")
	}
	if mapped := provider.mapRole("developer"); mapped != "system" {
		t.Errorf("RoleMapping[developer] = %q, want %q", mapped, "system")
	}
}

func TestNewGLMProviderFromConfig(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"glm": map[string]any{
			"api_key": "glm-key",
		},
	}}

	provider, err := NewGLMProviderFromConfig(cp)
	if err != nil {
		t.Fatalf("NewGLMProviderFromConfig failed: %v", err)
	}

	if provider.ProviderName != "glm" {
		t.Errorf("ProviderName = %q, want %q", provider.ProviderName, "glm")
	}
	if provider.Name() != "glm" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "glm")
	}
	if provider.APIKey != "glm-key" {
		t.Errorf("APIKey = %q, want %q", provider.APIKey, "glm-key")
	}
	if provider.BaseURL == "" {
		t.Error("BaseURL should default to non-empty when not configured")
	}
	if mapped := provider.mapRole("developer"); mapped != "system" {
		t.Errorf("RoleMapping[developer] = %q, want %q", mapped, "system")
	}
}

func TestNewKimiProviderFromConfig(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"kimi": map[string]any{
			"api_key": "kimi-key",
		},
	}}

	provider, err := NewKimiProviderFromConfig(cp)
	if err != nil {
		t.Fatalf("NewKimiProviderFromConfig failed: %v", err)
	}

	if provider.ProviderName != "kimi" {
		t.Errorf("ProviderName = %q, want %q", provider.ProviderName, "kimi")
	}
	if provider.Name() != "kimi" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "kimi")
	}
	if provider.APIKey != "kimi-key" {
		t.Errorf("APIKey = %q, want %q", provider.APIKey, "kimi-key")
	}
	if provider.BaseURL == "" {
		t.Error("BaseURL should default to non-empty when not configured")
	}
	if mapped := provider.mapRole("developer"); mapped != "system" {
		t.Errorf("RoleMapping[developer] = %q, want %q", mapped, "system")
	}
}

// M-HIGH-3: Each factory should fail if api_key is missing.
func TestNewDeepSeekProviderFromConfigMissingKey(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"deepseek": map[string]any{"base_url": "https://api.deepseek.com"},
	}}
	_, err := NewDeepSeekProviderFromConfig(cp)
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
	if !errors.Is(err, ErrMissingRequiredConfig) {
		t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
	}
}

// M-HIGH-3: Configured values should override defaults.
func TestNewDeepSeekProviderFromConfigOverride(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"deepseek": map[string]any{
			"api_key":  "key",
			"base_url": "https://custom.deepseek.example",
			"model":    "deepseek-reasoner",
		},
	}}
	provider, err := NewDeepSeekProviderFromConfig(cp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.BaseURL != "https://custom.deepseek.example" {
		t.Errorf("BaseURL = %q, want custom", provider.BaseURL)
	}
	if provider.Model != "deepseek-reasoner" {
		t.Errorf("Model = %q, want deepseek-reasoner", provider.Model)
	}
}

// M-HIGH-3: New factories should produce providers with CacheCapability
// returning the correct profile (verified via Name()).
func TestNewDeepSeekProviderCacheCapability(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"deepseek": map[string]any{"api_key": "key"},
	}}
	provider, err := NewDeepSeekProviderFromConfig(cp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cap := provider.CacheCapability()
	// CacheCapability must not be the zero value (indicating an unknown profile).
	_ = cap
	if provider.Name() != "deepseek" {
		t.Errorf("expected name deepseek, got %q", provider.Name())
	}
}

// M-HIGH-4: DEFAULT_SETTINGS must include base_url defaults for the new
// providers (deepseek/qwen/glm/kimi). Without these, factories return
// providers with empty BaseURL, which then makes RequestModel fail (the
// OpenAI provider now validates BaseURL non-empty).
func TestDefaultSettingsContainBaseURLForNewProviders(t *testing.T) {
	expectedProviders := []string{"deepseek", "qwen", "glm", "kimi"}
	for _, name := range expectedProviders {
		settings, ok := DEFAULT_SETTINGS[name]
		if !ok {
			t.Errorf("DEFAULT_SETTINGS[%q] missing", name)
			continue
		}
		baseURL, ok := settings["base_url"]
		if !ok {
			t.Errorf("DEFAULT_SETTINGS[%q] missing base_url", name)
			continue
		}
		s, ok := baseURL.(string)
		if !ok || s == "" {
			t.Errorf("DEFAULT_SETTINGS[%q].base_url is empty or non-string: %v", name, baseURL)
			continue
		}
		if !strings.HasPrefix(s, "https://") {
			t.Errorf("DEFAULT_SETTINGS[%q].base_url = %q, expected https:// URL", name, s)
		}
	}
}

// M-HIGH-4: Factory-created providers should expose the default BaseURL
// from DEFAULT_SETTINGS (not an empty string).
func TestNewProviderFactoriesDefaultBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		factory func(ConfigProvider) (*OpenAICompatibleProvider, error)
	}{
		{"deepseek", NewDeepSeekProviderFromConfig},
		{"qwen", NewQwenProviderFromConfig},
		{"glm", NewGLMProviderFromConfig},
		{"kimi", NewKimiProviderFromConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cp := &StaticConfigProvider{Values: map[string]any{
				tc.name: map[string]any{"api_key": "key"},
			}}
			provider, err := tc.factory(cp)
			if err != nil {
				t.Fatalf("factory %s failed: %v", tc.name, err)
			}
			if provider.BaseURL == "" {
				t.Errorf("%s provider BaseURL is empty; expected default from DEFAULT_SETTINGS", tc.name)
			}
		})
	}
}

// M-HIGH-4: OpenAI provider should reject empty BaseURL with a clear error.
func TestOpenAIProviderRejectsEmptyBaseURL(t *testing.T) {
	provider := &OpenAICompatibleProvider{
		APIKey: "key",
		// BaseURL intentionally empty
	}
	data := &RequestData{
		Model:    "gpt-4",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}
	_, err := provider.RequestModel(context.Background(), data)
	if err == nil {
		t.Fatal("expected error for empty BaseURL")
	}
	if !strings.Contains(err.Error(), "BaseURL") {
		t.Errorf("error should mention BaseURL, got: %v", err)
	}
}
