package model

import "testing"

// TestProviderCacheCapability_Anthropic verifies the Anthropic provider
// returns its cache capability from the profile.
func TestProviderCacheCapability_Anthropic(t *testing.T) {
	p := &AnthropicCompatibleProvider{}
	c := p.CacheCapability()
	if !c.PrefixCache {
		t.Errorf("Anthropic should support prefix cache")
	}
	if c.PrefixCacheGranularity != "token" {
		t.Errorf("Anthropic granularity = %q, want %q", c.PrefixCacheGranularity, "token")
	}
	if !c.SupportsSystemPrefix {
		t.Errorf("Anthropic should support system prefix")
	}
	if c.MaxEffectiveContext != 200000 {
		t.Errorf("Anthropic MaxEffectiveContext = %d, want 200000", c.MaxEffectiveContext)
	}
}

// TestProviderCacheCapability_Ollama verifies the Ollama provider returns
// the conservative no-cache profile.
func TestProviderCacheCapability_Ollama(t *testing.T) {
	p := &OllamaProvider{}
	c := p.CacheCapability()
	if c.PrefixCache {
		t.Errorf("Ollama should not support prefix cache")
	}
	if c.PrefixCacheGranularity != "none" {
		t.Errorf("Ollama granularity = %q, want %q", c.PrefixCacheGranularity, "none")
	}
	if c.SupportsSystemPrefix {
		t.Errorf("Ollama should not support system prefix")
	}
}

// TestProviderCacheCapability_OpenAICompatible verifies the OpenAI-compatible
// provider falls back to the conservative default because its Name() returns
// "openai-compatible" (not "openai").
func TestProviderCacheCapability_OpenAICompatible(t *testing.T) {
	p := &OpenAICompatibleProvider{}
	c := p.CacheCapability()
	if c.PrefixCache {
		t.Errorf("OpenAICompatibleProvider (name=%q) should fall back to no-cache default", p.Name())
	}
}

// TestProviderCacheCapability_Interface verifies that all three provider
// implementations satisfy the CacheAwareProvider interface.
func TestProviderCacheCapability_Interface(t *testing.T) {
	var _ CacheAwareProvider = (*OpenAICompatibleProvider)(nil)
	var _ CacheAwareProvider = (*AnthropicCompatibleProvider)(nil)
	var _ CacheAwareProvider = (*OllamaProvider)(nil)
}

// TestProviderCacheCapability_ProviderName verifies that a provider that sets
// ProviderName to a known profile name picks up the matching CacheCapability
// automatically via the Name() override.
func TestProviderCacheCapability_ProviderName(t *testing.T) {
	for _, name := range []string{"deepseek", "glm", "kimi", "qwen", "openai"} {
		t.Run(name, func(t *testing.T) {
			p := &OpenAICompatibleProvider{ProviderName: name}
			if p.Name() != name {
				t.Fatalf("Name() = %q, want %q", p.Name(), name)
			}
			c := p.CacheCapability()
			expected := ProviderCacheProfiles[name]
			if c != expected {
				t.Errorf("CacheCapability() = %+v, want %+v", c, expected)
			}
		})
	}
}
