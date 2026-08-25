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
	"reflect"
	"testing"
)

func TestResolveEffortScalePriority(t *testing.T) {
	scales := buildEffortScales(nil)
	cases := []struct {
		provider, model string
		wantMatch       string
	}{
		{"deepseek", "deepseek-chat", "deepseek"},
		{"deepseek", "anything", "deepseek"},
		{"openai", "gpt-5", "openai"},
		{"anthropic", "claude-x", "*"},
		{"unknown-provider", "m", "*"},
	}
	for _, c := range cases {
		s, ok := resolveEffortScale(c.provider, c.model, scales)
		if !ok {
			t.Fatalf("%s/%s: no scale resolved", c.provider, c.model)
		}
		if s.Match != c.wantMatch {
			t.Errorf("%s/%s: match = %q, want %q", c.provider, c.model, s.Match, c.wantMatch)
		}
	}
}

func TestDeepSeekScaleLevels(t *testing.T) {
	scales := buildEffortScales(nil)
	s, _ := resolveEffortScale("deepseek", "deepseek-chat", scales)
	names := effortLevelNames(s)
	want := []string{"off", "low", "high", "max"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("deepseek levels = %v, want %v", names, want)
	}
	// "medium" is NOT valid on deepseek.
	if effortLevelValid(s, "medium") {
		t.Fatal("medium should not be valid on deepseek scale")
	}
	for _, lv := range want {
		if !effortLevelValid(s, lv) {
			t.Errorf("level %q should be valid on deepseek", lv)
		}
	}
	// "" and "auto" always valid.
	if !effortLevelValid(s, "") || !effortLevelValid(s, "auto") {
		t.Fatal("empty/auto must always be valid")
	}
}

func TestEffortLevelParams(t *testing.T) {
	scales := buildEffortScales(nil)
	// deepseek max → reasoning_effort=max
	ds, _ := resolveEffortScale("deepseek", "deepseek-chat", scales)
	if p := paramsForLevel(ds, "max"); p == nil || p["reasoning_effort"] != "max" {
		t.Fatalf("deepseek max params = %v", p)
	}
	if p := paramsForLevel(ds, "off"); p != nil {
		t.Fatalf("deepseek off should inject nothing, got %v", p)
	}
	// openai medium → reasoning_effort=medium
	oa, _ := resolveEffortScale("openai", "gpt-5", scales)
	if p := paramsForLevel(oa, "medium"); p == nil || p["reasoning_effort"] != "medium" {
		t.Fatalf("openai medium params = %v", p)
	}
}

func TestEffortOptionsScaleAware(t *testing.T) {
	scales := buildEffortScales(nil)
	// deepseek route: max works, medium does not.
	ds := &chatTUI{effortScales: scales, route: ModelRoute{Provider: "deepseek", Model: "deepseek-chat"}}
	ds.effort = "max"
	if opts := ds.effortOptions(); opts == nil || opts["reasoning_effort"] != "max" {
		t.Fatalf("deepseek max options = %v", opts)
	}
	ds.effort = "medium"
	if opts := ds.effortOptions(); opts != nil {
		t.Fatalf("deepseek medium should inject nothing, got %v", opts)
	}
	// openai route: medium works.
	oa := &chatTUI{effortScales: scales, route: ModelRoute{Provider: "openai", Model: "gpt-5"}}
	oa.effort = "medium"
	if opts := oa.effortOptions(); opts == nil || opts["reasoning_effort"] != "medium" {
		t.Fatalf("openai medium options = %v", opts)
	}
	// auto/"" → nil on any scale.
	for _, lv := range []string{"", "auto"} {
		oa.effort = lv
		if opts := oa.effortOptions(); opts != nil {
			t.Fatalf("effort %q should inject nothing, got %v", lv, opts)
		}
	}
}

func TestBuildEffortScalesConfigOverride(t *testing.T) {
	cfg := map[string]map[string]EffortScaleLevelCfg{
		"mybox": {
			"light":  {Label: "L", Params: map[string]any{"reasoning_effort": "light"}},
			"turbo":  {Params: map[string]any{"thinking": "on"}},
			"silent": {},
		},
	}
	scales := buildEffortScales(cfg)
	// Exact provider/model override for mybox/deepseek-custom.
	scales = append(scales, EffortScale{Match: "mybox/deepseek-custom", Levels: []EffortLevel{
		{Name: "on", Params: map[string]any{"reasoning_effort": "high"}},
	}})

	// provider-level config scale wins over built-in "*".
	s, _ := resolveEffortScale("mybox", "m1", scales)
	if s.Match != "mybox" {
		t.Fatalf("mybox match = %q, want mybox", s.Match)
	}
	names := effortLevelNames(s)
	want := []string{"light", "silent", "turbo"} // sorted
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("mybox levels = %v, want %v", names, want)
	}
	if p := paramsForLevel(s, "silent"); p != nil {
		t.Fatalf("silent (empty params) should inject nothing, got %v", p)
	}
	if p := paramsForLevel(s, "turbo"); p == nil || p["thinking"] != "on" {
		t.Fatalf("turbo params = %v", p)
	}

	// exact provider/model beats provider.
	s2, _ := resolveEffortScale("mybox", "deepseek-custom", scales)
	if s2.Match != "mybox/deepseek-custom" {
		t.Fatalf("exact match = %q, want mybox/deepseek-custom", s2.Match)
	}
}

func TestEffortScaleNilFallsBackToDefaults(t *testing.T) {
	// A chatTUI without effortScales (tests, or unconfigured) still works.
	m := &chatTUI{route: ModelRoute{Provider: "deepseek", Model: "deepseek-chat"}}
	m.effort = "max"
	if opts := m.effortOptions(); opts == nil || opts["reasoning_effort"] != "max" {
		t.Fatalf("nil-scale deepseek max options = %v", opts)
	}
}

// paramsForLevel returns the Params for a named level (helper for tests).
func paramsForLevel(scale EffortScale, name string) map[string]any {
	for _, lv := range scale.Levels {
		if lv.Name == name {
			return lv.Params
		}
	}
	return nil
}

// TestEffortScaleForRouteTightened verifies the pi-ai thinkingLevelMap
// tightening: /effort on a deepseek route shows only off/low/high/max, on an
// anthropic route only off/xhigh/max, and a route without a profile keeps the
// full scale.
func TestEffortScaleForRouteTightened(t *testing.T) {
	scales := buildEffortScales(nil)

	// deepseek-v4-pro: profile offers off/low/high/max → medium filtered out.
	ds := &chatTUI{effortScales: scales, route: ModelRoute{Provider: "deepseek", Model: "deepseek-v4-pro"}}
	names := effortLevelNames(ds.effortScaleForRoute())
	want := []string{"off", "low", "high", "max"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("deepseek-v4-pro tightened levels = %v, want %v", names, want)
	}

	// anthropic claude-opus-4-7: profile offers off/xhigh/max.
	an := &chatTUI{effortScales: scales, route: ModelRoute{Provider: "anthropic", Model: "claude-opus-4-7"}}
	names = effortLevelNames(an.effortScaleForRoute())
	want = []string{"off", "xhigh", "max"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("anthropic tightened levels = %v, want %v", names, want)
	}

	// Unknown model (no profile): full default scale (off/low/medium/high).
	un := &chatTUI{effortScales: scales, route: ModelRoute{Provider: "openai", Model: "unknown-model"}}
	names = effortLevelNames(un.effortScaleForRoute())
	want = []string{"off", "low", "medium", "high"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("unknown model levels = %v, want %v (full default)", names, want)
	}
}

// TestEffortOptionsTightenedDrop verifies that a level the model profile does
// not offer is dropped from the injected options (e.g. medium on deepseek).
func TestEffortOptionsTightenedDrop(t *testing.T) {
	scales := buildEffortScales(nil)
	ds := &chatTUI{effortScales: scales, route: ModelRoute{Provider: "deepseek", Model: "deepseek-v4-pro"}}
	ds.effort = "medium" // not in deepseek profile
	if opts := ds.effortOptions(); opts != nil {
		t.Fatalf("deepseek medium should inject nothing (tightened), got %v", opts)
	}
	ds.effort = "max"
	if opts := ds.effortOptions(); opts == nil || opts["reasoning_effort"] != "max" {
		t.Fatalf("deepseek max options = %v, want reasoning_effort=max", opts)
	}
}
