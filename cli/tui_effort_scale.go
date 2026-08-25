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
	"sort"
)

// EffortLevel is one semantic reasoning-effort level on a model's scale.
// Name is the persisted/semantic identifier (e.g. "off", "low", "medium",
// "high", "max"); Params are injected into the model request Options when the
// level is active (nil = no injection). Label is a short human hint.
type EffortLevel struct {
	Name   string
	Label  string
	Params map[string]any
}

// EffortScale is the ordered set of effort levels for a provider or a
// specific provider/model. Match uses "provider/model", "provider", or "*"
// (exact provider/model wins over provider, which wins over the "*" default).
type EffortScale struct {
	Match  string
	Levels []EffortLevel
}

// ---- Built-in scales -------------------------------------------------------
//
// Every model family exposes a different semantic scale:
//   - DeepSeek: off / low / high / max (no "medium").
//   - OpenAI / Anthropic / generic OpenAI-compatible: off / low / medium / high.
//
// "off" injects nothing (provider default behaviour) because most APIs cannot
// disable reasoning explicitly; users can override per-model in config.

var (
	// defaultEffortScales are the fallback scales used when config provides no
	// override. Config-provided scales always take precedence.
	defaultEffortScales = []EffortScale{
		{
			Match: "deepseek",
			Levels: []EffortLevel{
				{Name: "off", Label: "关闭思考 · 不注入参数"},
				{Name: "low", Label: "轻量推理", Params: map[string]any{"reasoning_effort": "low"}},
				{Name: "high", Label: "深度推理", Params: map[string]any{"reasoning_effort": "high"}},
				{Name: "max", Label: "最强推理", Params: map[string]any{"reasoning_effort": "max"}},
			},
		},
		{
			Match: "openai",
			Levels: []EffortLevel{
				{Name: "off", Label: "关闭思考 · 不注入参数"},
				{Name: "low", Label: "轻量推理", Params: map[string]any{"reasoning_effort": "low"}},
				{Name: "medium", Label: "中等推理", Params: map[string]any{"reasoning_effort": "medium"}},
				{Name: "high", Label: "深度推理", Params: map[string]any{"reasoning_effort": "high"}},
			},
		},
		{
			Match: "*",
			Levels: []EffortLevel{
				{Name: "off", Label: "关闭思考 · 不注入参数"},
				{Name: "low", Label: "轻量推理", Params: map[string]any{"reasoning_effort": "low"}},
				{Name: "medium", Label: "中等推理", Params: map[string]any{"reasoning_effort": "medium"}},
				{Name: "high", Label: "深度推理", Params: map[string]any{"reasoning_effort": "high"}},
			},
		},
	}
)

// buildEffortScales merges config-provided scales (first, highest priority)
// with the built-in defaults. Level order within a config scale is sorted
// by name for determinism (JSON maps are unordered).
func buildEffortScales(cfgScales map[string]map[string]EffortScaleLevelCfg) []EffortScale {
	var out []EffortScale
	if len(cfgScales) > 0 {
		matches := make([]string, 0, len(cfgScales))
		for match := range cfgScales {
			matches = append(matches, match)
		}
		sort.Strings(matches)
		for _, match := range matches {
			cfg := cfgScales[match]
			names := make([]string, 0, len(cfg))
			for name := range cfg {
				names = append(names, name)
			}
			sort.Strings(names)
			scale := EffortScale{Match: match}
			for _, name := range names {
				levelCfg := cfg[name]
				scale.Levels = append(scale.Levels, EffortLevel{
					Name:   name,
					Label:  levelCfg.Label,
					Params: normalizeParams(levelCfg.Params),
				})
			}
			out = append(out, scale)
		}
	}
	return append(out, defaultEffortScales...)
}

// normalizeParams treats an empty param map as "no injection" (nil).
func normalizeParams(p map[string]any) map[string]any {
	if len(p) == 0 {
		return nil
	}
	return p
}

// resolveEffortScale finds the scale for a provider/model. Matching priority:
// exact "provider/model" → "provider" → "*". The bool is false only when no
// scale matches (cannot happen because "*" always exists).
func resolveEffortScale(provider, model string, scales []EffortScale) (EffortScale, bool) {
	pick := func(match func(string) bool) (EffortScale, bool) {
		for _, s := range scales {
			if match(s.Match) {
				return s, true
			}
		}
		return EffortScale{}, false
	}
	if s, ok := pick(func(m string) bool { return m == provider+"/"+model }); ok {
		return s, true
	}
	if s, ok := pick(func(m string) bool { return m == provider }); ok {
		return s, true
	}
	return pick(func(m string) bool { return m == "*" })
}

// effortLevelNames returns the ordered level names of a scale (excluding the
// synthetic "auto" pseudo-level, which is handled separately as "").
func effortLevelNames(scale EffortScale) []string {
	out := make([]string, 0, len(scale.Levels))
	for _, lv := range scale.Levels {
		out = append(out, lv.Name)
	}
	return out
}

// effortLevelLabel returns the label for a named level, or "" when absent.
func effortLevelLabel(scale EffortScale, name string) string {
	for _, lv := range scale.Levels {
		if lv.Name == name {
			return lv.Label
		}
	}
	return ""
}

// effortLevelValid reports whether level is a usable value for the given
// scale. "" and "auto" always mean provider-default (no injection).
func effortLevelValid(scale EffortScale, level string) bool {
	if level == "" || level == "auto" {
		return true
	}
	for _, lv := range scale.Levels {
		if lv.Name == level {
			return true
		}
	}
	return false
}
