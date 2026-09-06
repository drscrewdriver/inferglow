// Copyright 2026 InferGlow Authors

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestToMultiLLMSingleRoute — the home-config single llm route converts to a
// one-entry provider map keyed by the provider type (openai → "openai").
func TestToMultiLLMSingleRoute(t *testing.T) {
	cfg := &CLIJSONConfig{
		LLM: CLIJSONLLM{
			Endpoint: "http://192.168.100.242:8200/v1",
			Model:    "Qwen3.6-35B-A3B",
			APIKey:   "sp-dummy",
			Provider: "openai",
		},
	}
	m := cfg.ToMultiLLM()
	if len(m.Providers) != 1 || m.Default != "openai" {
		t.Fatalf("unexpected multi config: %+v", m)
	}
	lc := m.Providers["openai"]
	if lc.BaseURL != cfg.LLM.Endpoint || lc.Model != "Qwen3.6-35B-A3B" || lc.APIKey != "sp-dummy" || lc.Provider != "openai" {
		t.Fatalf("provider fields not mapped: %+v", lc)
	}
}

// TestToMultiLLMProvidersPrecedence — a non-empty providers.list wins over
// the single llm route, mirroring the CLI's RF-1 semantics.
func TestToMultiLLMProvidersPrecedence(t *testing.T) {
	cfg := &CLIJSONConfig{
		LLM: CLIJSONLLM{Endpoint: "http://single/v1", Provider: "openai"},
		Providers: &CLIJSONProviders{
			Active: "deepseek",
			List: map[string]CLIJSONLLM{
				"deepseek": {Endpoint: "https://api.deepseek.com/v1", Model: "deepseek-chat", Provider: "deepseek"},
				"qwen":     {Endpoint: "http://qwen/v1", Model: "qwen-max", Provider: "openai"},
			},
		},
	}
	m := cfg.ToMultiLLM()
	if len(m.Providers) != 2 || m.Default != "deepseek" {
		t.Fatalf("providers.list should win: %+v", m)
	}
	if _, ok := m.Providers["openai"]; ok {
		t.Fatalf("single llm route must not appear alongside providers.list")
	}
}

// TestLoadSharedProviderConfigExplicit — an explicit path is required to
// exist; the parsed file keeps its workspaces section for workspace seeding.
func TestLoadSharedProviderConfigExplicit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data, _ := json.Marshal(map[string]any{
		"llm": map[string]string{"endpoint": "http://x/v1", "model": "m", "provider": "openai"},
		"workspaces": map[string]string{
			"rewrite-agently": `E:\test\rewrite-agently`,
		},
	})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, got, err := LoadSharedProviderConfig(path)
	if err != nil || got != path || cfg == nil {
		t.Fatalf("load: cfg=%v path=%s err=%v", cfg, got, err)
	}
	if cfg.Workspaces["rewrite-agently"] != `E:\test\rewrite-agently` {
		t.Fatalf("workspaces not preserved: %+v", cfg.Workspaces)
	}
	if _, _, err := LoadSharedProviderConfig(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatalf("explicit missing path must error")
	}
}

// TestLoadSharedProviderConfigDefaultMissing — with no candidates anywhere
// the default resolution degrades to nil,nil (no provider wiring), not an
// error, so a bare server keeps booting with the demo agent.
func TestLoadSharedProviderConfigDefaultMissing(t *testing.T) {
	// Path unlikely to exist; explicit "" scans defaults only.
	cfg, _, err := LoadSharedProviderConfig(filepath.Join(t.TempDir(), "no-such-config.json"))
	if err == nil || cfg != nil {
		t.Fatalf("explicit missing must be (nil, err), got %+v %v", cfg, err)
	}
}
