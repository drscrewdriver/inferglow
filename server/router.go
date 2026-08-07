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

	// Dashboard (OT-13, no auth for dev convenience)
	s.mux.HandleFunc("GET /dashboard", s.handleDashboard)

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

	// Session management (C-4)
	api.HandleFunc("POST /v1/sessions", s.handleCreateSession)
	api.HandleFunc("GET /v1/sessions", s.handleListSessions)
	api.HandleFunc("GET /v1/sessions/{id}", s.handleGetSession)
	api.HandleFunc("PATCH /v1/sessions/{id}", s.handleUpdateSession)
	api.HandleFunc("DELETE /v1/sessions/{id}", s.handleDeleteSession)
	api.HandleFunc("GET /v1/sessions/{id}/stream", s.handleSessionStream)
	api.HandleFunc("GET /v1/sessions/{id}/messages", s.handleListSessionMessages)

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

	// Observability (OT-13)
	api.HandleFunc("GET /v1/observability/spans", s.handleObservabilitySpans)
	api.HandleFunc("GET /v1/observability/stats", s.handleObservabilityStats)

	// Scheduler management (C-5)
	api.HandleFunc("POST /v1/schedules", s.handleCreateSchedule)
	api.HandleFunc("GET /v1/schedules", s.handleListSchedules)
	api.HandleFunc("GET /v1/schedules/{id}", s.handleGetSchedule)
	api.HandleFunc("DELETE /v1/schedules/{id}", s.handleDeleteSchedule)
	api.HandleFunc("POST /v1/schedules/{id}/start", s.handleStartSchedule)
	api.HandleFunc("POST /v1/schedules/{id}/stop", s.handleStopSchedule)

	// Credential management (C-6)
	api.HandleFunc("POST /v1/credentials", s.handleCreateCredential)
	api.HandleFunc("GET /v1/credentials", s.handleListCredentials)
	api.HandleFunc("GET /v1/credentials/{id}", s.handleGetCredential)
	api.HandleFunc("DELETE /v1/credentials/{id}", s.handleDeleteCredential)

	// Workspace management (C-7)
	api.HandleFunc("POST /v1/workspaces", s.handleCreateWorkspace)
	api.HandleFunc("GET /v1/workspaces", s.handleListWorkspaces)
	api.HandleFunc("GET /v1/workspaces/{id}", s.handleGetWorkspace)
	api.HandleFunc("DELETE /v1/workspaces/{id}", s.handleDeleteWorkspace)
	api.HandleFunc("GET /v1/workspaces/{id}/files", s.handleListWorkspaceFiles)

	// Skill Hub management (C-10)
	api.HandleFunc("GET /v1/skill-hub", s.handleListSkills)
	api.HandleFunc("GET /v1/skill-hub/{name}", s.handleGetSkill)
	api.HandleFunc("DELETE /v1/skill-hub/{name}", s.handleDeleteSkill)
	api.HandleFunc("POST /v1/skill-hub/{name}/execute", s.handleExecuteSkill)

	// Knowledge Base management (C-8)
	api.HandleFunc("POST /v1/knowledge-bases", s.handleCreateKnowledgeBase)
	api.HandleFunc("GET /v1/knowledge-bases", s.handleListKnowledgeBases)
	api.HandleFunc("GET /v1/knowledge-bases/{name}", s.handleGetKnowledgeBase)
	api.HandleFunc("DELETE /v1/knowledge-bases/{name}", s.handleDeleteKnowledgeBase)
	api.HandleFunc("POST /v1/knowledge-bases/{name}/ingest", s.handleIngestKnowledgeBase)
	api.HandleFunc("POST /v1/knowledge-bases/{name}/search", s.handleSearchKnowledgeBase)

	// MCP Hub management (C-9)
	api.HandleFunc("GET /v1/mcp-hub", s.handleListMCPTools)
	api.HandleFunc("POST /v1/mcp-hub", s.handleInstallMCPTool)
	api.HandleFunc("GET /v1/mcp-hub/{name}", s.handleGetMCPTool)
	api.HandleFunc("DELETE /v1/mcp-hub/{name}", s.handleDeleteMCPTool)
	api.HandleFunc("POST /v1/mcp-hub/{name}/call", s.handleCallMCPTool)

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
