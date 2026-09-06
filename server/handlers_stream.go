// Copyright 2026 InferGlow Authors

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	agentpkg "github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/model"
)

// ToolStreamEvent represents a streaming event sent via SSE.
//
// Event types: run_start, delta, reasoning, llm_start, llm_end, tool_start,
// tool_end, run_end, error, done. delta/reasoning carry the incremental LLM
// output in Delta (never persisted; the full reply arrives at run_end and is
// the persistence contract). llm_end carries the provider-reported Usage.
type ToolStreamEvent struct {
	Type      string           `json:"type"`                // event kind (see above)
	ToolName  string           `json:"tool_name,omitempty"` // tool name (for tool events); full reply on run_end (contract quirk)
	Round     int              `json:"round,omitempty"`     // iteration round (for LLM events)
	Tokens    int              `json:"tokens,omitempty"`    // token count (for LLM end events)
	Error     string           `json:"error,omitempty"`
	Delta     string           `json:"delta,omitempty"`     // incremental text for delta/reasoning
	Usage     *model.UsageInfo `json:"usage,omitempty"`     // provider-reported usage (llm_end)
	Timestamp string           `json:"timestamp"`
}

// CallbacksRunner is implemented by agents able to take per-run options
// (ConfigAgent.RunWithCallbacks forwarding to *agent.Agent's variadic Run).
// A distinct method name is required: a single type cannot provide both
// Run(ctx,msg) and Run(ctx,msg,...opts) under the same name. handleStreamRun
// injects SSE streaming callbacks through it; plain AgentLike agents (demo
// echo) take the legacy no-callbacks path.
type CallbacksRunner interface {
	RunWithCallbacks(ctx context.Context, userMessage string, opts ...agentpkg.RunOption) (string, error)
}

// handleStreamRun handles POST /v1/agents/{id}/stream-run — streaming agent
// execution with real-time token deltas and tool call feedback via SSE.
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

	// SSE timeout fix: lift the absolute write deadline so a long agent run
	// with no events (e.g. a long thinking phase) is not cut off by the
	// server's WriteTimeout after 60s (mirrors handleSessionStream).
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Persist the user message up-front when a session is attached.
	s.recordMessage(req.SessionID, MessageRoleUser, req.Message, "", "")

	// Channel for streaming events; generously buffered so token-delta
	// streams do not stall the agent goroutine on a slow client.
	eventCh := make(chan ToolStreamEvent, 256)
	doneCh := make(chan struct{})

	sc := newStreamCallbacks(eventCh)
	runner, supportsCallbacks := agent.(CallbacksRunner)

	// Run agent in background goroutine. Persistence of tool/assistant
	// messages happens here — after the run settles — NOT in the client
	// write loop, so an aborted or dropped connection no longer truncates
	// the session history.
	go func() {
		defer close(doneCh)
		defer close(eventCh)

		var resp string
		var runErr error
		if supportsCallbacks {
			cbs := mergeCallbacks(sc.agentCallbacks(), persistedCallbacks(agent))
			resp, runErr = runner.RunWithCallbacks(r.Context(), req.Message, agentpkg.WithCallbacks(cbs))
		} else {
			// Demo/legacy agents expose no callback surface.
			sc.emit("run_start", "", 0, 0, "")
			resp, runErr = agent.Run(r.Context(), req.Message)
		}

		if runErr == nil {
			sc.persistMessages(s, req.SessionID, resp)
		}

		if runErr != nil {
			sc.emit("error", "", 0, 0, runErr.Error())
			return
		}
		// Contract quirk kept for compatibility: run_end carries the full
		// assistant reply in its ToolName field.
		sc.emit("run_end", resp, 0, 0, "")
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

// streamCallbacks bridges agentpkg.AgentCallbacks to SSE events and collects
// the settled records persisted once the run completes.
type streamCallbacks struct {
	eventCh chan<- ToolStreamEvent

	mu    sync.Mutex
	tools []toolRecord
}

type toolRecord struct {
	name   string
	status string // "ok" | "error"
}

func newStreamCallbacks(eventCh chan<- ToolStreamEvent) *streamCallbacks {
	return &streamCallbacks{eventCh: eventCh}
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
	c.send(ev)
}

func (c *streamCallbacks) send(ev ToolStreamEvent) {
	select {
	case c.eventCh <- ev:
	default:
		// Drop if channel full (non-blocking); delta streams self-heal at
		// run_end, which carries the full reply for client-side reconcile.
	}
}

// agentCallbacks renders the SSE bridging as an agentpkg.AgentCallbacks for
// injection via WithCallbacks.
func (c *streamCallbacks) agentCallbacks() *agentpkg.AgentCallbacks {
	return &agentpkg.AgentCallbacks{
		OnRunStart: func(ctx context.Context, userMessage string) {
			c.emit("run_start", "", 0, 0, "")
		},
		OnLLMCallStart: func(ctx context.Context, round int) {
			c.emit("llm_start", "", round, 0, "")
		},
		OnLLMCallEnd: func(ctx context.Context, round int, tokens int, usage *model.UsageInfo) {
			c.send(ToolStreamEvent{
				Type:      "llm_end",
				Round:     round,
				Tokens:    tokens,
				Usage:     usage,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			})
		},
		OnToolCallStart: func(ctx context.Context, toolName string) {
			c.emit("tool_start", toolName, 0, 0, "")
		},
		OnToolCallEnd: func(ctx context.Context, toolName string, err error) {
			status := "ok"
			errMsg := ""
			if err != nil {
				status = "error"
				errMsg = err.Error()
			}
			c.mu.Lock()
			c.tools = append(c.tools, toolRecord{name: toolName, status: status})
			c.mu.Unlock()
			c.emit("tool_end", toolName, 0, 0, errMsg)
		},
		OnToken: func(ctx context.Context, delta string) {
			c.send(ToolStreamEvent{
				Type:      "delta",
				Delta:     delta,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			})
		},
		OnReasoning: func(ctx context.Context, delta string) {
			c.send(ToolStreamEvent{
				Type:      "reasoning",
				Delta:     delta,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			})
		},
	}
}

// persistMessages records the settled tool calls and the assistant reply for
// the session. Called once per run from the agent goroutine.
func (c *streamCallbacks) persistMessages(s *Server, sessionID, reply string) {
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	tools := append([]toolRecord(nil), c.tools...)
	c.mu.Unlock()
	for _, t := range tools {
		s.recordMessage(sessionID, MessageRoleTool, "", t.name, t.status)
	}
	if reply != "" {
		s.recordMessage(sessionID, MessageRoleAssistant, reply, "", "")
	}
}

// persistedCallbacks reads the agent's construction-time callbacks so run-level
// injection does not replace them (ConfigAgent promotes *agent.Agent.Callbacks).
func persistedCallbacks(agent AgentLike) *agentpkg.AgentCallbacks {
	type callbacksReader interface{ Callbacks() *agentpkg.AgentCallbacks }
	if cr, ok := agent.(callbacksReader); ok {
		return cr.Callbacks()
	}
	return nil
}

// mergeCallbacks chains base behind stream for every field stream sets, and
// passes base-only fields (run-end hooks, approval, tool interventions,
// compression) through untouched. Safe with either side nil.
func mergeCallbacks(stream, base *agentpkg.AgentCallbacks) *agentpkg.AgentCallbacks {
	if stream == nil {
		return base
	}
	if base == nil {
		return stream
	}
	m := *stream
	chainStart := func(dst func(ctx context.Context, msg string), add func(ctx context.Context, msg string)) func(context.Context, string) {
		return func(ctx context.Context, msg string) {
			if dst != nil {
				dst(ctx, msg)
			}
			add(ctx, msg)
		}
	}
	if base.OnRunStart != nil {
		m.OnRunStart = chainStart(m.OnRunStart, base.OnRunStart)
	}
	if base.OnLLMCallStart != nil {
		prev := m.OnLLMCallStart
		m.OnLLMCallStart = func(ctx context.Context, round int) {
			if prev != nil {
				prev(ctx, round)
			}
			base.OnLLMCallStart(ctx, round)
		}
	}
	if base.OnLLMCallEnd != nil {
		prev := m.OnLLMCallEnd
		m.OnLLMCallEnd = func(ctx context.Context, round int, tokens int, usage *model.UsageInfo) {
			if prev != nil {
				prev(ctx, round, tokens, usage)
			}
			base.OnLLMCallEnd(ctx, round, tokens, usage)
		}
	}
	if base.OnToolCallStart != nil {
		prev := m.OnToolCallStart
		m.OnToolCallStart = func(ctx context.Context, toolName string) {
			if prev != nil {
				prev(ctx, toolName)
			}
			base.OnToolCallStart(ctx, toolName)
		}
	}
	if base.OnToolCallEnd != nil {
		prev := m.OnToolCallEnd
		m.OnToolCallEnd = func(ctx context.Context, toolName string, err error) {
			if prev != nil {
				prev(ctx, toolName, err)
			}
			base.OnToolCallEnd(ctx, toolName, err)
		}
	}
	if base.OnToken != nil {
		prev := m.OnToken
		m.OnToken = func(ctx context.Context, delta string) {
			if prev != nil {
				prev(ctx, delta)
			}
			base.OnToken(ctx, delta)
		}
	}
	if base.OnReasoning != nil {
		prev := m.OnReasoning
		m.OnReasoning = func(ctx context.Context, delta string) {
			if prev != nil {
				prev(ctx, delta)
			}
			base.OnReasoning(ctx, delta)
		}
	}
	if m.OnRunEnd == nil {
		m.OnRunEnd = base.OnRunEnd
	}
	if m.OnApprovalRequired == nil {
		m.OnApprovalRequired = base.OnApprovalRequired
	}
	if m.OnCompression == nil {
		m.OnCompression = base.OnCompression
	}
	if m.PreToolCall == nil {
		m.PreToolCall = base.PreToolCall
	}
	if m.PostToolCall == nil {
		m.PostToolCall = base.PostToolCall
	}
	return &m
}
