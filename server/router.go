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
	"net/http"

	"github.com/inferglow/server/handler"
	"github.com/inferglow/server/middleware"
)

// registerRoutes sets up all API routes.
func (s *Server) registerRoutes() {
	// Health check (no auth)
	s.mux.HandleFunc("GET /health", handler.Health)

	// API routes (with optional middleware)
	api := http.NewServeMux()

	// Agent CRUD + chat
	api.HandleFunc("GET /v1/agents", s.handleListAgents)
	api.HandleFunc("POST /v1/agents", s.handleCreateAgent)
	api.HandleFunc("GET /v1/agents/{id}", s.handleGetAgent)
	api.HandleFunc("DELETE /v1/agents/{id}", s.handleDeleteAgent)
	api.HandleFunc("POST /v1/agents/{id}/chat", s.handleChat)
	api.HandleFunc("POST /v1/agents/{id}/input", s.handleInput)
	api.HandleFunc("POST /v1/agents/{id}/stream", s.handleStream)
	api.HandleFunc("POST /v1/agents/{id}/stream-run", s.handleStreamRun)

	// Session
	api.HandleFunc("GET /v1/sessions/{id}", s.handleGetSession)

	// Tools
	api.HandleFunc("GET /v1/tools", s.handleListTools)

	// Memory
	api.HandleFunc("POST /v1/memories", s.handleCreateMemory)
	api.HandleFunc("GET /v1/memories", s.handleSearchMemory)
	api.HandleFunc("GET /v1/memories/{id}", s.handleGetMemory)
	api.HandleFunc("DELETE /v1/memories/{id}", s.handleDeleteMemory)

	// Flow management (recycled from inferflow)
	api.HandleFunc("GET /v1/flows", s.handleListFlows)
	api.HandleFunc("GET /v1/flows/{name}", s.handleGetFlow)
	api.HandleFunc("POST /v1/flows", s.handleRegisterFlow)
	api.HandleFunc("POST /v1/flows/{name}/validate", s.handleValidateFlow)

	// Run lifecycle
	api.HandleFunc("POST /v1/runs", s.handleCreateRun)
	api.HandleFunc("GET /v1/runs", s.handleListRuns)
	api.HandleFunc("GET /v1/runs/{id}", s.handleGetRun)
	api.HandleFunc("DELETE /v1/runs/{id}", s.handleCancelRun)
	api.HandleFunc("POST /v1/runs/{id}/pause", s.handlePauseRun)
	api.HandleFunc("POST /v1/runs/{id}/resume", s.handleResumeRun)
	api.HandleFunc("GET /v1/runs/{id}/events", s.handleRunEvents)
	api.HandleFunc("GET /v1/runs/{id}/state", s.handleGetRunState)
	api.HandleFunc("GET /v1/runs/{id}/steps", s.handleGetRunSteps)

	// Stages
	api.HandleFunc("GET /v1/stages", s.handleListStages)

	// Trigger management
	api.HandleFunc("POST /v1/triggers", s.handleCreateTrigger)
	api.HandleFunc("GET /v1/triggers", s.handleListTriggers)
	api.HandleFunc("GET /v1/triggers/{id}", s.handleGetTrigger)
	api.HandleFunc("DELETE /v1/triggers/{id}", s.handleDeleteTrigger)
	api.HandleFunc("POST /v1/triggers/{id}/start", s.handleStartTrigger)
	api.HandleFunc("POST /v1/triggers/{id}/stop", s.handleStopTrigger)

	// Webhook entry point
	api.HandleFunc("POST /v1/webhooks/{id}", s.handleWebhook)

	// Team coordination (enabled by SetTeamCoordinator)
	api.HandleFunc("POST /v1/teams", s.handleCreateTeam)
	api.HandleFunc("GET /v1/teams", s.handleListTeams)
	api.HandleFunc("GET /v1/teams/{id}", s.handleGetTeam)
	api.HandleFunc("DELETE /v1/teams/{id}", s.handleDeleteTeam)
	api.HandleFunc("POST /v1/teams/{id}/run", s.handleTeamRun)
	api.HandleFunc("POST /v1/teams/{id}/stream", s.handleTeamStream)

	// Context / semantic search (enabled by SetContextProvider)
	api.HandleFunc("GET /v1/context/search", s.handleContextSearch)
	api.HandleFunc("GET /v1/context/stats", s.handleContextStats)

	// Enhanced memory endpoints
	api.HandleFunc("POST /v1/memories/search", s.handleMemorySemanticSearch)
	api.HandleFunc("GET /v1/memories/stats", s.handleMemoryStats)

	// Agent status (TurnLoop phase)
	api.HandleFunc("GET /v1/agents/{id}/status", s.handleAgentStatus)

	// Audit chain verification and entry retrieval
	api.HandleFunc("GET /v1/audit/verify", s.handleAuditVerify)
	api.HandleFunc("GET /v1/audit/entries", s.handleAuditEntries)

	// OpenAPI spec
	api.HandleFunc("GET /openapi.json", handler.OpenAPISpec)

	// Apply middleware chain
	var h http.Handler = api
	h = middleware.Logging(h)
	h = middleware.Recovery(h)
	if len(s.cfg.CORSOrigins) > 0 {
		h = middleware.CORS(h, s.cfg.CORSOrigins)
	}
	if s.cfg.APIKey != "" {
		h = middleware.APIKeyAuth(h, s.cfg.APIKey)
	}

	s.mux.Handle("/", h)
}
