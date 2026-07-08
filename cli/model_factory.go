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
	"fmt"

	"github.com/inferglow/model"
)

// buildModelRequester constructs a model.ModelRequester from CLI config.
// It uses StaticConfigProvider + NewOpenAIProviderFromConfig (or provider-specific
// constructors) to build an OpenAI-compatible provider that satisfies the full
// ModelRequester interface (StreamRequester + ResponseBroadcaster).
func buildModelRequester(cfg CLIConfig) (model.ModelRequester, error) {
	if cfg.LLM.Endpoint == "" {
		return nil, fmt.Errorf("llm.endpoint is required")
	}

	provider := cfg.LLM.Provider
	if provider == "" {
		provider = "openai"
	}

	// StaticConfigProvider uses traverseMap which splits on ".", so keys must
	// be nested maps matching the provider prefix (e.g. {"openai": {"api_key": ...}}).
	providerValues := map[string]any{
		"base_url": cfg.LLM.Endpoint,
		"model":    cfg.LLM.Model,
	}
	if cfg.LLM.APIKey != "" {
		providerValues["api_key"] = cfg.LLM.APIKey
	}

	cp := &model.StaticConfigProvider{Values: map[string]any{
		provider: providerValues,
	}}

	switch provider {
	case "deepseek":
		return model.NewDeepSeekProviderFromConfig(cp)
	case "anthropic":
		return model.NewAnthropicProviderFromConfig(cp)
	case "qwen":
		return model.NewQwenProviderFromConfig(cp)
	case "glm":
		return model.NewGLMProviderFromConfig(cp)
	case "kimi":
		return model.NewKimiProviderFromConfig(cp)
	case "mimo":
		return model.NewMiMoProviderFromConfig(cp)
	default:
		// Default to OpenAI-compatible (covers local servers, vLLM, Ollama with
		// OpenAI compat, etc.)
		return model.NewOpenAIProviderFromConfig(cp)
	}
}
