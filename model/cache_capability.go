package model

// CacheCapability describes a provider's prefix cache capabilities.
type CacheCapability struct {
	// PrefixCache: whether prefix caching is supported at all
	PrefixCache bool
	// PrefixCacheGranularity: "token" / "block" / "none"
	PrefixCacheGranularity string
	// SupportsSystemPrefix: whether system prompt can be cached independently
	SupportsSystemPrefix bool
	// MaxEffectiveContext: effective context length beyond which cache invalidates
	MaxEffectiveContext int
}

// ProviderCacheProfiles is the per-provider default cache capability.
// Keys are provider Name() values (e.g., "openai", "deepseek", "glm", "kimi", "qwen", "anthropic", "ollama").
var ProviderCacheProfiles = map[string]CacheCapability{
	"openai":    {PrefixCache: true, PrefixCacheGranularity: "token", SupportsSystemPrefix: true, MaxEffectiveContext: 128000},
	"deepseek":  {PrefixCache: true, PrefixCacheGranularity: "token", SupportsSystemPrefix: true, MaxEffectiveContext: 64000},
	"glm":       {PrefixCache: true, PrefixCacheGranularity: "block", SupportsSystemPrefix: true, MaxEffectiveContext: 128000},
	"kimi":      {PrefixCache: true, PrefixCacheGranularity: "token", SupportsSystemPrefix: true, MaxEffectiveContext: 128000},
	"qwen":      {PrefixCache: true, PrefixCacheGranularity: "token", SupportsSystemPrefix: true, MaxEffectiveContext: 32000},
	"anthropic": {PrefixCache: true, PrefixCacheGranularity: "token", SupportsSystemPrefix: true, MaxEffectiveContext: 200000},
	"ollama":    {PrefixCache: false, PrefixCacheGranularity: "none", SupportsSystemPrefix: false, MaxEffectiveContext: 8192},
}

// CacheCapabilityFor returns the cache profile for a provider name.
// Falls back to a conservative default (no caching) for unknown providers.
func CacheCapabilityFor(providerName string) CacheCapability {
	if c, ok := ProviderCacheProfiles[providerName]; ok {
		return c
	}
	return CacheCapability{} // all zero = no caching
}

// CacheAwareProvider is an optional interface a Provider may implement to
// expose its prefix-cache capabilities. Code that wants to take advantage of
// prefix caching should type-assert against this interface; providers that do
// not implement it are treated as having no caching capability.
type CacheAwareProvider interface {
	ModelRequester
	CacheCapability() CacheCapability
}
