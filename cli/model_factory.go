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
	"context"
	"fmt"
	"strings"

	"github.com/inferglow/context/compress"
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

// buildCompressModelClient constructs a compress.CompressModelClient from an
// LLMConfig. Returns (nil, nil) when cfg is nil or has no endpoint, signalling
// callers to skip compression-model wiring.
func buildCompressModelClient(cfg *LLMConfig) (compress.CompressModelClient, error) {
	if cfg == nil || cfg.Endpoint == "" {
		return nil, nil
	}
	// Build a full CLIConfig so buildModelRequester can reuse its provider logic.
	tmpCfg := CLIConfig{LLM: *cfg}
	req, err := buildModelRequester(tmpCfg)
	if err != nil {
		return nil, fmt.Errorf("compress model: %w", err)
	}
	return &compressModelAdapter{requester: req}, nil
}

// compressModelAdapter wraps a model.ModelRequester into the narrow
// compress.CompressModelClient interface (non-streaming request + collect).
type compressModelAdapter struct {
	requester model.ModelRequester
}

func (a *compressModelAdapter) Compress(ctx context.Context, level int, prompt string) (string, error) {
	req := &model.ModelRequest{
		System: "You are a compression engine. Produce a concise summary retaining key facts.",
		Input:  prompt,
	}
	data, err := a.requester.GenerateRequestData(ctx, req)
	if err != nil {
		return "", fmt.Errorf("compress generate: %w", err)
	}
	stream, err := a.requester.RequestModel(ctx, data)
	if err != nil {
		return "", fmt.Errorf("compress request: %w", err)
	}
	var sb strings.Builder
	for chunk := range stream {
		if chunk.IsDone {
			break
		}
		sb.WriteString(chunk.Delta)
	}
	return sb.String(), nil
}

func (a *compressModelAdapter) Available() bool {
	// Lightweight probe: generate request data for a minimal request.
	ctx, cancel := context.WithTimeout(context.Background(), 3_000_000_000) // 3s
	defer cancel()
	req := &model.ModelRequest{Input: "ping"}
	_, err := a.requester.GenerateRequestData(ctx, req)
	return err == nil
}
