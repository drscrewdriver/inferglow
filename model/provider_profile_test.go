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

package model

import "testing"

func TestLookupDeepSeekProfile(t *testing.T) {
	p := LookupProviderProfile("deepseek")
	if p.EffortFormat != EffortDeepSeek {
		t.Fatalf("deepseek format = %q, want %q", p.EffortFormat, EffortDeepSeek)
	}
	mp := LookupModelProfile("deepseek", "deepseek-v4-pro")
	if mp.EffortFormat != EffortDeepSeek {
		t.Fatalf("deepseek-v4-pro format = %q, want deepseek", mp.EffortFormat)
	}
	if mp.DefaultEffort != "high" {
		t.Fatalf("deepseek-v4-pro default = %q, want high", mp.DefaultEffort)
	}
	if v, ok := mp.EffortLevels["max"]; !ok || v != "max" {
		t.Fatalf("deepseek-v4-pro max level = %v", v)
	}
	if _, ok := mp.EffortLevels["medium"]; ok {
		t.Fatal("deepseek should not offer medium")
	}
	// unknown model falls back to provider format, no level map
	unk := LookupModelProfile("deepseek", "unknown-model")
	if unk.EffortFormat != EffortDeepSeek {
		t.Fatalf("unknown model format = %q, want provider default", unk.EffortFormat)
	}
	if unk.EffortLevels != nil {
		t.Fatalf("unknown model levels = %v, want nil", unk.EffortLevels)
	}
}

func TestLookupOpenRouterProfile(t *testing.T) {
	mp := LookupModelProfile("openrouter", "anthropic/claude-opus-4-7")
	if mp.EffortFormat != EffortOpenRouter {
		t.Fatalf("openrouter claude format = %q, want openrouter", mp.EffortFormat)
	}
	if _, ok := mp.EffortLevels["max"]; !ok {
		t.Fatal("openrouter claude should offer max")
	}
	if _, ok := mp.EffortLevels["medium"]; ok {
		t.Fatal("openrouter claude-opus-4-7 should not offer medium")
	}
}

func TestLookupGoogleProfileUppercase(t *testing.T) {
	mp := LookupModelProfile("google", "gemini-3.1-pro-preview")
	if mp.EffortFormat != EffortGoogle {
		t.Fatalf("google format = %q, want google", mp.EffortFormat)
	}
	if v, _ := mp.EffortLevels["high"]; v != "HIGH" {
		t.Fatalf("gemini high wire = %v, want HIGH", v)
	}
	if _, ok := mp.EffortLevels["medium"]; ok {
		t.Fatal("gemini-3.1-pro should not offer medium")
	}
}

func TestLookupUnknownProviderZero(t *testing.T) {
	if p := LookupProviderProfile("does-not-exist"); p.Provider != "" {
		t.Fatalf("unknown provider profile = %+v, want zero", p)
	}
	mp := LookupModelProfile("does-not-exist", "m")
	if mp.EffortFormat != "" || mp.EffortLevels != nil {
		t.Fatalf("unknown model profile = %+v, want zero", mp)
	}
}

func TestRegisterProviderProfileMerge(t *testing.T) {
	registerProviderProfile(ProviderProfile{
		Provider:     "testmerge",
		EffortFormat: EffortQwen,
		Models: map[string]ModelProfile{
			"m1": {EffortLevels: EffortLevelMap{"low": "low"}},
		},
	})
	defer func() { delete(providerProfiles, "testmerge") }()

	p := LookupProviderProfile("testmerge")
	if p.EffortFormat != EffortQwen {
		t.Fatalf("testmerge format = %q", p.EffortFormat)
	}
	if _, ok := p.Models["m1"]; !ok {
		t.Fatal("testmerge m1 missing")
	}
	// empty provider ignored
	registerProviderProfile(ProviderProfile{Provider: ""})
	if _, ok := providerProfiles[""]; ok {
		t.Fatal("empty provider should not be registered")
	}
}
