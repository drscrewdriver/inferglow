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

// TestLLMPortProviderFactories verifies the new OpenAI-compatible providers
// (LLM-provider-port P5) construct from DEFAULT_SETTINGS and carry the right
// ProviderName.
func TestLLMPortProviderFactories(t *testing.T) {
	cases := []struct {
		prefix       string
		factory      func(ConfigProvider) (*OpenAICompatibleProvider, error)
		wantName     string
		wantBaseURL  string
	}{
		{"mistral", NewMistralProviderFromConfig, "mistral", "https://api.mistral.ai/v1"},
		{"groq", NewGroqProviderFromConfig, "groq", "https://api.groq.com/openai/v1"},
		{"xai", NewXAIProviderFromConfig, "xai", "https://api.x.ai/v1"},
		{"together", NewTogetherProviderFromConfig, "together", "https://api.together.ai/v1"},
		{"zai", NewZAIProviderFromConfig, "zai", "https://api.z.ai/api/coding/paas/v4"},
		{"moonshotai", NewMoonshotAIProviderFromConfig, "moonshotai", "https://api.moonshot.ai/v1"},
		{"nvidia", NewNVIDIAProviderFromConfig, "nvidia", "https://integrate.api.nvidia.com/v1"},
	}
	for _, c := range cases {
		cp := &StaticConfigProvider{Values: map[string]any{
			c.prefix: map[string]any{
				"api_key": "test-key",
			},
		}}
		p, err := c.factory(cp)
		if err != nil {
			t.Fatalf("%s factory: %v", c.prefix, err)
		}
		if p.Name() != c.wantName {
			t.Errorf("%s name = %q, want %q", c.prefix, p.Name(), c.wantName)
		}
		ds := DEFAULT_SETTINGS[c.prefix]
		if ds == nil {
			t.Fatalf("%s missing from DEFAULT_SETTINGS", c.prefix)
		}
		if b, _ := ds["base_url"].(string); b != c.wantBaseURL {
			t.Errorf("%s DEFAULT_SETTINGS base_url = %q, want %q", c.prefix, b, c.wantBaseURL)
		}
	}
}

// TestLLMPortApplyEffortProfile verifies ApplyEffortProfile wires the
// registry facts (format + level map) into a provider.
func TestLLMPortApplyEffortProfile(t *testing.T) {
	// openrouter/claude-opus-4-7 profile: openrouter format, xhigh/max levels.
	p := &OpenAICompatibleProvider{}
	ApplyEffortProfile(p, "openrouter", "anthropic/claude-opus-4-7")
	if p.EffortFormat != EffortOpenRouter {
		t.Fatalf("openrouter format = %q, want %q", p.EffortFormat, EffortOpenRouter)
	}
	if p.EffortLevels == nil {
		t.Fatal("openrouter level map should be wired")
	}
	if _, ok := p.EffortLevels["max"]; !ok {
		t.Fatal("openrouter claude should offer max")
	}

	// Generated provider: amazon-bedrock claude-opus-4-7 (bedrock format).
	p2 := &OpenAICompatibleProvider{}
	ApplyEffortProfile(p2, "amazon-bedrock", "anthropic.claude-opus-4-7")
	if p2.EffortFormat != EffortBedrock {
		t.Fatalf("bedrock format = %q, want %q", p2.EffortFormat, EffortBedrock)
	}

	// Unknown provider: no-op.
	p3 := &OpenAICompatibleProvider{}
	ApplyEffortProfile(p3, "no-such-provider", "m")
	if p3.EffortFormat != "" || p3.EffortLevels != nil {
		t.Fatalf("unknown provider should be untouched, got format=%q levels=%v", p3.EffortFormat, p3.EffortLevels)
	}
}

// TestLLMPortZaiProfileCollapsed verifies the zai glm-5.2 collapsed map:
// low/medium/high all wire to "high".
func TestLLMPortZaiProfileCollapsed(t *testing.T) {
	mp := LookupModelProfile("zai", "glm-5.2")
	if mp.EffortFormat != EffortZAI {
		t.Fatalf("zai format = %q, want zai", mp.EffortFormat)
	}
	for _, lv := range []string{"low", "medium", "high"} {
		if v, ok := mp.EffortLevels[lv]; !ok || v != "high" {
			t.Fatalf("zai glm-5.2 %s = %v, want \"high\" (collapsed)", lv, v)
		}
	}
}
