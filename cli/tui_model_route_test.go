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
	"os"
	"path/filepath"
	"testing"
)

func TestResolveModelRoute_ProvidersActive(t *testing.T) {
	cfg := CLIConfig{
		Providers: ProvidersConfig{
			Active: "deepseek",
			List: map[string]LLMConfig{
				"deepseek": {Endpoint: "https://api.deepseek.com/v1", Model: "deepseek-chat", Provider: "deepseek"},
				"openai":   {Endpoint: "https://api.openai.com/v1", Model: "gpt-4", Provider: "openai"},
			},
		},
	}
	r := resolveModelRoute(cfg, nil)
	if r.Provider != "deepseek" || r.Model != "deepseek-chat" {
		t.Fatalf("route = %+v, want deepseek/deepseek-chat", r)
	}
}

func TestResolveModelRoute_ActiveWithoutModelFallsToFirstComplete(t *testing.T) {
	cfg := CLIConfig{
		Providers: ProvidersConfig{
			Active: "half",
			List: map[string]LLMConfig{
				"half":  {Endpoint: "https://half.example/v1"}, // no model
				"openai": {Endpoint: "https://api.openai.com/v1", Model: "gpt-4", Provider: "openai"},
			},
		},
	}
	r := resolveModelRoute(cfg, nil)
	if r.Provider != "openai" || r.Model != "gpt-4" {
		t.Fatalf("route = %+v, want openai/gpt-4", r)
	}
}

func TestResolveModelRoute_SingleLLM(t *testing.T) {
	cfg := CLIConfig{LLM: LLMConfig{Endpoint: "https://x/v1", Model: "qwen-max", Provider: "qwen"}}
	r := resolveModelRoute(cfg, nil)
	if r.Provider != "qwen" || r.Model != "qwen-max" {
		t.Fatalf("route = %+v, want qwen/qwen-max", r)
	}
	// Empty provider falls back to "openai".
	cfg2 := CLIConfig{LLM: LLMConfig{Endpoint: "https://x/v1", Model: "gpt-4"}}
	r2 := resolveModelRoute(cfg2, nil)
	if r2.Provider != "openai" || r2.Model != "gpt-4" {
		t.Fatalf("route = %+v, want openai/gpt-4", r2)
	}
}

func TestResolveModelRoute_PrefFallback(t *testing.T) {
	// Config has no usable route → pref wins.
	cfg := CLIConfig{}
	pref := &ModelPref{Provider: "kimi", Model: "moonshot-v1-8k"}
	r := resolveModelRoute(cfg, pref)
	if r.Provider != "kimi" || r.Model != "moonshot-v1-8k" {
		t.Fatalf("route = %+v, want kimi/moonshot-v1-8k", r)
	}
	// Half pref (missing model) is ignored.
	r2 := resolveModelRoute(cfg, &ModelPref{Provider: "kimi"})
	if r2.Provider != defaultProvider || r2.Model != defaultModel {
		t.Fatalf("half pref should be ignored: %+v", r2)
	}
	r3 := resolveModelRoute(cfg, &ModelPref{Model: "moonshot-v1-8k"})
	if r3.Provider != defaultProvider || r3.Model != defaultModel {
		t.Fatalf("half pref should be ignored: %+v", r3)
	}
}

func TestResolveModelRoute_PrefWinsOverConfig(t *testing.T) {
	// The /model runtime choice (persisted pref) wins over the config so a
	// restart keeps the switch (plan smoke test: "model.json 生效").
	cfg := CLIConfig{
		Providers: ProvidersConfig{
			Active: "deepseek",
			List: map[string]LLMConfig{
				"deepseek": {Endpoint: "https://api.deepseek.com/v1", Model: "deepseek-chat"},
				"openai":   {Endpoint: "https://api.openai.com/v1", Model: "gpt-4"},
			},
		},
	}
	r := resolveModelRoute(cfg, &ModelPref{Provider: "openai", Model: "gpt-4"})
	if r.Provider != "openai" || r.Model != "gpt-4" {
		t.Fatalf("pref should win: %+v", r)
	}
	// Endpoint merged from providers.list.
	if r.Endpoint != "https://api.openai.com/v1" {
		t.Fatalf("endpoint should merge from config: %q", r.Endpoint)
	}
	// Pref provider not in config → endpoint from static directory.
	r2 := resolveModelRoute(cfg, &ModelPref{Provider: "glm", Model: "glm-4"})
	if r2.Provider != "glm" || r2.Model != "glm-4" {
		t.Fatalf("pref outside config should still win: %+v", r2)
	}
	if r2.Endpoint != "https://open.bigmodel.cn/api/paas/v4" {
		t.Fatalf("endpoint should merge from DEFAULT_SETTINGS: %q", r2.Endpoint)
	}
}

func TestResolveModelRoute_Default(t *testing.T) {
	r := resolveModelRoute(CLIConfig{}, nil)
	if r.Provider != defaultProvider || r.Model != defaultModel {
		t.Fatalf("route = %+v, want %s/%s", r, defaultProvider, defaultModel)
	}
}

func TestRouteConfigRoundTrip(t *testing.T) {
	r := ModelRoute{Provider: "glm", Model: "glm-4", Endpoint: "https://open.bigmodel.cn/api/paas/v4", APIKey: "sk-x"}
	cfg := r.routeConfig()
	if cfg.LLM.Provider != "glm" || cfg.LLM.Model != "glm-4" || cfg.LLM.Endpoint != "https://open.bigmodel.cn/api/paas/v4" || cfg.LLM.APIKey != "sk-x" {
		t.Fatalf("routeConfig = %+v", cfg.LLM)
	}
}

func TestModelPrefPersistRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), modelPrefFile)
	writeModelPrefTo(path, ModelPref{Provider: "openai", Model: "gpt-4"})
	p := readModelPrefFrom(path)
	if p == nil || p.Provider != "openai" || p.Model != "gpt-4" {
		t.Fatalf("read back = %+v, want openai/gpt-4", p)
	}
}

func TestModelPrefCorruptReturnsNil(t *testing.T) {
	path := filepath.Join(t.TempDir(), modelPrefFile)
	if err := os.WriteFile(path, []byte("{corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := readModelPrefFrom(path); p != nil {
		t.Fatalf("corrupt pref should return nil, got %+v", p)
	}
	// Empty object also yields nil.
	path2 := filepath.Join(t.TempDir(), modelPrefFile)
	if err := os.WriteFile(path2, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := readModelPrefFrom(path2); p != nil {
		t.Fatalf("empty pref should return nil, got %+v", p)
	}
	// Missing file → nil.
	if p := readModelPrefFrom(filepath.Join(t.TempDir(), "nope.json")); p != nil {
		t.Fatalf("missing pref should return nil, got %+v", p)
	}
}

func TestEffortPrefPersistRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), effortPrefFile)
	writeEffortPrefTo(path, "high")
	if e := readEffortPrefFrom(path); e != "high" {
		t.Fatalf("effort = %q, want high", e)
	}
	// Corrupt → "".
	if err := os.WriteFile(path, []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if e := readEffortPrefFrom(path); e != "" {
		t.Fatalf("corrupt effort should be \"\", got %q", e)
	}
	// Missing → "".
	if e := readEffortPrefFrom(filepath.Join(t.TempDir(), "nope.json")); e != "" {
		t.Fatalf("missing effort should be \"\", got %q", e)
	}
}

func TestModelRecentsPushDedupeCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), modelRecentsFile)
	for i := 0; i < 8; i++ {
		pushModelRecentTo(path, ModelPref{Provider: "p", Model: "m" + string(rune('0'+i%3))})
	}
	recents := readModelRecentsFrom(path)
	if len(recents) > modelRecentsMax {
		t.Fatalf("recents length %d exceeds max %d", len(recents), modelRecentsMax)
	}
	if recents[0].Model != "m1" { // last pushed: i=7 → 7%3=1
		t.Fatalf("newest first: %+v", recents)
	}
	// Dedupe: pushing the same pair again moves it to front without duplicates.
	pushModelRecentTo(path, ModelPref{Provider: "p", Model: "m1"})
	recents2 := readModelRecentsFrom(path)
	count := 0
	for _, r := range recents2 {
		if r.Model == "m1" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("recents should dedupe: %+v", recents2)
	}
	if recents2[0].Model != "m1" {
		t.Fatalf("pushed pair should move to front: %+v", recents2)
	}
}

func TestPushModelRecentIgnoresHalfPair(t *testing.T) {
	path := filepath.Join(t.TempDir(), modelRecentsFile)
	pushModelRecentTo(path, ModelPref{Provider: "p"}) // no model → ignored
	if recents := readModelRecentsFrom(path); len(recents) != 0 {
		t.Fatalf("half pair should not be recorded: %+v", recents)
	}
}
