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

package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// RunInitWizard performs an interactive first-run configuration wizard (DC-5).
// It prompts the user for LLM endpoint, model, API key, and provider, then
// writes the resulting config to the default config path.
func RunInitWizard() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║     InferGlow First-Run Configuration    ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Println()

	cfg := DefaultCLIConfig()

	// Endpoint (required).
	fmt.Print("LLM API Endpoint (e.g. https://api.openai.com/v1): ")
	endpoint, _ := reader.ReadString('\n')
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	cfg.LLM.Endpoint = endpoint

	// Model.
	fmt.Print("Model name [gpt-4o]: ")
	model, _ := reader.ReadString('\n')
	model = strings.TrimSpace(model)
	if model == "" {
		model = "gpt-4o"
	}
	cfg.LLM.Model = model

	// API Key (optional, can be set via env).
	fmt.Print("API Key (leave empty to use OPENAI_API_KEY env): ")
	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)
	cfg.LLM.APIKey = apiKey

	// Provider.
	fmt.Print("Provider [openai]: ")
	provider, _ := reader.ReadString('\n')
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = "openai"
	}
	cfg.LLM.Provider = provider

	// --- Audit trail configuration ---

	// Enable audit trail.
	fmt.Println()
	fmt.Print("Enable audit trail? (y/N): ")
	auditEnabled, _ := reader.ReadString('\n')
	auditEnabled = strings.TrimSpace(strings.ToLower(auditEnabled))
	cfg.Audit.Enabled = auditEnabled == "y" || auditEnabled == "yes"

	if cfg.Audit.Enabled {
		// Storage path.
		defaultAuditPath := cfg.DataDir + "/audit/"
		fmt.Printf("Audit storage path [%s]: ", defaultAuditPath)
		auditPath, _ := reader.ReadString('\n')
		auditPath = strings.TrimSpace(auditPath)
		if auditPath == "" {
			auditPath = defaultAuditPath
		}
		cfg.Audit.StoragePath = auditPath

		// Enable signature.
		fmt.Print("Enable audit trail signing? (y/N): ")
		signEnabled, _ := reader.ReadString('\n')
		signEnabled = strings.TrimSpace(strings.ToLower(signEnabled))
		if signEnabled == "y" || signEnabled == "yes" {
			fmt.Print("Audit signature key: ")
			sigKey, _ := reader.ReadString('\n')
			sigKey = strings.TrimSpace(sigKey)
			cfg.Audit.SignatureKey = sigKey
		}
	}

	// Ensure data directories exist.
	if err := EnsureDataDirs(cfg.DataDir); err != nil {
		return fmt.Errorf("create data dirs: %w", err)
	}

	// Save config.
	cfgPath := cfg.DataDir + "/config.json"
	if err := SaveConfig(cfg, cfgPath); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Println()
	fmt.Printf("✓ Configuration saved to %s\n", cfgPath)
	fmt.Println("  You can now start InferGlow with: inferglow")
	return nil
}

// CheckFirstRun returns true if the configuration has no LLM endpoint
// configured (neither in config nor via environment), indicating that
// the user should run `inferglow init`.
func CheckFirstRun(cfg CLIConfig) bool {
	if cfg.LLM.Endpoint != "" {
		return false
	}
	// Check common env vars.
	for _, env := range []string{"OPENAI_API_KEY", "OPENAI_BASE_URL", "LLM_ENDPOINT"} {
		if os.Getenv(env) != "" {
			return false
		}
	}
	return true
}
