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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	agentpkg "github.com/inferglow/orchestrator/agent"
)

// handleListAgents returns all agents.
func (s *Server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents := s.agentStore.List()
	writeJSON(w, http.StatusOK, map[string]any{
		"agents": agents,
		"count":  len(agents),
	})
}

// handleCreateAgent creates a new agent.
func (s *Server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	var cfg AgentConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if err := validate.Struct(cfg); err != nil {
		writeError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	id, err := s.agentStore.Create(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":   id,
		"name": cfg.Name,
	})
}

// handleGetAgent returns an agent by ID.
func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent := s.agentStore.Get(id)
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":     id,
		"agent":  agent,
	})
}

// handleDeleteAgent removes an agent.
func (s *Server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.agentStore.Delete(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ChatRequest is the request body for agent chat.
type ChatRequest struct {
	Message     string `json:"message" validate:"required"`
	PreemptMode string `json:"preempt_mode,omitempty"` // "queue"|"safe_point"|"force"
}

// ChatResponse is the response body for agent chat.
type ChatResponse struct {
	Response string `json:"response"`
	AgentID  string `json:"agent_id"`
}

// AgentInputter is the subset of agent.Agent needed by the /input endpoint.
type AgentInputter interface {
	SubmitInput(ctx context.Context, msg string, mode agentpkg.PreemptMode) (<-chan agentpkg.InputResponse, error)
}

// parsePreemptMode converts a string mode to the agent PreemptMode type.
func parsePreemptMode(s string) agentpkg.PreemptMode {
	switch s {
	case "queue":
		return agentpkg.PreemptQueue
	case "safe_point":
		return agentpkg.PreemptSafePoint
	case "force":
		return agentpkg.PreemptForce
	default:
		return agentpkg.PreemptQueue
	}
}

// handleChat performs a synchronous agent chat.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent := s.agentStore.Get(id)
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	// When preempt_mode is specified and the agent supports SubmitInput,
	// use the input queue path.
	if req.PreemptMode != "" {
		if inputter, ok := agent.(AgentInputter); ok {
			mode := parsePreemptMode(req.PreemptMode)
			ch, err := inputter.SubmitInput(r.Context(), req.Message, mode)
			if err != nil {
				if errors.Is(err, agentpkg.ErrQueueFull) {
					w.Header().Set("Retry-After", "5")
					writeError(w, http.StatusTooManyRequests, "input queue is full")
					return
				}
				writeError(w, http.StatusInternalServerError, "agent error: "+err.Error())
				return
			}
			resp := <-ch
			if resp.Error != nil {
				writeError(w, http.StatusInternalServerError, "agent error: "+resp.Error.Error())
				return
			}
			writeJSON(w, http.StatusOK, ChatResponse{
				Response: resp.Response,
				AgentID:  id,
			})
			return
		}
		// Fall through to synchronous Run if agent does not support SubmitInput.
	}

	resp, err := agent.Run(r.Context(), req.Message)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "agent error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, ChatResponse{
		Response: resp,
		AgentID:  id,
	})
}

// handleStream performs a streaming agent chat (SSE).
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent := s.agentStore.Get(id)
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Execute agent and stream response
	resp, err := agent.Run(r.Context(), req.Message)
	if err != nil {
		// Send error as SSE event
		writeSSEEvent(w, "error", map[string]string{"error": err.Error()})
		flusher.Flush()
		return
	}

	// Send response as SSE events
	writeSSEEvent(w, "delta", map[string]string{"content": resp})
	writeSSEEvent(w, "done", map[string]string{"agent_id": id})
	flusher.Flush()
}

// handleInput processes user input with preempt mode support.
// POST /v1/agents/{id}/input
func (s *Server) handleInput(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent := s.agentStore.Get(id)
	if agent == nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	inputter, ok := agent.(AgentInputter)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "agent does not support input queue")
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if err := validate.Struct(req); err != nil {
		writeError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	mode := parsePreemptMode(req.PreemptMode)
	ch, err := inputter.SubmitInput(r.Context(), req.Message, mode)
	if err != nil {
		if errors.Is(err, agentpkg.ErrQueueFull) {
			w.Header().Set("Retry-After", "5")
			writeError(w, http.StatusTooManyRequests, "input queue is full")
			return
		}
		writeError(w, http.StatusAccepted, "agent error: "+err.Error())
		return
	}

	// Block until the agent processes the input.
	resp := <-ch
	if resp.Error != nil {
		writeError(w, http.StatusInternalServerError, "agent error: "+resp.Error.Error())
		return
	}
	writeJSON(w, http.StatusOK, ChatResponse{
		Response: resp.Response,
		AgentID:  id,
	})
}

// handleListTools returns available tools (stub).
func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"tools": []any{},
		"count": 0,
	})
}

// handleCreateMemory creates a memory entry.
func (s *Server) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	if s.memStore == nil {
		writeError(w, http.StatusServiceUnavailable, "memory store not configured")
		return
	}
	var req struct {
		ID       string   `json:"id"`
		Content  string   `json:"content"`
		Category string   `json:"category,omitempty"`
		Facts    []string `json:"facts,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if req.Content == "" && len(req.Facts) == 0 {
		writeError(w, http.StatusBadRequest, "content or facts required")
		return
	}
	if req.ID == "" {
		req.ID = fmt.Sprintf("mem-%d", time.Now().UnixNano())
	}

	rec := MemoryRecord{
		ID:       req.ID,
		Content:  req.Content,
		Category: req.Category,
		Facts:    req.Facts,
	}
	if err := s.memStore.UpsertMemory(rec); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": rec.ID, "status": "created"})
}

// handleSearchMemory searches memories.
func (s *Server) handleSearchMemory(w http.ResponseWriter, r *http.Request) {
	if s.memStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"memories": []any{}, "count": 0})
		return
	}
	q := r.URL.Query().Get("q")
	category := r.URL.Query().Get("category")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	results, err := s.memStore.SearchMemory(q, category, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"memories": results,
		"count":    len(results),
	})
}

// handleGetMemory returns a single memory by ID.
func (s *Server) handleGetMemory(w http.ResponseWriter, r *http.Request) {
	if s.memStore == nil {
		writeError(w, http.StatusServiceUnavailable, "memory store not configured")
		return
	}
	id := r.PathValue("id")
	rec, err := s.memStore.GetMemory(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// handleDeleteMemory removes a memory by ID.
func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	if s.memStore == nil {
		writeError(w, http.StatusServiceUnavailable, "memory store not configured")
		return
	}
	id := r.PathValue("id")
	if err := s.memStore.DeleteMemory(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// writeSSEEvent writes a Server-Sent Event.
func writeSSEEvent(w http.ResponseWriter, event string, data any) {
	b, _ := json.Marshal(data)
	w.Write([]byte("event: " + event + "\n"))
	w.Write([]byte("data: "))
	w.Write(b)
	w.Write([]byte("\n\n"))
}
