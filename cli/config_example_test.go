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
	"reflect"
	"testing"
)

// TestConfigExampleFileParses verifies that cli/examples/config.example.json
// (the reference multi-provider configuration) deserializes into CLIConfig
// with every documented field intact. Guards against drift between the
// example and the config schema.
func TestConfigExampleFileParses(t *testing.T) {
	path := filepath.Join("examples", "config.example.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var cfg CLIConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}

	// llm single-route fallback present.
	if cfg.LLM.Endpoint == "" || cfg.LLM.Model == "" {
		t.Fatalf("llm missing endpoint/model: %+v", cfg.LLM)
	}

	// providers.list multi-route: every key carries a full LLMConfig.
	if len(cfg.Providers.List) == 0 {
		t.Fatal("providers.list empty — example should declare multiple providers")
	}
	for key, lc := range cfg.Providers.List {
		if lc.Endpoint == "" || lc.Model == "" || lc.Provider == "" {
			t.Errorf("providers.list[%q] incomplete: %+v", key, lc)
		}
	}
	// active must reference a listed key.
	if _, ok := cfg.Providers.List[cfg.Providers.Active]; !ok {
		t.Errorf("providers.active = %q not in list", cfg.Providers.Active)
	}

	// New provider keys from the LLM-provider-port must be present.
	for _, want := range []string{"google", "deepseek", "openrouter", "zai", "mistral", "groq", "xai", "together", "nvidia", "cerebras", "huggingface", "fireworks", "moonshotai", "qwen-token-plan-cn"} {
		if _, ok := cfg.Providers.List[want]; !ok {
			t.Errorf("providers.list missing %q", want)
		}
	}

	// tui.effort_scales custom overrides parse.
	if len(cfg.TUI.EffortScales) == 0 {
		t.Error("tui.effort_scales empty — example should demonstrate overrides")
	}
	if _, ok := cfg.TUI.EffortScales["deepseek"]; !ok {
		t.Error("tui.effort_scales missing deepseek override")
	}

	// RF feature gates present.
	for _, f := range []string{
		"ModelSwitch", "EffortControl", "ThemeSwitch", "InputHistory",
		"TurnStats", "TPS", "CacheHit", "Welcome", "HealthCheck",
	} {
		v := featureFlagValue(cfg.Features, f)
		if !v {
			t.Errorf("features.%s = false, want true (default)", f)
		}
	}
}

// featureFlagValue reads one FeatureFlags field by name via reflection.
func featureFlagValue(f FeatureFlags, name string) bool {
	rv := reflect.ValueOf(f)
	if !rv.IsValid() {
		return false
	}
	fv := rv.FieldByName(name)
	if !fv.IsValid() {
		return false
	}
	return fv.Bool()
}
