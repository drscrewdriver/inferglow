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
	"syscall"
	"time"

	"github.com/inferglow/messagebus"
	"github.com/inferglow/observability"
	"github.com/inferglow/server"
	"github.com/inferglow/server/config"
)

func main() {
	var (
		addr       = flag.String("addr", ":8080", "Listen address")
		apiKey     = flag.String("api-key", "", "API key for Bearer auth (empty = disabled)")
		cors       = flag.String("cors", "", "Comma-separated CORS origins (empty = disabled)")
		timeout    = flag.Duration("timeout", 30*time.Second, "Request read timeout")
		usageDir   = flag.String("usage-dir", "data", "Directory holding sessions/*.usage.jsonl")
		demoAgent  = flag.Bool("demo-agent", false, "Wire a local echo agent (id a1) so the GUI chat can be exercised without a real model provider")
		configPath = flag.String("config", "", "Server YAML config: llm.providers are wired as real chat agents (one per provider, pure-chat)")
	)
	flag.Parse()

	cfg := server.DefaultConfig()
	cfg.Addr = *addr
	cfg.APIKey = *apiKey
	cfg.ReadTimeout = *timeout
	cfg.UsageDataDir = *usageDir

	if *cors != "" {
		cfg.CORSOrigins = splitComma(*cors)
	}

	var agentStore server.AgentStore
	if *configPath != "" {
		scfg, err := config.NewLoader(*configPath).Load()
		if err != nil {
			log.Fatalf("load config %s: %v", *configPath, err)
		}
		if len(scfg.LLM.Providers) > 0 {
			store, err := server.NewConfigAgentStore(scfg.LLM)
			if err != nil {
				log.Fatalf("wire real agents: %v", err)
			}
			agentStore = store
			log.Printf("real agents wired from config: %d provider(s)", len(scfg.LLM.Providers))
		}
	}
	if *demoAgent {
		if agentStore != nil {
			log.Println("-demo-agent ignored: real agents already wired from config")
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
	srv.SetSessionStore(server.NewSessionStore())
	srv.SetScheduleStore(server.NewScheduleStore())
	srv.SetCredentialStore(server.NewCredentialStore())
	srv.SetWorkspaceProvider(server.NewWorkspaceProvider())
	srv.SetMessageStore(server.NewMessageStore())

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

	fmt.Println("Server stopped gracefully")
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
