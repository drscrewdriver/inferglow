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

	"github.com/inferglow/model"
)

// ModelRoute is a complete model route (provider + model atomic pair).
type ModelRoute struct {
	Provider string
	Model    string
	Endpoint string
	APIKey   string
}

// ModelPref is the persisted route preference (~/.inferglow/model.json).
type ModelPref struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// EffortPref is the persisted reasoning-effort preference
// (~/.inferglow/effort.json). Effort "" means provider default.
type EffortPref struct {
	Effort string `json:"effort"`
}

const (
	modelPrefFile    = "model.json"
	effortPrefFile   = "effort.json"
	modelRecentsFile = "model_recents.json"
	defaultProvider  = "deepseek"
	defaultModel     = "deepseek-chat"
	modelRecentsMax  = 5
)

// resolveModelRoute resolves the effective model route (atomic: full pairs
// override, half pairs are ignored). Mirrors dsh-tui modelRoute.ts (issue
// #67) atomic semantics. Priority:
//  1. persisted ModelPref (complete pair only) — the /model runtime choice
//     survives restarts (plan smoke test: "重启 → model.json 生效");
//     endpoint/api_key are merged from config providers.list, the matching
//     single-route llm, or the static DEFAULT_SETTINGS directory;
//  2. providers.active → its List entry with a model;
//  3. first complete entry in providers.list;
//  4. single-route llm (model present);
//  5. default deepseek/deepseek-chat.
func resolveModelRoute(cfg CLIConfig, pref *ModelPref) ModelRoute {
	if pref != nil && pref.Provider != "" && pref.Model != "" {
		r := ModelRoute{Provider: pref.Provider, Model: pref.Model}
		if p, ok := cfg.Providers.List[pref.Provider]; ok {
			r.Endpoint = p.Endpoint
			r.APIKey = p.APIKey
		} else if cfg.LLM.Provider == pref.Provider && cfg.LLM.Model != "" {
			r.Endpoint = cfg.LLM.Endpoint
			r.APIKey = cfg.LLM.APIKey
		} else if ds, ok := model.DEFAULT_SETTINGS[pref.Provider]; ok {
			if b, ok := ds["base_url"].(string); ok {
				r.Endpoint = b
			}
		}
		return r
	}
	if cfg.Providers.Active != "" {
		if p, ok := cfg.Providers.List[cfg.Providers.Active]; ok && p.Model != "" {
			return ModelRoute{Provider: cfg.Providers.Active, Model: p.Model, Endpoint: p.Endpoint, APIKey: p.APIKey}
		}
	}
	if len(cfg.Providers.List) > 0 {
		// No active (or active without a model) → first complete entry.
		for key, p := range cfg.Providers.List {
			if p.Model != "" {
				return ModelRoute{Provider: key, Model: p.Model, Endpoint: p.Endpoint, APIKey: p.APIKey}
			}
		}
	}
	if cfg.LLM.Model != "" {
		prov := cfg.LLM.Provider
		if prov == "" {
			prov = "openai"
		}
		return ModelRoute{Provider: prov, Model: cfg.LLM.Model, Endpoint: cfg.LLM.Endpoint, APIKey: cfg.LLM.APIKey}
	}
	return ModelRoute{Provider: defaultProvider, Model: defaultModel}
}

// routeConfig reconstructs a single-route CLIConfig from a ModelRoute so
// buildModelRequester can construct a requester for it.
func (r ModelRoute) routeConfig() CLIConfig {
	return CLIConfig{LLM: LLMConfig{Endpoint: r.Endpoint, Model: r.Model, APIKey: r.APIKey, Provider: r.Provider}}
}

// prefsDir returns the ~/.inferglow data dir (fallback: current dir).
func prefsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".inferglow")
}

// readModelPref loads the persisted route preference. Corrupt/missing files
// return nil (non-fatal).
func readModelPref() *ModelPref {
	return readModelPrefFrom(filepath.Join(prefsDir(), modelPrefFile))
}

func readModelPrefFrom(path string) *ModelPref {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var p ModelPref
	if err := json.Unmarshal(data, &p); err != nil {
		return nil
	}
	if p.Provider == "" && p.Model == "" {
		return nil
	}
	return &p
}

// writeModelPref persists the route preference. Failures are silent.
func writeModelPref(p ModelPref) {
	writeModelPrefTo(filepath.Join(prefsDir(), modelPrefFile), p)
}

func writeModelPrefTo(path string, p ModelPref) {
	writePrefJSON(path, p)
}

// readEffortPref loads the persisted effort level ("" when unset/corrupt).
func readEffortPref() string {
	return readEffortPrefFrom(filepath.Join(prefsDir(), effortPrefFile))
}

func readEffortPrefFrom(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var e EffortPref
	if err := json.Unmarshal(data, &e); err != nil {
		return ""
	}
	return e.Effort
}

// writeEffortPref persists the effort level. Failures are silent.
func writeEffortPref(e string) {
	writeEffortPrefTo(filepath.Join(prefsDir(), effortPrefFile), e)
}

func writeEffortPrefTo(path, e string) {
	writePrefJSON(path, EffortPref{Effort: e})
}

// readModelRecents loads the recently used model routes (newest first).
func readModelRecents() []ModelPref {
	return readModelRecentsFrom(filepath.Join(prefsDir(), modelRecentsFile))
}

func readModelRecentsFrom(path string) []ModelPref {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var recents []ModelPref
	if err := json.Unmarshal(data, &recents); err != nil {
		return nil
	}
	return recents
}

// pushModelRecent records a route as recently used: dedupes, newest first,
// capped at modelRecentsMax. Failures are silent.
func pushModelRecent(p ModelPref) {
	pushModelRecentTo(filepath.Join(prefsDir(), modelRecentsFile), p)
}

func pushModelRecentTo(path string, p ModelPref) {
	if p.Provider == "" || p.Model == "" {
		return
	}
	recents := readModelRecentsFrom(path)
	out := make([]ModelPref, 0, modelRecentsMax+1)
	out = append(out, p)
	for _, r := range recents {
		if r.Provider == p.Provider && r.Model == p.Model {
			continue
		}
		out = append(out, r)
		if len(out) >= modelRecentsMax {
			break
		}
	}
	writePrefJSON(path, out)
}

// writePrefJSON marshals v and writes it under path, creating the parent
// directory as needed. All failures are silent (best-effort persistence).
func writePrefJSON(path string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
	}
	_ = os.WriteFile(path, data, 0o644)
}
