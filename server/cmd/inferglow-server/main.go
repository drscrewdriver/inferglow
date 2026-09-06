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

// Command inferglow-server starts the InferGlow REST API server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/inferglow/builtins/actions"
	"github.com/inferglow/messagebus"
	"github.com/inferglow/observability"
	"github.com/inferglow/server"
	"github.com/inferglow/server/config"
)

// workspaceFlagList collects repeated -workspace name=rootDir flags.
type workspaceFlagList struct {
	seeds []server.WorkspaceSeed
}

func (w *workspaceFlagList) String() string {
	parts := make([]string, len(w.seeds))
	for i, s := range w.seeds {
		parts[i] = s.Name + "=" + s.Root
	}
	return strings.Join(parts, ",")
}

func (w *workspaceFlagList) Set(v string) error {
	name, root, ok := strings.Cut(v, "=")
	if !ok || strings.TrimSpace(name) == "" || strings.TrimSpace(root) == "" {
		return fmt.Errorf("-workspace expects name=rootDir, got %q", v)
	}
	w.seeds = append(w.seeds, server.WorkspaceSeed{Name: name, Root: root})
	return nil
}

func main() {
	var (
		addr        = flag.String("addr", ":8080", "Listen address")
		apiKey      = flag.String("api-key", "", "API key for Bearer auth (empty = disabled)")
		cors        = flag.String("cors", "", "Comma-separated CORS origins (empty = disabled)")
		timeout     = flag.Duration("timeout", 30*time.Second, "Request read timeout")
		usageDir    = flag.String("usage-dir", "data", "Directory holding sessions/*.usage.jsonl")
		demoAgent   = flag.Bool("demo-agent", false, "Wire a local echo agent (id a1) so the GUI chat can be exercised without a real model provider")
		configPath  = flag.String("config", "", "Server YAML config: llm.providers are wired as real chat agents (one per provider, pure-chat)")
		execEnable  = flag.Bool("exec", false, "Enable POST /v1/exec gated command execution for the webui terminal (requires -api-key)")
		ptyEnable   = flag.Bool("pty", false, "Enable GET /v1/pty persistent interactive shells for the webui terminal (full-permission; requires -api-key)")
		ptyShell    = flag.String("pty-shell", "", "Shell program for PTY sessions (default: cmd.exe on Windows, $SHELL elsewhere)")
		providerCfg = flag.String("provider-config", "", "Shared provider JSON config (etc/config.json schema). Default: project etc/config.json, falling back to ~/.inferglow/config.json")
		authOpen    = flag.Bool("auth-open", false, "T0 test mode: skip API-key checks entirely (any browser can use the webui; exec/pty still need -exec/-pty)")
	)
	wsFlags := &workspaceFlagList{}
	flag.Var(wsFlags, "workspace", "Seed a workspace for the webui selector, repeatable: -workspace name=rootDir")
	flag.Parse()

	cfg := server.DefaultConfig()
	cfg.Addr = *addr
	cfg.APIKey = *apiKey
	cfg.ReadTimeout = *timeout
	cfg.UsageDataDir = *usageDir
	cfg.ExecEnabled = *execEnable
	cfg.PTYEnabled = *ptyEnable
	cfg.PTYShell = *ptyShell
	cfg.AuthOpen = *authOpen
	if *authOpen {
		log.Println("⚠ 鉴权已关闭(-auth-open):任意浏览器可访问全部 API — 仅限本机/可信网络测试")
	}

	if *cors != "" {
		cfg.CORSOrigins = splitComma(*cors)
	}

	// Provider resolution order (agent/model list for the webui picker):
	// shared JSON (project etc/config.json over ~/.inferglow/config.json, the
	// TUI's own config) → server YAML llm.providers override → -demo-agent
	// echo agent appended last so it never crowds out real providers.
	merged := config.MultiLLMConfig{}
	var seedsFromConfig []server.WorkspaceSeed
	if shared, sharedPath, err := config.LoadSharedProviderConfig(*providerCfg); err != nil {
		log.Fatalf("load provider config: %v", err)
	} else if shared != nil {
		merged = shared.ToMultiLLM()
		if len(merged.Providers) > 0 {
			log.Printf("provider config loaded from %s (%d provider(s))", sharedPath, len(merged.Providers))
		}
		if len(shared.Workspaces) > 0 {
			for name, root := range shared.Workspaces {
				seedsFromConfig = append(seedsFromConfig, server.WorkspaceSeed{Name: name, Root: root})
			}
		}
	}

	var loadedConfig *config.Config
	if *configPath != "" {
		scfg, err := config.NewLoader(*configPath).Load()
		if err != nil {
			log.Fatalf("load config %s: %v", *configPath, err)
		}
		loadedConfig = scfg
		if len(scfg.LLM.Providers) > 0 {
			if merged.Providers == nil {
				merged.Providers = map[string]config.LLMConfig{}
			}
			for name, lc := range scfg.LLM.Providers {
				merged.Providers[name] = lc
			}
			if scfg.LLM.Default != "" {
				merged.Default = scfg.LLM.Default
			}
			log.Printf("yaml providers merged: %d provider(s) total", len(merged.Providers))
		}
	}

	// R8: shared task store must exist BEFORE agent wiring — the model-side
	// task_tracker tools register only when the store is non-nil.
	server.SetTaskStore(actions.NewTaskStore(filepath.Join(*usageDir, "tasks.json")))

	var agentStore server.AgentStore
	if len(merged.Providers) > 0 {
		// Tool roots: every workspace root known at startup (flags + shared
		// config + YAML). The agents' file tools are confined to these dirs;
		// empty list means pure-chat agents without tools.
		toolDirs := make([]string, 0, len(wsFlags.seeds)+len(seedsFromConfig))
		for _, seed := range wsFlags.seeds {
			toolDirs = append(toolDirs, seed.Root)
		}
		for _, seed := range seedsFromConfig {
			toolDirs = append(toolDirs, seed.Root)
		}
		if loadedConfig != nil {
			for _, root := range loadedConfig.Workspaces {
				toolDirs = append(toolDirs, root)
			}
		}
		store, err := server.NewConfigAgentStore(merged, toolDirs)
		if err != nil {
			log.Fatalf("wire real agents: %v", err)
		}
		agentStore = store
		log.Printf("real agents wired: %d provider(s), file tools on %d workspace root(s)", len(merged.Providers), len(toolDirs))
	}
	if *demoAgent {
		if agentStore != nil {
			agentStore = &agentStoreGroup{stores: []server.AgentStore{agentStore, newDemoAgentStore()}}
			log.Println("demo agent appended: id=a1 (echo)")
		} else {
			agentStore = newDemoAgentStore()
			log.Println("demo agent wired: id=a1 (echo)")
		}
	}

	srv := server.NewServer(cfg, agentStore)

	// C-3: demo wiring of the in-memory message bus (out-of-the-box).
	srv.SetMessageBus(messagebus.NewInMemoryMessageBus())

	// OT-13: bounded in-memory span ring so /v1/observability/* and the
	// webui-dsh 轨迹 panel have real data. Spans are recorded by the
	// stream-run bridge (agent/llm/tool lifecycles).
	srv.SetSpanCollector(observability.NewSpanCollector(4096))

	// C-4~C-7: default in-memory wiring for the management backend, zero-config.
	// R8: disk persistence — sessions, chat history (incl. run traces) and
	// later tasks survive restarts under -usage-dir.
	persistDir := *usageDir
	sessStore := server.NewSessionStore()
	if sessStore.SetPersistence(filepath.Join(persistDir, "sessions.json")) {
		log.Printf("已从快照恢复会话 (data=%s)", persistDir)
	}
	srv.SetSessionStore(sessStore)
	srv.SetScheduleStore(server.NewScheduleStore())
	srv.SetCredentialStore(server.NewCredentialStore())
	srv.SetWorkspaceProvider(server.NewWorkspaceProvider())

	// Seed the workspace registry: -workspace flags first, then the shared
	// provider config's workspaces section, then YAML workspaces. Later
	// entries override earlier names (Provider.Open replaces bindings).
	var seeds []server.WorkspaceSeed
	seeds = append(seeds, wsFlags.seeds...)
	seeds = append(seeds, seedsFromConfig...)
	if loadedConfig != nil {
		for name, root := range loadedConfig.Workspaces {
			seeds = append(seeds, server.WorkspaceSeed{Name: name, Root: root})
		}
	}
	srv.SeedWorkspaces(seeds)
	msgStore := server.NewMessageStore()
	msgStore.SetPersistence(filepath.Join(persistDir, "messages.json"))
	srv.SetMessageStore(msgStore)

	// C-10: Skill Hub store (backed by action.ActionRegistry). Skills are
	// installed by Go-side registration via SkillStore.Install; see the
	// server tests for an example.
	srv.SetSkillStore(server.NewSkillStore())

	// Graceful shutdown on SIGINT/SIGTERM
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("InferGlow server starting on %s", cfg.Addr)
		if err := srv.Start(); err != nil {
			log.Printf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
		os.Exit(1)
	}
	srv.ShutdownTerminals()

	fmt.Println("Server stopped gracefully")
}

// agentStoreGroup merges several AgentStore implementations into one listing
// (e.g. config-derived agents + the demo echo agent).
type agentStoreGroup struct {
	stores []server.AgentStore
}

func (g *agentStoreGroup) Get(id string) server.AgentLike {
	for _, st := range g.stores {
		if a := st.Get(id); a != nil {
			return a
		}
	}
	return nil
}

func (g *agentStoreGroup) List() []server.AgentLike {
	var out []server.AgentLike
	for _, st := range g.stores {
		out = append(out, st.List()...)
	}
	return out
}

func (g *agentStoreGroup) Create(_ server.AgentConfig) (string, error) {
	return "", fmt.Errorf("grouped agent store is read-only")
}

func (g *agentStoreGroup) Delete(id string) error {
	for _, st := range g.stores {
		if st.Get(id) != nil {
			return st.Delete(id)
		}
	}
	return nil
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range split(s, ',') {
		if part = trim(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func split(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

// demoAgent is a local echo agent for GUI testing without a real provider.
// It implements server.AgentLike (Run) and is wired by -demo-agent.
type demoAgent struct{}

// Run echoes the user message back, prefixed for visibility.
func (demoAgent) Run(_ context.Context, userMessage string) (string, error) {
	return "demo echo: " + userMessage, nil
}

// demoAgentRecord carries the agent's identity through /v1/agents JSON
// (AgentLike has no metadata fields; without it the demo agent marshals
// as an empty object and clients cannot resolve its id).
type demoAgentRecord struct {
	demoAgent
	ID   string `json:"id"`
	Name string `json:"name"`
}

// demoAgentStore is an in-memory AgentStore holding the demo agent as "a1".
// It satisfies server.AgentStore so the GUI chat endpoints resolve an agent.
type demoAgentStore struct {
	agents map[string]server.AgentLike
}

func newDemoAgentStore() *demoAgentStore {
	return &demoAgentStore{agents: map[string]server.AgentLike{
		"a1": demoAgentRecord{demoAgent: demoAgent{}, ID: "a1", Name: "Demo Echo"},
	}}
}

// Get returns the agent with the given ID, or nil if not found.
func (s *demoAgentStore) Get(id string) server.AgentLike {
	return s.agents[id]
}

// List returns all agents.
func (s *demoAgentStore) List() []server.AgentLike {
	out := make([]server.AgentLike, 0, len(s.agents))
	for _, a := range s.agents {
		out = append(out, a)
	}
	return out
}

// Create registers a new demo agent and returns its ID.
func (s *demoAgentStore) Create(_ server.AgentConfig) (string, error) {
	return "", fmt.Errorf("demo agent store is read-only")
}

// Delete removes an agent by ID.
func (s *demoAgentStore) Delete(id string) error {
	delete(s.agents, id)
	return nil
}
