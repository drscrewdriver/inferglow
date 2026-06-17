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
	cfg        Config
	mux        *http.ServeMux
	httpServer *http.Server
	agentStore AgentStore
	tenantMgr  *TenantManager
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
	Name         string `json:"name"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt,omitempty"`
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

// NewServer creates a new REST API server.
func NewServer(cfg Config, agentStore AgentStore) *Server {
	s := &Server{
		cfg:        cfg,
		mux:        http.NewServeMux(),
		agentStore: agentStore,
		tenantMgr:  NewTenantManager(),
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

	return s.httpServer.Serve(ln)
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}
