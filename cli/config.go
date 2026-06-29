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
	"os"
	"path/filepath"
)

// CLIConfig holds the full configuration for the CLI agent.
type CLIConfig struct {
	LLM          LLMConfig    `json:"llm"`
	DataDir      string       `json:"data_dir"`
	WorkspaceDir string       `json:"workspace_dir"`
	Constitutional string     `json:"constitutional,omitempty"`
	WindowTokens int          `json:"window_tokens"`
	TopK         int          `json:"top_k"`
	UnsafeMode   bool         `json:"unsafe_mode"`
	Features     FeatureFlags `json:"features"`
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
	MemoryInjection  bool `json:"memory_injection"`  // Per-turn auto recall
	MemoryStorage    bool `json:"memory_storage"`    // Tool result auto-ingest
	Constitutional   bool `json:"constitutional"`    // Load constitutional zone
	Compression      bool `json:"compression"`       // Auto compression
	ProactiveRecall  bool `json:"proactive_recall"`  // Auto recall on session start
}

// DefaultCLIConfig returns a CLIConfig with sensible defaults.
func DefaultCLIConfig() CLIConfig {
	home, _ := os.UserHomeDir()
	return CLIConfig{
		DataDir:      filepath.Join(home, ".inferglow"),
		WorkspaceDir: ".",
		WindowTokens: 32000,
		TopK:         5,
		Features: FeatureFlags{
			MemoryInjection: true,
			MemoryStorage:   true,
			Compression:     true,
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
