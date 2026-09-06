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
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/inferglow/action"
	"github.com/inferglow/builtins/actions"
	"github.com/inferglow/model"
	agentpkg "github.com/inferglow/orchestrator/agent"
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

// NewConfigAgentStore builds one agent per configured LLM provider (agent id
// = provider key). toolDirs (absolute workspace roots) wires the file tools
// so models that answer with function calls get a real executor — without
// them the pure-chat agent has an empty registry and a tool-calling model
// just loops on "unknown action" (looks like a hang). nil/empty keeps the
// pure-chat form (used by tests).
func NewConfigAgentStore(llm config.MultiLLMConfig, toolDirs []string) (*ConfigAgentStore, error) {
	if len(llm.Providers) == 0 {
		return nil, fmt.Errorf("no llm.providers configured")
	}
	s := &ConfigAgentStore{agents: make(map[string]AgentLike, len(llm.Providers))}
	for name := range llm.Providers {
		s.order = append(s.order, name)
	}
	sort.Strings(s.order)

	// One shared action extension: the tools are stateless and safe to share
	// across provider agents.
	actExt := agentpkg.NewActionExtension()
	if len(toolDirs) > 0 {
		if err := registerWorkspaceTools(actExt, toolDirs); err != nil {
			return nil, err
		}
	}

	for _, name := range s.order {
		lc := llm.Providers[name]
		req, err := modelRequesterFromServerConfig(name, lc)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", name, err)
		}
		sess := session.NewSession("sess-"+name, defaultAgentWindowTokens)
		ag := agentpkg.New(sess, actExt, req)
		s.agents[name] = &ConfigAgent{Agent: ag, ID: name, Name: name, Model: lc.Model}
	}
	return s, nil
}

// registerWorkspaceTools wires the builtins file tools (list_dir, file_read,
// file_write, grep) confined to the given workspace roots, plus aliases for
// the tool names served models commonly invent (list_directory/read_file/
// write_file) so a model following its own template still lands on a real
// executor instead of "unknown action". Deliberately NO shell/bash tool:
// model-side command execution stays out of the server's default surface.
func registerWorkspaceTools(ext *agentpkg.ActionExtension, dirs []string) error {
	abs := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if a, err := filepath.Abs(d); err == nil {
			abs = append(abs, filepath.Clean(a))
		}
	}
	if len(abs) == 0 {
		return nil
	}
	listDir := actions.NewListDirAction(actions.ListDirConfig{AllowedDirs: abs})
	fileRead := actions.NewFileReadAction(actions.FileReadConfig{AllowedDirs: abs})
	fileWrite := actions.NewFileWriteAction(actions.FileWriteConfig{AllowedDirs: abs})
	grep := actions.NewGrepAction(&nativeGrepRunner{roots: abs})

	// Models speak workspace-relative paths ("."  for the root); the builtins
	// resolve relative paths against the process CWD, which on a server is
	// NOT the workspace. Rewrite relative "path" params against the first
	// workspace root before the builtin sees them.
	anchor := func(a *action.Action) *action.Action {
		a.Executor = pathAnchoredExecutor{inner: a.Executor, root: abs[0]}
		return a
	}
	listDir, fileRead, fileWrite, grep = anchor(listDir), anchor(fileRead), anchor(fileWrite), anchor(grep)

	for _, a := range []*action.Action{listDir, fileRead, fileWrite, grep} {
		if err := ext.Register(a); err != nil {
			return fmt.Errorf("register %s: %w", a.Name, err)
		}
	}
	for _, alias := range []struct{ name, of string }{
		{"list_directory", actions.ListDirActionID},
		{"read_file", actions.FileReadActionID},
		{"write_file", actions.FileWriteActionID},
	} {
		src, err := ext.GetRegistry().Get(alias.of)
		if err != nil {
			continue
		}
		if err := ext.Register(&action.Action{
			Name: alias.name, Description: src.Description + " (alias of " + alias.of + ")",
			Schema: src.Schema, Executor: src.Executor,
		}); err != nil {
			return fmt.Errorf("register alias %s: %w", alias.name, err)
		}
	}
	return nil
}

// pathAnchoredExecutor resolves workspace-relative "path" parameters against
// the workspace root before delegating to the real executor.
type pathAnchoredExecutor struct {
	inner action.ActionExecutor
	root  string
}

func (e pathAnchoredExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	if p, ok := input["path"].(string); ok && p != "" && !filepath.IsAbs(p) {
		input["path"] = filepath.Join(e.root, filepath.FromSlash(p))
	}
	return e.inner.Execute(ctx, input)
}

// nativeGrepRunner implements actions.GrepRunner in pure Go (no system grep —
// Windows server hosts don't ship one). Walks the workspace roots, matching
// lines with regexp, with binary/hidden-dir skips and hard result caps.
type nativeGrepRunner struct {
	roots []string
}

const (
	grepMaxFileSize    = 512 << 10 // skip files above this
	grepMaxMatches     = 200
	grepMaxFilesWalked = 5000
)

func (r *nativeGrepRunner) Run(ctx context.Context, req actions.GrepRequest) ([]actions.GrepMatch, error) {
	if req.Pattern == "" {
		return nil, fmt.Errorf("grep: pattern is required")
	}
	re, err := regexp.Compile(req.Pattern)
	if err != nil {
		return nil, fmt.Errorf("grep: bad pattern: %w", err)
	}
	// Search path: a requested subpath of (or inside) the roots; default ".".
	base := r.roots
	if req.Path != "" && req.Path != "." {
		p := filepath.Clean(req.Path)
		if !filepath.IsAbs(p) {
			p = filepath.Join(r.roots[0], p)
		}
		allowed := false
		for _, root := range r.roots {
			if strings.HasPrefix(p, root+string(filepath.Separator)) || p == root {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("grep: path %q is outside the workspace", req.Path)
		}
		base = []string{p}
	}
	skipNames := map[string]bool{".git": true, "node_modules": true, ".gobuild": true}
	matches := make([]actions.GrepMatch, 0, 32)
	walked := 0
	for _, root := range base {
		walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // unreadable entries are skipped, not fatal
			}
			if d.IsDir() {
				if skipNames[d.Name()] && path != root {
					return filepath.SkipDir
				}
				return nil
			}
			if walked++; walked > grepMaxFilesWalked {
				return filepath.SkipAll
			}
			info, err := d.Info()
			if err != nil || info.Size() > grepMaxFileSize {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil || bytes.IndexByte(data, 0) >= 0 {
				return nil // binary
			}
			rel, _ := filepath.Rel(root, path)
			for i, line := range strings.Split(string(data), "\n") {
				if re.MatchString(line) {
					matches = append(matches, actions.GrepMatch{
						File: filepath.ToSlash(rel), Line: i + 1,
						Content: strings.TrimSpace(line),
					})
					if len(matches) >= grepMaxMatches {
						return filepath.SkipAll
					}
				}
			}
			return ctx.Err()
		})
		if walkErr != nil {
			break
		}
	}
	return matches, nil
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
