package model

import "testing"

// TestCacheCapabilityFor_KnownProviders verifies that each provider in
// ProviderCacheProfiles is returned correctly.
func TestCacheCapabilityFor_KnownProviders(t *testing.T) {
	cases := []struct {
		name             string
		wantPrefix       bool
		wantGranularity  string
		wantSystemPrefix bool
		wantMinContext   int
	}{
		{"openai", true, "token", true, 128000},
		{"deepseek", true, "token", true, 64000},
		{"glm", true, "block", true, 128000},
		{"kimi", true, "token", true, 128000},
		{"qwen", true, "token", true, 32000},
		{"anthropic", true, "token", true, 200000},
		{"ollama", false, "none", false, 8192},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := CacheCapabilityFor(tc.name)
			if c.PrefixCache != tc.wantPrefix {
				t.Errorf("PrefixCache = %v, want %v", c.PrefixCache, tc.wantPrefix)
			}
			if c.PrefixCacheGranularity != tc.wantGranularity {
				t.Errorf("PrefixCacheGranularity = %q, want %q", c.PrefixCacheGranularity, tc.wantGranularity)
			}
			if c.SupportsSystemPrefix != tc.wantSystemPrefix {
				t.Errorf("SupportsSystemPrefix = %v, want %v", c.SupportsSystemPrefix, tc.wantSystemPrefix)
			}
			if c.MaxEffectiveContext < tc.wantMinContext {
				t.Errorf("MaxEffectiveContext = %d, want >= %d", c.MaxEffectiveContext, tc.wantMinContext)
			}
		})
	}
}

// TestCacheCapabilityFor_UnknownProvider verifies the conservative default
// (no caching) is returned for an unknown provider name.
func TestCacheCapabilityFor_UnknownProvider(t *testing.T) {
	c := CacheCapabilityFor("totally-unknown-provider")
	if c.PrefixCache {
		t.Errorf("PrefixCache should be false for unknown provider")
	}
	if c.PrefixCacheGranularity != "" {
		t.Errorf("PrefixCacheGranularity should be empty for unknown provider, got %q", c.PrefixCacheGranularity)
	}
	if c.SupportsSystemPrefix {
		t.Errorf("SupportsSystemPrefix should be false for unknown provider")
	}
	if c.MaxEffectiveContext != 0 {
		t.Errorf("MaxEffectiveContext should be 0 for unknown provider, got %d", c.MaxEffectiveContext)
	}
}

// TestProviderCacheProfiles_ContainsExpectedProviders verifies the registry
// contains all the providers documented in the spec.
func TestProviderCacheProfiles_ContainsExpectedProviders(t *testing.T) {
	expected := []string{"openai", "deepseek", "glm", "kimi", "qwen", "anthropic", "ollama"}
	for _, name := range expected {
		if _, ok := ProviderCacheProfiles[name]; !ok {
			t.Errorf("expected %q in ProviderCacheProfiles", name)
		}
	}
}
