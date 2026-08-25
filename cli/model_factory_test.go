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

	"github.com/inferglow/model"
)

// TestBuildModelRequesterNewProviders verifies the LLM-provider-port providers
// route through buildModelRequester and carry the effort profile facts.
func TestBuildModelRequesterNewProviders(t *testing.T) {
	cases := []struct {
		provider string
		model    string
		wantName string
	}{
		{"google", "gemini-3.1-pro-preview", "google"},
		{"mistral", "mistral-medium-latest", "mistral"},
		{"groq", "openai/gpt-oss-120b", "groq"},
		{"xai", "grok-4.5", "xai"},
		{"together", "openai/gpt-oss-120b", "together"},
		{"zai", "glm-5.2", "zai"},
		{"moonshotai", "kimi-k3", "moonshotai"},
		{"nvidia", "nvidia/nemotron-3-super-120b-a12b", "nvidia"},
		{"cerebras", "gpt-oss-120b", "cerebras"},
		{"huggingface", "openai/gpt-oss-120b", "huggingface"},
		{"fireworks", "accounts/fireworks/models/gpt-oss-120b", "fireworks"},
		{"qwen-token-plan-cn", "qwen3.7-plus", "qwen-token-plan-cn"},
	}
	for _, c := range cases {
		cfg := CLIConfig{LLM: LLMConfig{
			Endpoint: "https://example.test/v1",
			Model:    c.model,
			APIKey:   "test-key",
			Provider: c.provider,
		}}
		req, err := buildModelRequester(cfg)
		if err != nil {
			t.Fatalf("%s buildModelRequester: %v", c.provider, err)
		}
		if req.Name() != c.wantName {
			t.Errorf("%s name = %q, want %q", c.provider, req.Name(), c.wantName)
		}
	}
}

// TestBuildModelRequesterGoogleEffortProfile verifies that a google route gets
// the EffortGoogle format + uppercase level map wired through ApplyEffortProfile.
func TestBuildModelRequesterGoogleEffortProfile(t *testing.T) {
	cfg := CLIConfig{LLM: LLMConfig{
		Endpoint: "https://example.test/v1beta",
		Model:    "gemini-3.1-pro-preview",
		APIKey:   "test-key",
		Provider: "google",
	}}
	req, err := buildModelRequester(cfg)
	if err != nil {
		t.Fatalf("buildModelRequester: %v", err)
	}
	gp, ok := req.(*model.GoogleGenerativeProvider)
	if !ok {
		t.Fatalf("requester type = %T, want *GoogleGenerativeProvider", req)
	}
	if gp.EffortFormat != model.EffortGoogle {
		t.Fatalf("EffortFormat = %q, want google", gp.EffortFormat)
	}
	if gp.EffortLevels == nil {
		t.Fatal("EffortLevels should be wired")
	}
	if v, _ := gp.EffortLevels["high"]; v != "HIGH" {
		t.Fatalf("high wire = %v, want HIGH", v)
	}
}

// TestMultiProviderConfigResolvesRoute verifies the multi-provider config
// shape: providers.list with several entries, resolveModelRoute picks the
// active key, and every listed provider constructs a requester with the
// correct effort profile (LLM-provider-port P5 multi-provider support).
func TestMultiProviderConfigResolvesRoute(t *testing.T) {
	cfg := CLIConfig{
		Providers: ProvidersConfig{
			Active: "deepseek",
			List: map[string]LLMConfig{
				"deepseek": {
					Endpoint: "https://api.deepseek.com/v1",
					Model:    "deepseek-v4-pro",
					APIKey:   "key-ds",
					Provider: "deepseek",
				},
				"google": {
					Endpoint: "https://generativelanguage.googleapis.com/v1beta",
					Model:    "gemini-3.1-pro-preview",
					APIKey:   "key-g",
					Provider: "google",
				},
				"openrouter": {
					Endpoint: "https://openrouter.ai/api/v1",
					Model:    "anthropic/claude-opus-4-7",
					APIKey:   "key-or",
					Provider: "openrouter",
				},
			},
		},
	}

	// Active key wins.
	route := resolveModelRoute(cfg, nil)
	if route.Provider != "deepseek" || route.Model != "deepseek-v4-pro" {
		t.Fatalf("active route = %s/%s, want deepseek/deepseek-v4-pro", route.Provider, route.Model)
	}
	if route.Endpoint != "https://api.deepseek.com/v1" {
		t.Fatalf("active endpoint = %q", route.Endpoint)
	}

	// Every listed provider constructs a requester with correct effort facts.
	for key, lc := range cfg.Providers.List {
		req, err := buildModelRequester(CLIConfig{LLM: lc})
		if err != nil {
			t.Fatalf("%s buildModelRequester: %v", key, err)
		}
		if req.Name() != lc.Provider {
			t.Errorf("%s requester name = %q, want %q", key, req.Name(), lc.Provider)
		}
	}
	// google route carries EffortGoogle; deepseek carries EffortDeepSeek.
	gp, _ := buildModelRequester(CLIConfig{LLM: cfg.Providers.List["google"]})
	if gg, ok := gp.(*model.GoogleGenerativeProvider); !ok || gg.EffortFormat != model.EffortGoogle {
		t.Fatalf("google requester = %T, want *GoogleGenerativeProvider with EffortGoogle", gp)
	}
	dp, _ := buildModelRequester(CLIConfig{LLM: cfg.Providers.List["deepseek"]})
	if od, ok := dp.(*model.OpenAICompatibleProvider); !ok || od.EffortFormat != model.EffortDeepSeek {
		t.Fatalf("deepseek requester = %T, want *OpenAICompatibleProvider with EffortDeepSeek", dp)
	}
}
