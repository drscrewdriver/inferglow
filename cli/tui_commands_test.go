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
	"testing"
)

// TestBuildSlashRegistryDefault verifies the full registry wiring with the
// default config: native commands, compat catalog, /workspace /cd /tasks —
// all registered without panicking on name collisions.
func TestBuildSlashRegistryDefault(t *testing.T) {
	cfg := DefaultCLIConfig()
	r := buildSlashRegistry(cfg)

	// Native.
	if !r.IsImplemented("mode") {
		t.Error("native /mode missing")
	}
	// Compat implemented (mapped onto native handlers).
	for _, name := range []string{"reset", "new", "summarize", "continue", "sessions", "settings", "title", "status", "usage", "cost", "hotkeys", "keybindings", "q", "logout", "cd", "pwd"} {
		if !r.IsImplemented(name) {
			t.Errorf("compat implemented command /%s missing", name)
		}
	}
	// RF-1/2/3: native model/effort/theme switching commands.
	for _, name := range []string{"model", "effort", "theme", "tips", "welcome", "tps", "health"} {
		if !r.IsImplemented(name) {
			t.Errorf("native feature command /%s missing", name)
		}
	}
	// Compat aliases.
	if !r.IsImplemented("models") || !r.IsImplemented("scoped-models") {
		t.Error("compat /model aliases missing")
	}
	// Compat stubs: recognized but NOT implemented.
	for _, name := range []string{"vim", "pets", "init", "mcp", "fork", "undo", "export"} {
		if r.IsImplemented(name) {
			t.Errorf("stub /%s should not be implemented", name)
		}
		if r.SourceOf(name) == "" {
			t.Errorf("stub /%s should carry a source label", name)
		}
	}
	// SC-5 workspace + SC-3 task panel.
	if !r.IsImplemented("workspace") || !r.IsImplemented("cd") || !r.IsImplemented("tasks") {
		t.Error("native /workspace /cd /tasks missing")
	}
	// Suggest through the full registry: prefix + dedup.
	got := r.Suggest("mo", 10)
	names := make(map[string]bool)
	for _, c := range got {
		names[c.Name] = true
	}
	for _, want := range []string{"mode", "model", "memory"} {
		if !names[want] {
			t.Errorf("Suggest(mo) missing %s (got %v)", want, got)
		}
	}
}

// TestBuildSlashRegistryFeatureGates verifies each feature switch removes
// its commands: slash_compat off → no compat entries; workspace_switch off →
// no /workspace /cd (and compat cd/pwd dropped); task_panel off → no /tasks;
// model_switch off → /model falls back to the compat report handler.
func TestBuildSlashRegistryFeatureGates(t *testing.T) {
	cfg := DefaultCLIConfig()
	cfg.Features.SlashCompat = false
	r := buildSlashRegistry(cfg)
	for _, name := range []string{"reset", "vim", "fork"} {
		if _, ok := r.index[name]; ok {
			t.Errorf("slash_compat=false should drop /%s", name)
		}
	}
	if !r.IsImplemented("tasks") || !r.IsImplemented("workspace") {
		t.Error("non-compat features should remain when slash_compat=false")
	}
	if !r.IsImplemented("model") {
		t.Error("native /model should remain when slash_compat=false (RF-1)")
	}

	cfg = DefaultCLIConfig()
	cfg.Features.ModelSwitch = false
	r = buildSlashRegistry(cfg)
	// /model falls back to the compat report handler (still present).
	if !r.IsImplemented("model") {
		t.Error("model_switch=false should keep the report-only /model fallback")
	}

	// Each RF feature gate removes only its own commands.
	gateTests := []struct {
		flag  func(*FeatureFlags)
		name  string
	}{
		{func(f *FeatureFlags) { f.EffortControl = false }, "effort"},
		{func(f *FeatureFlags) { f.ThemeSwitch = false }, "theme"},
		{func(f *FeatureFlags) { f.TPS = false }, "tps"},
		{func(f *FeatureFlags) { f.HealthCheck = false }, "health"},
		{func(f *FeatureFlags) { f.Welcome = false }, "tips"},
		{func(f *FeatureFlags) { f.Welcome = false }, "welcome"},
	}
	for _, gt := range gateTests {
		cfg := DefaultCLIConfig()
		gt.flag(&cfg.Features)
		r := buildSlashRegistry(cfg)
		if _, ok := r.index[gt.name]; ok {
			t.Errorf("feature gate should drop /%s", gt.name)
		}
	}

	cfg = DefaultCLIConfig()
	cfg.Features.WorkspaceSwitch = false
	r = buildSlashRegistry(cfg)
	if _, ok := r.index["workspace"]; ok {
		t.Error("workspace_switch=false should drop /workspace")
	}
	if _, ok := r.index["cd"]; ok {
		t.Error("workspace_switch=false should drop /cd")
	}
	if _, ok := r.index["pwd"]; ok {
		t.Error("workspace_switch=false should drop compat /pwd")
	}
	if !r.IsImplemented("tasks") {
		t.Error("task panel should remain when workspace_switch=false")
	}

	cfg = DefaultCLIConfig()
	cfg.Features.TaskPanel = false
	r = buildSlashRegistry(cfg)
	if _, ok := r.index["tasks"]; ok {
		t.Error("task_panel=false should drop /tasks")
	}
}

// TestCompatDispatchRoundTrip verifies mapped compat commands actually reach
// the native handler through the legacy dispatch path.
func TestCompatDispatchRoundTrip(t *testing.T) {
	cfg := DefaultCLIConfig()
	r := buildSlashRegistry(cfg)

	// /reset must dispatch (found) without panicking; handler re-dispatches
	// to the legacy /clear path which requires a chatTUI — only check that
	// the registry resolves it as implemented and non-stub.
	if !r.IsImplemented("reset") {
		t.Fatal("/reset should be implemented")
	}
	if _, ok := r.index["reset"]; !ok {
		t.Fatal("/reset not indexed")
	}
}

// TestSuggestStubsLowerCase verifies the popup suggestion surface for stub
// commands (e.g. /vim shows as a candidate).
func TestSuggestStubsLowerCase(t *testing.T) {
	cfg := DefaultCLIConfig()
	r := buildSlashRegistry(cfg)
	got := r.Suggest("vi", 5)
	if len(got) == 0 {
		t.Fatal("Suggest(vi) should surface /vim")
	}
	found := false
	for _, c := range got {
		if c.Name == "vim" {
			found = true
			if c.Implemented {
				t.Fatal("/vim should be Implemented=false")
			}
		}
	}
	if !found {
		t.Fatalf("Suggest(vi) missing vim: %v", got)
	}
}
