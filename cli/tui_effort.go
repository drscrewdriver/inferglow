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

// effortStatus returns the display form of the current effort level
// ("" → "auto").
func (m *chatTUI) effortStatus() string {
	if m.effort == "" {
		return "auto"
	}
	return m.effort
}

// effortScale returns the scale for the current route. Falls back to the
// built-in defaults when no scales are configured, then to the "*" default.
func (m *chatTUI) effortScale() EffortScale {
	scales := m.effortScales
	if scales == nil {
		scales = defaultEffortScales
	}
	s, _ := resolveEffortScale(m.route.Provider, m.route.Model, scales)
	return s
}

// effortScaleForRoute returns the scale for the current route, tightened to
// the model profile's offered levels when the provider declares an
// EffortLevels map (LLM-provider-port P3: pi-ai thinkingLevelMap tightening).
// "auto" is always selectable on top. Levels the model does not offer are
// filtered out; levels the model offers but the scale does not declare are
// appended (with their wire value as the Params).
func (m *chatTUI) effortScaleForRoute() EffortScale {
	s := m.effortScale()
	mp := model.LookupModelProfile(m.route.Provider, m.route.Model)
	if mp.EffortLevels == nil {
		return s // no model-level map → show the full scale
	}
	tight := EffortScale{Match: s.Match}
	seen := map[string]bool{}
	for _, lv := range s.Levels {
		if lv.Name == "off" {
			// "off" = no injection, universally valid when reasoning is on;
			// keep it regardless of the model map (pi-ai always offers it).
			tight.Levels = append(tight.Levels, lv)
			seen["off"] = true
			continue
		}
		wire, declared := mp.EffortLevels[lv.Name]
		if declared && wire != nil {
			tight.Levels = append(tight.Levels, lv)
			seen[lv.Name] = true
		}
	}
	// Append profile-offered levels missing from the scale (e.g. xhigh/max on
	// anthropic whose default scale stops at high).
	for level, wire := range mp.EffortLevels {
		if level == "off" || seen[level] || wire == nil {
			continue
		}
		tight.Levels = append(tight.Levels, EffortLevel{
			Name:   level,
			Label:  level,
			Params: map[string]any{"reasoning_effort": level},
		})
	}
	return tight
}

// effortOptions builds the ModelRequest.Options injection for the current
// effort level against the current route's scale. "" and "auto" return nil
// (provider default, no injection); unknown levels also return nil.
func (m *chatTUI) effortOptions() map[string]any {
	if m.effort == "" || m.effort == "auto" {
		return nil
	}
	scale := m.effortScaleForRoute()
	for _, lv := range scale.Levels {
		if lv.Name == m.effort {
			return lv.Params
		}
	}
	return nil
}

// tuiHandleEffort handles /effort (RF-2):
//
//	/effort            → list the current model's effort scale + current
//	/effort status     → report current level and scale
//	/effort <level>    → set + persist (scale-dependent; "auto" = default)
func tuiHandleEffort(m *chatTUI, args string) (tea.Cmd, bool) {
	args = strings.TrimSpace(args)
	m.commitLine("")
	scale := m.effortScaleForRoute()
	names := effortLevelNames(scale)
	cur := m.effortStatus()

	switch {
	case args == "":
		m.commitLine(accent("Reasoning effort (" + m.route.Provider + "/" + m.route.Model + "):"))
		// "auto" (provider default) is always selectable, then the scale.
		m.commitLine(pickerLine("auto", cur, "provider 默认 · 不注入参数"))
		for _, name := range names {
			m.commitLine(pickerLine(name, cur, effortLevelLabel(scale, name)))
		}
		m.commitLine(dim("  Usage: /effort <level> | /effort status"))

	case args == "status":
		m.commitLine(accent("Reasoning effort: " + cur))
		m.commitLine(dim("  Scale: " + strings.Join(append([]string{"auto"}, names...), " | ")))
		if m.effort == "" || m.effort == "auto" {
			m.commitLine(dim("  Using the provider default (no effort params injected)."))
		} else if label := effortLevelLabel(scale, m.effort); label != "" {
			m.commitLine(dim("  " + label + " → " + paramsDesc(m.effortOptions())))
		} else {
			m.commitLine(dim("  Level not in this model's scale; nothing injected."))
		}

	case args == "auto":
		m.setEffort("")
		m.commitLine(successText("  ✓ 已恢复 provider 默认思考等级"))

	case containsString(names, args):
		m.setEffort(args)
		label := effortLevelLabel(scale, args)
		if label != "" {
			m.commitLine(successText("  ✓ 思考等级已设为 " + args + " · " + label))
		} else {
			m.commitLine(successText("  ✓ 思考等级已设为 " + args))
		}

	default:
		m.commitLine(errorText("  Unknown effort level for this model: " + args))
		m.commitLine(dim("  Available: auto | " + strings.Join(names, " | ")))
	}
	return nil, false
}

// setEffort assigns the effort level, persists it and marks the transcript
// dirty. It does not validate against the current scale (callers should use
// effortLevelValid first); the runtime degrades to no-injection for invalid
// levels.
func (m *chatTUI) setEffort(level string) {
	m.effort = level
	writeEffortPref(level)
	m.transcriptDirty = true
}

// pickerLine renders one selectable entry: "→" marker when current.
func pickerLine(name, current, label string) string {
	marker := "  "
	if name == current {
		marker = "→ "
	}
	line := dim(marker + name)
	if label != "" {
		line += dim("  · " + label)
	}
	return line
}

// paramsDesc renders an Options injection map for status display.
func paramsDesc(params map[string]any) string {
	if len(params) == 0 {
		return "no injection"
	}
	parts := make([]string, 0, len(params))
	for k, v := range params {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

// containsString reports whether s is in list.
func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
