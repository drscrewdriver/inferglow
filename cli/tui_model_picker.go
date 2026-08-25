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
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/inferglow/model"
)

// modelPicker is the interactive two-level model selector (RF-1):
// level 0 = provider list, level 1 = model list for the chosen provider.
type modelPicker struct {
	active    bool
	level     int        // 0=provider, 1=model
	provider  string     // chosen provider at level 1
	providers []string   // level 0 candidates
	models    []string   // level 1 candidates
	selected  int
	recents   []ModelPref // recently used routes
}

// Enter opens the provider level of the picker.
func (p *modelPicker) Enter(providers []string, recents []ModelPref) {
	p.active = true
	p.level = 0
	p.providers = providers
	p.recents = recents
	p.models = nil
	p.selected = 0
}

// NextLevel descends into the model list of the given provider.
func (p *modelPicker) NextLevel(provider string, models []string) {
	p.active = true
	p.level = 1
	p.provider = provider
	p.models = models
	p.selected = 0
}

// Move shifts the selection (wrapping).
func (p *modelPicker) Move(delta int) {
	list := p.providers
	if p.level == 1 {
		list = p.models
	}
	if len(list) == 0 {
		return
	}
	p.selected = (p.selected + delta + len(list)) % len(list)
}

// Cancel closes the picker.
func (p *modelPicker) Cancel() {
	p.active = false
	p.level = 0
	p.selected = 0
}

// Selected returns the currently highlighted candidate.
func (p *modelPicker) Selected() string {
	if p.level == 0 {
		if p.selected < len(p.providers) {
			return p.providers[p.selected]
		}
	}
	if p.selected < len(p.models) {
		return p.models[p.selected]
	}
	return ""
}

// Render renders the picker popup (list + hint footer). Empty when inactive.
func (p *modelPicker) Render(width int) string {
	if !p.active {
		return ""
	}
	list := p.providers
	title := "Provider"
	if p.level == 1 {
		list = p.models
		title = "Model (" + p.provider + ")"
	}
	if width > 60 {
		width = 60
	}
	const maxItems = 15
	start := 0
	if len(list) > maxItems {
		start = max(p.selected-maxItems+1, 0)
	}
	shown := list[start:]
	if len(shown) > maxItems {
		shown = shown[:maxItems]
	}
	var sb strings.Builder
	sb.WriteString(dim("─── " + title + " ─────────────────────────"))
	for i, item := range shown {
		marker := "  "
		style := dim
		if start+i == p.selected {
			marker = "→ "
			style = accent
		}
		sb.WriteString("\n" + marker + style(compactMiddle(item, width-4)))
	}
	sb.WriteString("\n" + dim("  [↑/↓] select · [Enter] confirm · [Esc] cancel"))
	return sb.String()
}

// modelProviders lists the provider candidates: config providers.list keys
// first, then the static DEFAULT_SETTINGS directory (deduplicated, sorted
// for determinism).
func (m *chatTUI) modelProviders() []string {
	seen := map[string]bool{}
	var out []string
	for key := range m.cfg.Providers.List {
		if !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	var static []string
	for key := range model.DEFAULT_SETTINGS {
		if !seen[key] {
			seen[key] = true
			static = append(static, key)
		}
	}
	sort.Strings(static)
	out = append(out, static...)
	return out
}

// knownProvider reports whether the provider is configured or in the static
// directory.
func (m *chatTUI) knownProvider(provider string) bool {
	if _, ok := m.cfg.Providers.List[provider]; ok {
		return true
	}
	_, ok := model.DEFAULT_SETTINGS[provider]
	return ok
}

// modelCandidates lists the model candidates for a provider:
// DEFAULT_SETTINGS[provider]["model"] + the configured provider model,
// deduplicated.
func (m *chatTUI) modelCandidates(provider string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if ds, ok := model.DEFAULT_SETTINGS[provider]; ok {
		if mm, ok := ds["model"].(string); ok {
			add(mm)
		}
	}
	if p, ok := m.cfg.Providers.List[provider]; ok {
		add(p.Model)
	}
	return out
}

// tuiHandleModel handles the native /model command (RF-1):
//
//	/model                 → open the interactive picker (provider level)
//	/model <provider>      → open the model level for that provider
//	/model <provider> <model> → switch immediately (validated + persisted)
//	/model status          → report the effective route
//	/model recents         → list recently used routes
func tuiHandleModel(m *chatTUI, args string) (tea.Cmd, bool) {
	args = strings.TrimSpace(args)
	fields := strings.Fields(args)
	m.commitLine("")
	switch {
	case args == "":
		m.picker.Enter(m.modelProviders(), readModelRecents())
		if len(m.picker.providers) == 0 {
			m.picker.Cancel()
			m.commitLine(warnText("  No providers available (config providers.list is empty)."))
			return nil, false
		}
		m.commitLine(dim("  Model picker: [↑/↓] select · [Enter] confirm · [Esc] cancel"))

	case fields[0] == "status":
		m.commitLine(accent("Model route:"))
		m.commitLine(dim("  Provider: ") + footerInfo(m.route.Provider))
		m.commitLine(dim("  Model:    ") + footerInfo(m.route.Model))
		if m.route.Endpoint != "" {
			m.commitLine(dim("  Endpoint: ") + footerInfo(m.route.Endpoint))
		}
		m.commitLine(dim("  Effort:   ") + footerInfo(m.effortStatus()))

	case fields[0] == "recents":
		recents := readModelRecents()
		if len(recents) == 0 {
			m.commitLine(dim("  No recent model routes yet."))
			return nil, false
		}
		m.commitLine(accent("Recent model routes:"))
		for i, r := range recents {
			m.commitLine(dim(fmt.Sprintf("  %d. %s/%s", i+1, r.Provider, r.Model)))
		}

	case len(fields) == 1:
		if !m.knownProvider(fields[0]) {
			m.commitLine(errorText(fmt.Sprintf("  Unknown provider: %s", fields[0])))
			m.commitLine(dim("  Use /model to browse providers."))
			return nil, false
		}
		models := m.modelCandidates(fields[0])
		if len(models) == 0 {
			m.commitLine(errorText(fmt.Sprintf("  No known models for provider %s.", fields[0])))
			m.commitLine(dim("  Usage: /model " + fields[0] + " <model>"))
			return nil, false
		}
		m.picker.NextLevel(fields[0], models)
		m.commitLine(dim("  Model picker (" + fields[0] + "): [↑/↓] select · [Enter] confirm · [Esc] cancel"))

	default: // len(fields) >= 2 → direct switch
		m.modelSetRoute(fields[0], fields[1])
	}
	return nil, false
}

// modelSetRoute validates the route, builds a requester for it, updates the
// runtime route and persists the preference (RF-1).
func (m *chatTUI) modelSetRoute(provider, model string) {
	route, err := m.buildRouteFor(provider, model)
	if err != nil {
		m.commitLine(errorText("  ✗ " + err.Error()))
		return
	}
	req, err := buildModelRequester(route.routeConfig())
	if err != nil {
		m.commitLine(errorText("  ✗ 无法构造请求器: " + err.Error()))
		return
	}
	m.route = route
	m.requester = req
	m.modelLabel = provider + "/" + model
	writeModelPref(ModelPref{Provider: provider, Model: model})
	pushModelRecent(ModelPref{Provider: provider, Model: model})
	// RF-2: re-validate the effort level against the new model's scale
	// (e.g. "medium" exists on openai but not on deepseek, which uses
	// off/low/high/max). Invalid levels reset to provider default.
	if m.effort != "" && m.effort != "auto" {
		scale, _ := resolveEffortScale(provider, model, m.effortScales)
		if !effortLevelValid(scale, m.effort) {
			m.commitLine(warnText("  effort '" + m.effort + "' 不适用于 " + provider + "/" + model + "，已恢复默认"))
			m.effort = ""
		}
	}
	m.transcriptDirty = true
	m.commitLine(successText(fmt.Sprintf("  ✓ 已切换到 %s/%s", provider, model)))
	m.commitLine(dim("  Next turn uses the new route (persisted to ~/.inferglow/model.json)."))
}

// buildRouteFor resolves the endpoint/api-key for a provider/model pair:
// config providers.list first, then the static DEFAULT_SETTINGS directory.
func (m *chatTUI) buildRouteFor(provider, modelName string) (ModelRoute, error) {
	endpoint := ""
	apiKey := ""
	if p, ok := m.cfg.Providers.List[provider]; ok {
		endpoint = p.Endpoint
		apiKey = p.APIKey
	}
	if endpoint == "" {
		if ds, ok := model.DEFAULT_SETTINGS[provider]; ok {
			if b, ok := ds["base_url"].(string); ok {
				endpoint = b
			}
			if k, ok := ds["api_key"].(string); ok {
				apiKey = k
			}
		}
	}
	if endpoint == "" {
		return ModelRoute{}, fmt.Errorf("provider %q has no endpoint configured (add it to config providers.list)", provider)
	}
	return ModelRoute{Provider: provider, Model: modelName, Endpoint: endpoint, APIKey: apiKey}, nil
}

// handlePickerKey processes keys while the model picker is active. Returns
// true when the key was consumed.
func (m *chatTUI) handlePickerKey(key string) bool {
	switch key {
	case "up":
		m.picker.Move(-1)
	case "down":
		m.picker.Move(+1)
	case "enter", "tab":
		if m.picker.level == 0 {
			prov := m.picker.Selected()
			if prov == "" {
				break
			}
			models := m.modelCandidates(prov)
			if len(models) == 0 {
				m.picker.Cancel()
				m.commitLine("")
				m.commitLine(warnText("  No known models for " + prov + "; use /model " + prov + " <model>"))
				break
			}
			m.picker.NextLevel(prov, models)
		} else {
			mdl := m.picker.Selected()
			prov := m.picker.provider
			m.picker.Cancel()
			if mdl != "" {
				m.modelSetRoute(prov, mdl)
			}
		}
	case "esc":
		m.picker.Cancel()
		m.commitLine("")
		m.commitLine(dim("  Model picker cancelled."))
	default:
		return false
	}
	m.transcriptDirty = true
	return true
}
