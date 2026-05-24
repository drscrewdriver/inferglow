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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package model

import "testing"

// TestFactoryFullURL verifies that all three primary factory functions
// (OpenAI / Anthropic / Ollama) correctly propagate the `<prefix>.full_url`
// config key to Provider.FullURL. This is the integration seam between
// LoadProviderConfig and the Provider struct fields.
func TestFactoryFullURL(t *testing.T) {
	t.Run("openai_factory_reads_full_url", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"openai": map[string]any{
				"api_key":  "sk-test",
				"full_url": "https://proxy.example.com/chat",
			},
		}}
		p, err := NewOpenAIProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewOpenAIProviderFromConfig failed: %v", err)
		}
		if p.FullURL != "https://proxy.example.com/chat" {
			t.Errorf("OpenAICompatibleProvider.FullURL = %q, want %q", p.FullURL, "https://proxy.example.com/chat")
		}
	})

	t.Run("anthropic_factory_reads_full_url", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"anthropic": map[string]any{
				"api_key":  "sk-test",
				"full_url": "https://proxy.example.com/messages",
			},
		}}
		p, err := NewAnthropicProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewAnthropicProviderFromConfig failed: %v", err)
		}
		if p.FullURL != "https://proxy.example.com/messages" {
			t.Errorf("AnthropicCompatibleProvider.FullURL = %q, want %q", p.FullURL, "https://proxy.example.com/messages")
		}
	})

	// Ollama has no factory function (LoadProviderConfig requires api_key
	// which Ollama doesn't need). Its FullURL field is verified directly in
	// TestOllamaFullURL via struct construction (see ollama_fullurl_test.go).

	t.Run("default_full_url_empty_preserves_legacy_behavior", func(t *testing.T) {
		// When full_url is not set, Provider.FullURL should be empty so
		// ResolveURL degrades to the legacy TrimRight + defaultPath behavior.
		cp := &StaticConfigProvider{Values: map[string]any{
			"openai": map[string]any{
				"api_key": "sk-test",
			},
		}}
		p, err := NewOpenAIProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewOpenAIProviderFromConfig failed: %v", err)
		}
		if p.FullURL != "" {
			t.Errorf("OpenAICompatibleProvider.FullURL = %q, want empty", p.FullURL)
		}
	})
}

// TestFactoryContentMapping verifies that LoadProviderConfig reads the
// `<prefix>.content_mapping` subtree and the OpenAI factory propagates it to
// Provider.ContentMapping. Spec: model-parity Phase 3.
func TestFactoryContentMapping(t *testing.T) {
	t.Run("openai_factory_reads_content_mapping", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"openai": map[string]any{
				"api_key": "sk-test",
				"content_mapping": map[string]any{
					"reasoning": "data.thinking",
					"delta":     "choices[0].delta.content",
				},
			},
		}}
		p, err := NewOpenAIProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewOpenAIProviderFromConfig failed: %v", err)
		}
		if p.ContentMapping == nil {
			t.Fatal("ContentMapping = nil, want non-nil")
		}
		if got := p.ContentMapping["reasoning"]; got != "data.thinking" {
			t.Errorf("ContentMapping[reasoning] = %q, want %q", got, "data.thinking")
		}
		if got := p.ContentMapping["delta"]; got != "choices[0].delta.content" {
			t.Errorf("ContentMapping[delta] = %q, want %q", got, "choices[0].delta.content")
		}
	})

	t.Run("missing_content_mapping_yields_nil", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"openai": map[string]any{
				"api_key": "sk-test",
			},
		}}
		p, err := NewOpenAIProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewOpenAIProviderFromConfig failed: %v", err)
		}
		if p.ContentMapping != nil {
			t.Errorf("ContentMapping = %v, want nil (legacy behavior)", p.ContentMapping)
		}
	})
}
