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
