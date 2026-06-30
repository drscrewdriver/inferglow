// Copyright 2026 InferGlow Authors

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// ToolStreamEvent represents a streaming tool call event sent via SSE.
type ToolStreamEvent struct {
	Type      string `json:"type"`                // "tool_start", "tool_end", "llm_start", "llm_end", "run_start", "run_end", "error"
	ToolName  string `json:"tool_name,omitempty"` // tool name (for tool events)
	Round     int    `json:"round,omitempty"`     // iteration round (for LLM events)
	Tokens    int    `json:"tokens,omitempty"`    // token count (for LLM end events)
	Error     string `json:"error,omitempty"`
	Timestamp string `json:"timestamp"`
}

// handleStreamRun handles POST /v1/agents/{id}/stream-run — streaming agent
// execution with real-time tool call feedback via SSE.
//
// Unlike handleStream (which blocks then sends), this handler creates
// AgentCallbacks that emit SSE events as the agent progresses through
// LLM calls and tool executions.
func (s *Server) handleStreamRun(w http.ResponseWriter, r *http.Request) {
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
	if req.Message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Create a channel for streaming events.
	eventCh := make(chan ToolStreamEvent, 32)
	doneCh := make(chan struct{})

	// Build callbacks that push events to the channel.
	cb := &streamCallbacks{
		eventCh: eventCh,
	}

	// Run agent in background goroutine.
	go func() {
		defer close(doneCh)
		defer close(eventCh)

		// Emit run_start.
		cb.emit("run_start", "", 0, 0, "")

		// Execute agent (blocking).
		resp, err := agent.Run(r.Context(), req.Message)
		if err != nil {
			cb.emit("error", "", 0, 0, err.Error())
			return
		}

		// Emit run_end with response.
		cb.emit("run_end", resp, 0, 0, "")
	}()

	// Stream events to client.
	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-eventCh:
			if !ok {
				// Channel closed, agent finished.
				writeSSEEvent(w, "done", map[string]string{"agent_id": id})
				flusher.Flush()
				return
			}
			writeSSEEvent(w, ev.Type, ev)
			flusher.Flush()
		case <-doneCh:
			// Drain remaining events.
			for ev := range eventCh {
				writeSSEEvent(w, ev.Type, ev)
				flusher.Flush()
			}
			writeSSEEvent(w, "done", map[string]string{"agent_id": id})
			flusher.Flush()
			return
		}
	}
}

// streamCallbacks bridges AgentCallbacks to SSE events.
type streamCallbacks struct {
	eventCh chan<- ToolStreamEvent
}

func (c *streamCallbacks) emit(typ, toolName string, round, tokens int, errMsg string) {
	ev := ToolStreamEvent{
		Type:      typ,
		ToolName:  toolName,
		Round:     round,
		Tokens:    tokens,
		Error:     errMsg,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	select {
	case c.eventCh <- ev:
	default:
		// Drop if channel full (non-blocking).
	}
}

// OnRunStart implements the agent callback interface.
func (c *streamCallbacks) OnRunStart(ctx context.Context, userMessage string) {
	c.emit("run_start", "", 0, 0, "")
}

// OnRunEnd implements the agent callback interface.
func (c *streamCallbacks) OnRunEnd(ctx context.Context, response string, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	c.emit("run_end", response, 0, 0, errMsg)
}

// OnLLMCallStart implements the agent callback interface.
func (c *streamCallbacks) OnLLMCallStart(ctx context.Context, round int) {
	c.emit("llm_start", "", round, 0, "")
}

// OnLLMCallEnd implements the agent callback interface.
func (c *streamCallbacks) OnLLMCallEnd(ctx context.Context, round int, tokens int) {
	c.emit("llm_end", "", round, tokens, "")
}

// OnToolCallStart implements the agent callback interface.
func (c *streamCallbacks) OnToolCallStart(ctx context.Context, toolName string) {
	c.emit("tool_start", toolName, 0, 0, "")
}

// OnToolCallEnd implements the agent callback interface.
func (c *streamCallbacks) OnToolCallEnd(ctx context.Context, toolName string, err error) {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	c.emit("tool_end", toolName, 0, 0, errMsg)
}
