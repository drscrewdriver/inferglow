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

// Provider profile registry (LLM-provider-port P1): data-driven descriptions
// of the providers pi-ai knows, so inferglow can declare a provider's wire
// protocol (EffortFormat) and per-model effort maps (EffortLevels) without
// hand-writing a provider per vendor.
//
// The data mirrors pi-ai 0.82.1 `dist/providers/data/*.json` + DSH adapter
// facts (see docs/guides/effort-and-providers-pi-ai-reference.md). Profiles
// are merged with config: config base_url/model always win; the profile only
// supplies protocol facts (effort format + level maps + defaults).

// ModelProfile describes one model's protocol facts.
type ModelProfile struct {
	// EffortFormat is the wire format for this model (defaults to the
	// provider profile's format when empty).
	EffortFormat EffortWireFormat
	// EffortLevels maps a semantic level to its wire value (nil entry =
	// not offered). Absent = no declared map (raw passthrough).
	EffortLevels EffortLevelMap
	// DefaultEffort is the provider/model default semantic level ("" = none).
	DefaultEffort string
}

// ProviderProfile describes a provider's protocol facts shared across models.
type ProviderProfile struct {
	// Provider key (matches DEFAULT_SETTINGS / providers.list keys).
	Provider string
	// EffortFormat is the default wire format for this provider's models.
	EffortFormat EffortWireFormat
	// Models holds per-model protocol facts keyed by model id. A missing
	// model falls back to the provider-level format with no level map.
	Models map[string]ModelProfile
}

// providerProfiles is the built-in registry. Seeded with the core protocol
// family; the generator (P4) appends the long tail from pi-ai data.
var providerProfiles = map[string]ProviderProfile{}

// LookupProviderProfile returns the profile for a provider key (zero value
// when unknown).
func LookupProviderProfile(provider string) ProviderProfile {
	return providerProfiles[provider]
}

// LookupModelProfile resolves the protocol facts for a provider/model pair:
// model-level facts win over provider-level defaults.
func LookupModelProfile(provider, modelName string) ModelProfile {
	p := providerProfiles[provider]
	if m, ok := p.Models[modelName]; ok {
		mp := m
		if mp.EffortFormat == "" {
			mp.EffortFormat = p.EffortFormat
		}
		return mp
	}
	return ModelProfile{EffortFormat: p.EffortFormat}
}

// EffortProfileApplier is implemented by providers that carry the semantic
// effort translation fields (EffortFormat + EffortLevels).
type EffortProfileApplier interface {
	applyEffortProfile(ModelProfile)
}

// ApplyEffortProfile wires the registry's protocol facts (effort format +
// level map) into a provider. Providers that do not carry the fields are
// left unchanged. Unknown providers/models are a no-op (raw passthrough).
// This is the single choke point the CLI calls after constructing a
// requester, so both startup and /model runtime switches pick up the facts.
func ApplyEffortProfile(p any, provider, modelName string) {
	a, ok := p.(EffortProfileApplier)
	if !ok {
		return
	}
	mp := LookupModelProfile(provider, modelName)
	if mp.EffortFormat == "" && mp.EffortLevels == nil {
		return
	}
	a.applyEffortProfile(mp)
}

// registerProviderProfile adds/merges a provider profile (test + generator
// entry point). Existing entries are overwritten key by key.
func registerProviderProfile(p ProviderProfile) {
	if p.Provider == "" {
		return
	}
	providerProfiles[p.Provider] = p
}

// ---------------------------------------------------------------------------
// Built-in core profiles (hand-curated from pi-ai / DSH authoritative data;
// the P4 generator extends this set).
// ---------------------------------------------------------------------------

func init() {
	// DeepSeek official (DSH llm-deepseek adapter authoritative):
	// off/low/high/max, thinking:{type} + reasoning_effort.
	registerProviderProfile(ProviderProfile{
		Provider:     "deepseek",
		EffortFormat: EffortDeepSeek,
		Models: map[string]ModelProfile{
			"deepseek-chat": {
				EffortLevels: EffortLevelMap{"off": nil, "low": "low", "high": "high", "max": "max"},
				DefaultEffort: "high",
			},
			"deepseek-v4-flash": {
				EffortLevels: EffortLevelMap{"off": nil, "low": "low", "high": "high", "max": "max"},
				DefaultEffort: "high",
			},
			"deepseek-v4-pro": {
				EffortLevels: EffortLevelMap{"off": nil, "low": "low", "high": "high", "max": "max"},
				DefaultEffort: "high",
			},
			"deepseek-reasoner": {
				EffortLevels: EffortLevelMap{"off": nil, "low": "low", "high": "high", "max": "max"},
				DefaultEffort: "high",
			},
		},
	})

	// OpenRouter aggregate: reasoning:{effort}.
	registerProviderProfile(ProviderProfile{
		Provider:     "openrouter",
		EffortFormat: EffortOpenRouter,
		Models: map[string]ModelProfile{
			"deepseek/deepseek-v4-flash": {
				EffortLevels: EffortLevelMap{"off": nil, "high": "high", "xhigh": "xhigh"},
				DefaultEffort: "high",
			},
			"deepseek/deepseek-v4-pro": {
				EffortLevels: EffortLevelMap{"off": nil, "high": "high", "xhigh": "xhigh"},
				DefaultEffort: "high",
			},
			"anthropic/claude-opus-4-7": {
				EffortLevels: EffortLevelMap{"off": nil, "xhigh": "xhigh", "max": "max"},
				DefaultEffort: "max",
			},
			"anthropic/claude-sonnet-5": {
				EffortLevels: EffortLevelMap{"off": nil, "xhigh": "xhigh", "max": "max"},
				DefaultEffort: "max",
			},
			"openai/gpt-5.4": {
				EffortLevels: EffortLevelMap{"off": "none", "low": "low", "medium": "medium", "high": "high", "xhigh": "xhigh"},
				DefaultEffort: "medium",
			},
			"openai/gpt-5.6-luna": {
				EffortLevels: EffortLevelMap{"off": "none", "low": "low", "medium": "medium", "high": "high", "xhigh": "xhigh", "max": "max"},
				DefaultEffort: "medium",
			},
		},
	})

	// ZAI / GLM (Z.AI coding paas): thinking:{type} + reasoning_effort.
	// glm-5.2 collapses low/medium/high to "high" per pi-ai.
	registerProviderProfile(ProviderProfile{
		Provider:     "zai",
		EffortFormat: EffortZAI,
		Models: map[string]ModelProfile{
			"glm-5.2": {
				EffortLevels: EffortLevelMap{"off": nil, "low": "high", "medium": "high", "high": "high", "max": "max"},
				DefaultEffort: "high",
			},
		},
	})

	// Google (Gemini generativelanguage): thinkingConfig uppercase.
	registerProviderProfile(ProviderProfile{
		Provider:     "google",
		EffortFormat: EffortGoogle,
		Models: map[string]ModelProfile{
			"gemini-3.1-pro-preview": {
				EffortLevels: EffortLevelMap{"off": nil, "low": "LOW", "high": "HIGH"},
				DefaultEffort: "high",
			},
			"gemini-3-pro-preview": {
				EffortLevels: EffortLevelMap{"off": nil, "low": "LOW", "high": "HIGH"},
				DefaultEffort: "high",
			},
			"gemma-4-31b-it": {
				EffortLevels: EffortLevelMap{"off": nil, "minimal": "MINIMAL", "high": "HIGH"},
				DefaultEffort: "high",
			},
		},
	})

	// Anthropic: thinking:{type,effort}.
	registerProviderProfile(ProviderProfile{
		Provider:     "anthropic",
		EffortFormat: EffortAnthropic,
		Models: map[string]ModelProfile{
			"claude-opus-4-7": {
				EffortLevels: EffortLevelMap{"off": nil, "xhigh": "xhigh", "max": "max"},
				DefaultEffort: "max",
			},
			"claude-opus-4-8": {
				EffortLevels: EffortLevelMap{"off": nil, "xhigh": "xhigh", "max": "max"},
				DefaultEffort: "max",
			},
			"claude-sonnet-5": {
				EffortLevels: EffortLevelMap{"off": nil, "xhigh": "xhigh", "max": "max"},
				DefaultEffort: "max",
			},
		},
	})

	// Mistral: routed through the OpenAI-compatible endpoint (/v1/chat/completions)
	// → reasoning_effort wire (EffortOpenAI). The native reasoningEffort wire
	// (EffortMistral) would only apply to a future native mistral-conversations
	// provider; the format constant is kept for that path.
	registerProviderProfile(ProviderProfile{
		Provider:     "mistral",
		EffortFormat: EffortOpenAI,
	})

	// Qwen (DashScope): enable_thinking.
	registerProviderProfile(ProviderProfile{
		Provider:     "qwen",
		EffortFormat: EffortQwen,
	})

	// Moonshot / Kimi: openai-compat reasoning_effort (kimi-k3 low/high/max).
	registerProviderProfile(ProviderProfile{
		Provider: "moonshotai",
		Models: map[string]ModelProfile{
			"kimi-k3": {
				EffortLevels: EffortLevelMap{"off": nil, "low": "low", "high": "high", "max": "max"},
				DefaultEffort: "high",
			},
		},
	})

	// StepFun / SiliconFlow / Groq / xAI / Together / NVIDIA / Cerebras /
	// Fireworks / HuggingFace / Cloudflare etc.: OpenAI-compat reasoning_effort
	// (the default). Profiles without a declared format need no entry — the
	// OpenAICompatibleProvider default (EffortOpenAI) applies. The P4
	// generator extends this registry with their per-model level maps.
}
