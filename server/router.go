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

	// GUI — Desktop GUI (openhanako style, embedded in webui/, no auth)
	s.mux.HandleFunc("GET /gui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/gui/", http.StatusMovedPermanently)
	})
	s.mux.HandleFunc("GET /gui/{path...}", s.handleGUI)

	// Web UI — Browser Web UI (DeepSeek Harness style, embedded in webbrowser/, no auth)
	s.mux.HandleFunc("GET /web", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/web/", http.StatusMovedPermanently)
	})
	s.mux.HandleFunc("GET /web/{path...}", s.handleWebUI)

	// WebUI2 — 原型重构布局（embedded in webui2/, 独立于 /gui/ 与 /web/, no auth）
	s.mux.HandleFunc("GET /webui2", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/webui2/", http.StatusMovedPermanently)
	})
	s.mux.HandleFunc("GET /webui2/{path...}", s.handleWebUI2)

	// WebUI DSH — vendored dsh-transition-webui 整合版（embedded in webui-dsh/,
	// 与 /web/ 双挂载渐进替换, no auth）
	s.mux.HandleFunc("GET /webui-dsh", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/webui-dsh/", http.StatusMovedPermanently)
	})
	s.mux.HandleFunc("GET /webui-dsh/{path...}", s.handleWebUIDsh)

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
	api.HandleFunc("GET /v1/sessions/{id}/trace", s.handleGetSessionTrace)
	api.HandleFunc("GET /v1/sessions/{id}/messages", s.handleListSessionMessages)
	api.HandleFunc("POST /v1/sessions/{id}/fork", s.handleSessionFork)

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
	api.HandleFunc("PATCH /v1/runs/{id}/queue", s.handleRunQueue)
	api.HandleFunc("GET /v1/runs/{id}/jobs", s.handleRunJobs)
	api.HandleFunc("POST /v1/runs/{id}/input", s.handleRunInput)

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
	api.HandleFunc("GET /v1/context/modes", s.handleContextModes)
	api.HandleFunc("GET /v1/context/search", s.handleContextSearch)
	api.HandleFunc("GET /v1/context/stats", s.handleContextStats)

	// Approval records (enabled by SetApprovalManager)
	api.HandleFunc("GET /v1/approvals", s.handleListApprovals)
	api.HandleFunc("POST /v1/approvals", s.handleSubmitApproval)
	api.HandleFunc("POST /v1/approvals/{id}/decision", s.handleApprovalDecision)

	// Usage aggregation report
	api.HandleFunc("GET /v1/usage/report", s.handleUsageReport)

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

	// Workspace file management (Spec B) — /v1/workspace/* and /v1/fs/* aliases
	for _, prefix := range []string{"/v1/workspace", "/v1/fs"} {
		api.HandleFunc("GET "+prefix+"/tree", s.handleWorkspaceTree)
		api.HandleFunc("GET "+prefix+"/read", s.handleWorkspaceRead)
		api.HandleFunc("POST "+prefix+"/write", s.handleWorkspaceWrite)
		api.HandleFunc("POST "+prefix+"/rename", s.handleWorkspaceRename)
		api.HandleFunc("POST "+prefix+"/delete", s.handleWorkspaceDelete)
		api.HandleFunc("GET "+prefix+"/search", s.handleWorkspaceSearch)
		api.HandleFunc("POST "+prefix+"/upload", s.handleWorkspaceUpload)
	}

	// Sandbox (Spec B)
	api.HandleFunc("GET /v1/sandbox/runtimes", s.handleListSandboxRuntimes)
	api.HandleFunc("GET /v1/sandbox/presets", s.handleSandboxPresets)
	api.HandleFunc("POST /v1/sandbox/preset", s.handleSandboxSetPreset)
	api.HandleFunc("GET /v1/sandbox/rejections", s.handleListSandboxRejections)
	api.HandleFunc("POST /v1/sandbox/rejections", s.handleRecordSandboxRejection)

	// Background jobs (Spec B)
	api.HandleFunc("GET /v1/jobs", s.handleListJobs)
	api.HandleFunc("GET /v1/jobs/stream", s.handleJobsStream)
	api.HandleFunc("GET /v1/jobs/{id}", s.handleGetJob)

	// Git (Spec B)
	api.HandleFunc("GET /v1/git/status", s.handleGitStatus)
	api.HandleFunc("GET /v1/git/diff", s.handleGitDiff)
	api.HandleFunc("GET /v1/git/log", s.handleGitLog)
	api.HandleFunc("GET /v1/git/branches", s.handleGitBranches)
	api.HandleFunc("GET /v1/git/worktrees", s.handleGitWorktrees)
	api.HandleFunc("POST /v1/git/commit", s.handleGitCommit)
	api.HandleFunc("POST /v1/git/stage", s.handleGitStage)
	api.HandleFunc("POST /v1/git/reset", s.handleGitReset)
	api.HandleFunc("POST /v1/git/checkout", s.handleGitCheckout)

	// Tasks — webui 待办 + model task_tracker tools share one store (R8)
	api.HandleFunc("GET /v1/tasks", s.handleListTasks)
	api.HandleFunc("POST /v1/tasks", s.handleCreateTask)
	api.HandleFunc("PATCH /v1/tasks/{id}", s.handlePatchTask)
	api.HandleFunc("DELETE /v1/tasks/{id}", s.handleDeleteTask)

	// R9 Phase 0: sub-agent spawn observation (任务管理 panel data source).
	api.HandleFunc("GET /v1/subagents", s.handleListSubagents)

	// R9: workspace registry rename (binding name change, root preserved).
	api.HandleFunc("PATCH /v1/workspaces/{id}", s.handleRenameWorkspace)

	// Produced files (Spec B)
	api.HandleFunc("GET /v1/produced-files", s.handleProducedFiles)

	// Exec — webui terminal (v1: one command per request, allowlisted).
	// Fail-closed: the route exists only when BOTH an API key and the -exec
	// switch are configured (no key / no flag ⇒ 404, not 401).
	if s.cfg.ExecEnabled && (s.cfg.APIKey != "" || s.cfg.AuthOpen) {
		api.HandleFunc("POST /v1/exec", s.handleExec)
	}

	// PTY — webui terminal v2: one persistent interactive shell per
	// workspace, bridged over WebSocket. Full-permission by nature, so it
	// carries its own gate (-pty) on top of the API key; fail-closed 404
	// when either is missing. Registered directly on the mux (more specific
	// than the "/" middleware chain) because a browser WebSocket cannot set
	// the Authorization header — APIKeyAuthQuery also accepts ?token=.
	if s.cfg.PTYEnabled && (s.cfg.APIKey != "" || s.cfg.AuthOpen) {
		authed := http.HandlerFunc(s.handlePtyWS)
		if !s.cfg.AuthOpen {
			authed = middleware.APIKeyAuthQuery(authed, s.cfg.APIKey).(http.HandlerFunc)
		}
		s.mux.Handle("GET /v1/pty", authed)
	}

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
	if s.cfg.APIKey != "" && !s.cfg.AuthOpen {
		h = middleware.APIKeyAuth(h, s.cfg.APIKey)
	}

	s.mux.Handle("/", h)
}
