// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software are
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

package server

import (
	"context"
	"fmt"
	"sort"

	agentpkg "github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/model"
	"github.com/inferglow/server/config"
	"github.com/inferglow/session"
)

// defaultAgentWindowTokens mirrors the CLI default window size; the server
// YAML config has no dedicated knob yet.
const defaultAgentWindowTokens = 32000

// ConfigAgent adapts a real *agent.Agent to the server's AgentLike narrow
// interface (*agent.Agent's variadic Run does not satisfy it directly) and
// carries JSON identity so /v1/agents serializes id/name/model instead of an
// empty object (contrast: demoAgent marshals as {}).
type ConfigAgent struct {
	*agentpkg.Agent
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
}

// Run satisfies AgentLike with the plain two-argument form. Per-run streaming
// callbacks go through RunWithCallbacks instead: an adapter cannot expose both
// signatures under one name (the two-parameter Run shadows the variadic one,
// making a CallbacksRunner type assertion impossible).
func (c *ConfigAgent) Run(ctx context.Context, userMessage string) (string, error) {
	return c.Agent.Run(ctx, userMessage)
}

// RunWithCallbacks exposes the underlying agent's variadic Run so
// handleStreamRun can inject per-run SSE streaming callbacks.
func (c *ConfigAgent) RunWithCallbacks(ctx context.Context, userMessage string, opts ...agentpkg.RunOption) (string, error) {
	return c.Agent.Run(ctx, userMessage, opts...)
}

// ConfigAgentStore serves real agents built from the YAML llm.providers
// section. Read-only: agents come from config, not from POST /v1/agents.
type ConfigAgentStore struct {
	agents map[string]AgentLike
	order  []string // provider names, sorted for deterministic listing
}

func (s *ConfigAgentStore) Get(id string) AgentLike { return s.agents[id] }

func (s *ConfigAgentStore) List() []AgentLike {
	out := make([]AgentLike, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.agents[id])
	}
	return out
}

func (s *ConfigAgentStore) Create(_ AgentConfig) (string, error) {
	return "", fmt.Errorf("config agent store is read-only (agents come from the YAML llm section)")
}

func (s *ConfigAgentStore) Delete(id string) error {
	delete(s.agents, id)
	return nil
}

// NewConfigAgentStore builds one pure-chat agent per configured LLM provider
// (agent id = provider key). Pure-chat v1: no tool actions are registered, so
// these agents answer with the LLM directly (streaming deltas included) and
// never invoke tools; the CLI remains the tool-capable runtime.
func NewConfigAgentStore(llm config.MultiLLMConfig) (*ConfigAgentStore, error) {
	if len(llm.Providers) == 0 {
		return nil, fmt.Errorf("no llm.providers configured")
	}
	s := &ConfigAgentStore{agents: make(map[string]AgentLike, len(llm.Providers))}
	for name := range llm.Providers {
		s.order = append(s.order, name)
	}
	sort.Strings(s.order)

	for _, name := range s.order {
		lc := llm.Providers[name]
		req, err := modelRequesterFromServerConfig(name, lc)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", name, err)
		}
		sess := session.NewSession("sess-"+name, defaultAgentWindowTokens)
		ag := agentpkg.New(sess, agentpkg.NewActionExtension(), req)
		s.agents[name] = &ConfigAgent{Agent: ag, ID: name, Name: name, Model: lc.Model}
	}
	return s, nil
}

// modelRequesterFromServerConfig maps the server YAML llm entry onto the
// model package's provider constructors. Mirrors cli/model_factory.go's
// buildModelRequester (kept in sync by hand); the default branch covers every
// OpenAI-compatible endpoint (vLLM, Ollama, openrouter, siliconflow, ...).
func modelRequesterFromServerConfig(name string, lc config.LLMConfig) (model.ModelRequester, error) {
	if lc.BaseURL == "" {
		return nil, fmt.Errorf("base_url is required")
	}
	provider := lc.Provider
	if provider == "" {
		provider = name
	}

	providerValues := map[string]any{
		"base_url": lc.BaseURL,
		"model":    lc.Model,
	}
	if key := lc.ResolveAPIKey(); key != "" {
		providerValues["api_key"] = key
	}
	cp := &model.StaticConfigProvider{Values: map[string]any{
		provider: providerValues,
	}}

	var req model.ModelRequester
	var err error
	switch provider {
	case "deepseek":
		req, err = model.NewDeepSeekProviderFromConfig(cp)
	case "anthropic":
		req, err = model.NewAnthropicProviderFromConfig(cp)
	case "qwen":
		req, err = model.NewQwenProviderFromConfig(cp)
	case "glm":
		req, err = model.NewGLMProviderFromConfig(cp)
	case "kimi":
		req, err = model.NewKimiProviderFromConfig(cp)
	case "mimo":
		req, err = model.NewMiMoProviderFromConfig(cp)
	case "google":
		// Google native protocol (non-OpenAI-compatible).
		req, err = model.NewGoogleProviderFromConfig(cp)
	default:
		// OpenAI-compatible default, keyed by the actual provider name so
		// multi-provider configs keep working (same rationale as the CLI).
		req, err = model.NewOpenAICompatProviderFromConfig(cp, provider, provider)
	}
	if err != nil {
		return nil, err
	}
	model.ApplyEffortProfile(req, provider, lc.Model)
	return req, nil
}
