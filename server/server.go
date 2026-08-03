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

package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/inferglow/flow/stage"
	"github.com/inferglow/messagebus"
	"github.com/inferglow/observability"
	"github.com/inferglow/server/trigger"
)

// Config holds server configuration.
type Config struct {
	Addr         string        // Listen address, e.g. ":8080"
	ReadTimeout  time.Duration // Per-request read timeout
	WriteTimeout time.Duration // Per-request write timeout
	IdleTimeout  time.Duration // Keep-alive idle timeout
	APIKey       string        // Optional Bearer token for auth (empty = disabled)
	CORSOrigins  []string      // Allowed CORS origins (empty = disabled)
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		Addr:         ":8080",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

// Server is the InferGlow REST API server.
type Server struct {
	cfg            Config
	mux            *http.ServeMux
	httpServer     *http.Server
	agentStore     AgentStore
	tenantMgr      *TenantManager
	flowStore      *FlowStore
	runMgr         *RunManager
	triggerReg     *trigger.Registry
	memStore       MemoryStore
	teamStore      *TeamStore
	teamRunner     *TeamRunner
	ctxProvider    ContextProvider
	spanCollector  *observability.SpanCollector // OT-13
	resourcePolicy ResourceAccessPolicy
	bus            messagebus.MessageBus
	sessionStore   *SessionStore
	scheduleStore  *ScheduleStore
	credStore      *CredentialStore
	wsProvider     WorkspaceProvider
	skillStore     *SkillStore
	kbStore        *KBStore
	mcpHubStore    *MCPHubStore
}

// MemoryRecord represents a persistent memory entry.
type MemoryRecord struct {
	ID       string   `json:"id"`
	Content  string   `json:"content"`
	Category string   `json:"category,omitempty"`
	Facts    []string `json:"facts,omitempty"`
}

// MemoryStore is the interface for persistent memory operations.
// Implementations include in-memory, JSONL, SQLite, etc.
type MemoryStore interface {
	UpsertMemory(rec MemoryRecord) error
	GetMemory(id string) (*MemoryRecord, error)
	SearchMemory(query string, category string, limit int) ([]MemoryRecord, error)
	DeleteMemory(id string) error
}

// AgentStore provides access to agents by ID.
type AgentStore interface {
	// Get returns an agent by ID, or nil if not found.
	Get(id string) AgentLike
	// List returns all agents.
	List() []AgentLike
	// Create creates a new agent and returns its ID.
	Create(cfg AgentConfig) (string, error)
	// Delete removes an agent by ID.
	Delete(id string) error
}

// AgentLike is the subset of agent.Agent needed by the server.
type AgentLike interface {
	Run(ctx context.Context, userMessage string) (string, error)
}

// AgentConfig holds the configuration for creating a new agent.
type AgentConfig struct {
	Name           string `json:"name" validate:"required"`
	Model          string `json:"model" validate:"required"`
	SystemPrompt   string `json:"system_prompt,omitempty"`
	MemoryStrategy string `json:"memory_strategy,omitempty"` // "token_buffer" | "summary" | "" (default)
}

// TenantManager handles multi-tenant isolation and usage tracking.
type TenantManager struct {
	tenants map[string]*Tenant
}

// Tenant represents a single tenant.
type Tenant struct {
	ID       string
	APIKey   string
	Agents   map[string]bool // agent IDs owned by this tenant
	MaxRPM   int             // max requests per minute
	reqCount int
}

// NewTenantManager creates a new tenant manager.
func NewTenantManager() *TenantManager {
	return &TenantManager{
		tenants: make(map[string]*Tenant),
	}
}

// RegisterTenant adds a new tenant.
func (tm *TenantManager) RegisterTenant(id, apiKey string, maxRPM int) {
	tm.tenants[id] = &Tenant{
		ID:     id,
		APIKey: apiKey,
		Agents: make(map[string]bool),
		MaxRPM: maxRPM,
	}
}

// Authenticate validates the API key and returns the tenant.
func (tm *TenantManager) Authenticate(apiKey string) (*Tenant, error) {
	for _, t := range tm.tenants {
		if t.APIKey == apiKey {
			return t, nil
		}
	}
	return nil, fmt.Errorf("invalid API key")
}

// UserIdentity is a minimal marker for a resource access principal. It exists
// so TenantManager establishes *who* a caller is while ResourceAccessPolicy
// decides *what* that identity may see. The identity model is intentionally
// left open for later contract extensions (C-4+).
type UserIdentity interface {
	ID() string
}

// ResourceAccessPolicy is the resource-level authorization contract (spec
// C-2). It decides whether userID may perform action on resource resourceType
// resourceID. A concrete implementation may bridge the existing
// security/rbac (roles & permission matrix) and security/pii (redaction)
// layers; this work package neither builds an RBAC engine nor wires it into the
// request hot path.
type ResourceAccessPolicy interface {
	CanAccess(ctx context.Context, userID, resourceType, resourceID, action string) bool
}

// DefaultResourceAccessPolicy permits everything. It is the zero-config
// behaviour when no policy is injected, preserving existing semantics.
type DefaultResourceAccessPolicy struct{}

// CanAccess always grants access.
func (DefaultResourceAccessPolicy) CanAccess(_ context.Context, _, _, _, _ string) bool {
	return true
}

// NewServer creates a new REST API server.
func NewServer(cfg Config, agentStore AgentStore) *Server {
	stages := stage.NewRegistry()
	flowStore := NewFlowStore(stages)
	runMgr := NewRunManager(flowStore)
	starter := trigger.StarterFunc(func(flowName string, inputs map[string]any, owner string) (trigger.RunHandle, error) {
		return runMgr.Start(flowName, inputs, owner)
	})
	s := &Server{
		cfg:        cfg,
		mux:        http.NewServeMux(),
		agentStore: agentStore,
		tenantMgr:  NewTenantManager(),
		flowStore:  flowStore,
		runMgr:     runMgr,
		triggerReg: trigger.NewRegistry(starter),
	}
	s.registerRoutes()
	return s
}

// NewServerWithFlows creates a server with a pre-configured FlowStore.
func NewServerWithFlows(cfg Config, agentStore AgentStore, flowStore *FlowStore) *Server {
	runMgr := NewRunManager(flowStore)
	starter := trigger.StarterFunc(func(flowName string, inputs map[string]any, owner string) (trigger.RunHandle, error) {
		return runMgr.Start(flowName, inputs, owner)
	})
	s := &Server{
		cfg:        cfg,
		mux:        http.NewServeMux(),
		agentStore: agentStore,
		tenantMgr:  NewTenantManager(),
		flowStore:  flowStore,
		runMgr:     runMgr,
		triggerReg: trigger.NewRegistry(starter),
	}
	s.registerRoutes()
	return s
}

// Handler returns the HTTP handler (for testing or custom wrapping).
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Start begins listening. It blocks until the server shuts down.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.Addr, err)
	}

	s.httpServer = &http.Server{
		Handler:      s.mux,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
		IdleTimeout:  s.cfg.IdleTimeout,
	}

	// Start all enabled triggers.
	_ = s.triggerReg.StartAll(context.Background())

	return s.httpServer.Serve(ln)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	// Stop all triggers first.
	_ = s.triggerReg.StopAll()

	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

// SetContextFactory sets the factory for creating Context in runs.
// This enables stage functions to call LLM via fctx.GenerateModel().
func (s *Server) SetContextFactory(factory ContextFactory) {
	s.runMgr.SetContextFactory(factory)
}

// SetMemoryStore sets the persistent memory backend.
// When set, the /v1/memories endpoints use this store for CRUD operations.
func (s *Server) SetMemoryStore(store MemoryStore) {
	s.memStore = store
}

// SetSessionEndHook sets a hook called after each successful flow run.
// Typical use: inject LongMemPromoter.OnSessionEnd for auto-promotion.
func (s *Server) SetSessionEndHook(hook SessionEndHook) {
	s.runMgr.SetSessionEndHook(hook)
}

// ContextProvider is a lightweight interface for context/semantic search.
// Implementations wrap the full context.ContextManager without pulling
// the entire context module dependency into the server package.
type ContextProvider interface {
	// Search performs a semantic or keyword search.
	Search(ctx context.Context, query string, limit int, scope string) ([]ContextHit, error)
	// Stats returns context subsystem statistics.
	Stats() map[string]any
}

// ContextHit is a single search result from ContextProvider.
type ContextHit struct {
	Content   string  `json:"content"`
	Score     float64 `json:"score"`
	Source    string  `json:"source,omitempty"`
	SessionID string  `json:"session_id,omitempty"`
}

// SetTeamCoordinator initializes the team coordination subsystem.
// This enables the /v1/teams endpoints.
func (s *Server) SetTeamCoordinator(agentStore AgentStore) {
	s.teamStore = NewTeamStore()
	s.teamRunner = NewTeamRunner(agentStore, s.teamStore)
}

// SetContextProvider sets the context/semantic search provider.
// This enables the /v1/context/search and /v1/context/stats endpoints.
func (s *Server) SetContextProvider(p ContextProvider) {
	s.ctxProvider = p
}

// SetResourceAccessPolicy injects the resource-level authorization policy
// (spec C-2). When no policy is set the server falls back to
// DefaultResourceAccessPolicy (allow-all), so behaviour is unchanged until a
// concrete policy is wired in.
func (s *Server) SetResourceAccessPolicy(p ResourceAccessPolicy) {
	s.resourcePolicy = p
}

// SetMessageBus attaches the message bus infrastructure (spec C-3) used for
// session streaming, live updates and cross-component events. It is an opt-in
// injection point: nil leaves the server with no active bus until a consumer
// needs one.
func (s *Server) SetMessageBus(b messagebus.MessageBus) {
	s.bus = b
}

// SetSessionStore attaches the C-4 session management store. When set, the
// /v1/sessions endpoints use this store for CRUD and SSE streaming.
func (s *Server) SetSessionStore(st *SessionStore) {
	s.sessionStore = st
}

// SetScheduleStore attaches the C-5 scheduler management store. When set, the
// /v1/schedules endpoints manage persistent and in-memory cron schedules.
func (s *Server) SetScheduleStore(st *ScheduleStore) {
	s.scheduleStore = st
}

// SetCredentialStore attaches the C-6 credential management store. When set,
// the /v1/credentials endpoints operate on it with secret masking.
func (s *Server) SetCredentialStore(st *CredentialStore) {
	s.credStore = st
}

// SetWorkspaceProvider attaches the C-7 workspace provider. When set, the
// /v1/workspaces endpoints manage workspace records.
func (s *Server) SetWorkspaceProvider(p WorkspaceProvider) {
	s.wsProvider = p
}

// SetSkillStore attaches the C-10 Skill Hub store. When set, the /v1/skill-hub
// endpoints list, inspect, remove and execute installed skills (backed by
// action.ActionRegistry).
func (s *Server) SetSkillStore(st *SkillStore) {
	s.skillStore = st
}

// SetKBStore attaches the C-8 Knowledge Base store. When set, the
// /v1/knowledge-bases endpoints create, list, inspect, delete and search
// knowledge bases (reusing the rag module's Loader/Splitter/Store contracts
// through a thin in-memory adapter).
func (s *Server) SetKBStore(st *KBStore) {
	s.kbStore = st
}

// SetMCPHubStore attaches the C-9 MCP Hub store. When set, the /v1/mcp-hub
// endpoints list, install, inspect, remove and call MCP tools (reusing
// mcpserver's ActionRegistryAdapter for discovery and invocation).
func (s *Server) SetMCPHubStore(st *MCPHubStore) {
	s.mcpHubStore = st
}

// SemanticMemoryStore is an optional interface that MemoryStore
// implementations can satisfy to enable semantic search via /v1/memories/search.
type SemanticMemoryStore interface {
	SemanticSearch(ctx context.Context, query string, limit int) ([]MemoryRecord, error)
	MemoryStats() map[string]any
}
