// Copyright 2026 InferGlow Authors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/inferglow/context/toolclean"

	"github.com/inferglow/model"
	"github.com/inferglow/observability"
	"github.com/inferglow/orchestrator/actionruntime"
	agentpkg "github.com/inferglow/orchestrator/agent"
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
	Delta     string           `json:"delta,omitempty"` // incremental text for delta/reasoning
	Usage     *model.UsageInfo `json:"usage,omitempty"` // provider-reported usage (llm_end)
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

	sc := newStreamCallbacks(eventCh, s.spanCollector, req.SessionID)
	sc.agentID = id
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
		runStart := time.Now()
		// R9: run ctx carries the chat session id so tools (spawn_agent)
		// can attribute their side effects to this session.
		runCtx := WithRunSessionID(r.Context(), req.SessionID)
		if supportsCallbacks {
			cbs := mergeCallbacks(sc.agentCallbacks(), persistedCallbacks(agent))
			opts := []agentpkg.RunOption{agentpkg.WithCallbacks(cbs)}
			// R10: orthogonal tool-output denoise — per request, falling back
			// to the server default. The hook runs in the engine funnel
			// BEFORE the 16KB truncation, for every base context mode.
			denoise := s.cfg.ToolDenoise
			if req.ToolDenoise != nil {
				denoise = *req.ToolDenoise
			}
			if denoise {
				opts = append(opts, agentpkg.WithToolResultHook(func(toolName, content string) string {
					cleaned, rep := toolclean.Clean(content)
					if rep.OutputBytes < rep.InputBytes {
						log.Printf("[tool_denoise] %s: %d -> %d bytes (ansi=%d cr=%d dup=%d err_kept=%d)",
							toolName, rep.InputBytes, rep.OutputBytes, rep.ANSIRemoved, rep.CRFolded, rep.DupLinesRemoved, rep.ErrorLinesKept)
					}
					return cleaned
				}))
			}
			resp, runErr = runner.RunWithCallbacks(runCtx, req.Message, opts...)
		} else {
			// Demo/legacy agents expose no callback surface.
			sc.emit("run_start", "", 0, 0, "")
			resp, runErr = agent.Run(runCtx, req.Message)
		}
		sc.recordSpan(observability.SpanKindAgent, "inferglow.agent.run", runStart, runErr != nil,
			map[string]string{"inferglow.agent_id": id})

		if runErr == nil {
			sc.persistMessages(s, req.SessionID, sanitizeAgentReply(resp))
		}
		sc.persistTrace(s, req.SessionID, runErr)

		if runErr != nil {
			sc.emit("error", "", 0, 0, runErr.Error())
			return
		}
		// Contract quirk kept for compatibility: run_end carries the full
		// assistant reply in its ToolName field. Sanitized so an LLM that
		// answers in the planning-decision envelope doesn't leak raw JSON
		// into the transcript.
		sc.emit("run_end", sanitizeAgentReply(resp), 0, 0, "")
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

// streamCallbacks bridges agentpkg.AgentCallbacks to SSE events, collects the
// settled records persisted once the run completes, and records observability
// spans (plain SpanSummary, no OTel round-trip) for the 轨迹 panel.
type streamCallbacks struct {
	eventCh    chan<- ToolStreamEvent
	collector  *observability.SpanCollector
	sessionID  string
	llmStart   time.Time
	toolStarts map[string]time.Time

	mu    sync.Mutex
	tools []toolRecord

	// Run summary accumulation — persisted as a MessageRoleTrace record at
	// run completion so 轨迹/上下文 survive restarts and session restores
	// (the SpanCollector ring is in-memory only).
	agentID  string
	runStart time.Time
	runSpans []runSpanRec
	runUsage *model.UsageInfo

	// Decision-envelope gating (per LLM round). Some served models answer in
	// the framework's planning-decision JSON ({"action_calls":[...],
	// "final_response":"..."}) — raw deltas would render as JSON soup in the
	// chat. While a round's output still looks like a potential envelope
	// (first non-space char `{` or a code fence), deltas are buffered; the
	// verdict happens at OnLLMCallEnd: envelope → emit extracted
	// final_response, anything else → flush the raw buffer unchanged.
	roundBuf strings.Builder
	roundRaw bool // true once the round is decided plain (pass-through)
}

type toolRecord struct {
	name   string
	status string // "ok" | "error"
}

// runSpanRec is one persisted span line of the run summary.
type runSpanRec struct {
	Kind       string `json:"kind"` // agent | llm | tool
	Name       string `json:"name"`
	DurationMs int64  `json:"duration_ms"`
	HasError   bool   `json:"error,omitempty"`
}

func newStreamCallbacks(eventCh chan<- ToolStreamEvent, collector *observability.SpanCollector, sessionID string) *streamCallbacks {
	return &streamCallbacks{
		eventCh:    eventCh,
		collector:  collector,
		sessionID:  sessionID,
		toolStarts: make(map[string]time.Time),
	}
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

// recordSpan records a finished span: into the collector when one is wired,
// and always into the run summary that becomes the persisted trace record.
func (c *streamCallbacks) recordSpan(kind observability.SpanKind, name string, start time.Time, hasError bool, extra map[string]string) {
	c.mu.Lock()
	c.runSpans = append(c.runSpans, runSpanRec{
		Kind: string(kind), Name: name,
		DurationMs: time.Since(start).Milliseconds(), HasError: hasError,
	})
	c.mu.Unlock()
	if c.collector == nil {
		return
	}
	attrs := map[string]string{}
	if c.sessionID != "" {
		attrs["inferglow.session_id"] = c.sessionID
	}
	for k, v := range extra {
		attrs[k] = v
	}
	c.collector.OnEnd(observability.SpanSummary{
		Name:     name,
		Kind:     kind,
		Duration: time.Since(start),
		EndTime:  time.Now().UTC(),
		HasError: hasError,
		Attrs:    attrs,
	})
}

// agentCallbacks renders the SSE bridging as an agentpkg.AgentCallbacks for
// injection via WithCallbacks. Lifecycle timing also feeds the span recorder.
func (c *streamCallbacks) agentCallbacks() *agentpkg.AgentCallbacks {
	return &agentpkg.AgentCallbacks{
		OnRunStart: func(ctx context.Context, userMessage string) {
			c.mu.Lock()
			c.runStart = time.Now()
			c.mu.Unlock()
			c.emit("run_start", "", 0, 0, "")
		},
		OnLLMCallStart: func(ctx context.Context, round int) {
			c.mu.Lock()
			c.llmStart = time.Now()
			c.roundBuf.Reset()
			c.roundRaw = false
			c.mu.Unlock()
			c.emit("llm_start", "", round, 0, "")
		},
		OnLLMCallEnd: func(ctx context.Context, round int, tokens int, usage *model.UsageInfo) {
			// Resolve the round's buffered output BEFORE llm_end so the
			// extracted reply lands on the client ahead of the round marker.
			c.mu.Lock()
			buf := c.roundBuf.String()
			wasRaw := c.roundRaw
			c.roundBuf.Reset()
			c.roundRaw = false
			start := c.llmStart
			c.llmStart = time.Time{}
			c.mu.Unlock()
			if !wasRaw {
				// Verdict time: envelope → emit its user-facing text (the
				// extracted final_response, or the tool-call notice); not an
				// envelope → flush raw so nothing the model said is lost.
				if text, ok := envelopeDisplayText(buf); ok {
					if text != "" {
						c.send(ToolStreamEvent{Type: "delta", Delta: text, Round: round,
							Timestamp: time.Now().UTC().Format(time.RFC3339)})
					}
				} else if strings.TrimSpace(buf) != "" {
					c.send(ToolStreamEvent{Type: "delta", Delta: buf, Round: round,
						Timestamp: time.Now().UTC().Format(time.RFC3339)})
				}
			}
			if !start.IsZero() {
				c.recordSpan(observability.SpanKindLLM, fmt.Sprintf("inferglow.llm.call.%d", round), start, false, nil)
			}
			if usage != nil {
				c.mu.Lock()
				c.runUsage = usage
				c.mu.Unlock()
			}
			c.send(ToolStreamEvent{
				Type:      "llm_end",
				Round:     round,
				Tokens:    tokens,
				Usage:     usage,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			})
		},
		OnToolCallStart: func(ctx context.Context, toolName string) {
			c.mu.Lock()
			if c.toolStarts == nil {
				c.toolStarts = make(map[string]time.Time)
			}
			c.toolStarts[toolName] = time.Now()
			c.mu.Unlock()
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
			start := c.toolStarts[toolName]
			delete(c.toolStarts, toolName)
			c.tools = append(c.tools, toolRecord{name: toolName, status: status})
			c.mu.Unlock()
			if !start.IsZero() {
				c.recordSpan(observability.SpanKindTool, "inferglow.tool."+toolName, start, err != nil, nil)
			}
			c.emit("tool_end", toolName, 0, 0, errMsg)
		},
		OnToken: func(ctx context.Context, delta string) {
			c.mu.Lock()
			if c.roundRaw {
				c.mu.Unlock()
				c.send(ToolStreamEvent{
					Type:      "delta",
					Delta:     delta,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				})
				return
			}
			c.roundBuf.WriteString(delta)
			buf := c.roundBuf.String()
			trimmed := strings.TrimLeft(buf, " \t\r\n")
			if trimmed == "" {
				c.mu.Unlock()
				return
			}
			if trimmed[0] == '{' || strings.HasPrefix(trimmed, "```") {
				// Potential decision envelope — hold until the round ends.
				c.mu.Unlock()
				return
			}
			c.roundRaw = true
			c.mu.Unlock()
			c.send(ToolStreamEvent{
				Type:      "delta",
				Delta:     buf,
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
// agentDecision is the lenient view of the framework's planning-decision
// envelope. Only final_response being present as a JSON string is required —
// served models routinely omit next_action, which fails the engine's strict
// ParseDecision and leaves the raw JSON as the run's reply.
type agentDecision struct {
	nextAction string
	toolNames  []string
	final      *string
}

// extractAgentDecision reports whether s is a planning-decision envelope
// ({"next_action":...,"action_calls":[...],"final_response":"..."}) and
// returns its fields. Fences and trailing-comma noise are repaired first
// (same pipeline the engine uses before strict parsing).
func extractAgentDecision(s string) (agentDecision, bool) {
	t := strings.TrimSpace(s)
	if t == "" || (t[0] != '{' && !strings.HasPrefix(t, "```")) {
		return agentDecision{}, false
	}
	repaired := actionruntime.RepairLLMJSON(t)
	var probe struct {
		NextAction  string `json:"next_action"`
		ActionCalls []struct {
			Name string `json:"name"`
		} `json:"action_calls"`
		FinalResponse *string `json:"final_response"`
	}
	if err := json.Unmarshal([]byte(repaired), &probe); err != nil {
		return agentDecision{}, false
	}
	// Envelope shape requires at least one of the three decision fields; a
	// random JSON object without any is not a decision.
	if probe.NextAction == "" && probe.ActionCalls == nil && probe.FinalResponse == nil {
		return agentDecision{}, false
	}
	d := agentDecision{nextAction: probe.NextAction, final: probe.FinalResponse}
	for _, c := range probe.ActionCalls {
		if c.Name != "" {
			d.toolNames = append(d.toolNames, c.Name)
		}
	}
	return d, true
}

// persistTrace stores one run summary (agent, timing, spans, usage) as a
// trace-role message so the 轨迹/上下文 panels can rebuild from history.
func (c *streamCallbacks) persistTrace(s *Server, sessionID string, runErr error) {
	if sessionID == "" {
		return
	}
	c.mu.Lock()
	summary := map[string]any{
		"agent_id": c.agentID,
		"start":    c.runStart.UTC().Format(time.RFC3339),
		"duration": time.Since(c.runStart).Round(time.Millisecond).String(),
		"spans":    c.runSpans,
		"error":    "",
	}
	if runErr != nil {
		summary["error"] = runErr.Error()
	}
	if c.runUsage != nil {
		summary["usage"] = c.runUsage
	}
	c.mu.Unlock()
	b, err := json.Marshal(summary)
	if err != nil {
		return
	}
	s.recordMessage(sessionID, MessageRoleTrace, string(b), "", "")
}

// envelopeDisplayText is the streaming-side twin of sanitizeAgentReply: the
// user-facing text for an envelope-shaped round output. ok=false means s is
// not an envelope (caller flushes it raw).
func envelopeDisplayText(s string) (string, bool) {
	d, ok := extractAgentDecision(s)
	if !ok {
		return "", false
	}
	if d.final != nil && *d.final != "" {
		return *d.final, true
	}
	if len(d.toolNames) > 0 {
		return fmt.Sprintf("（模型请求调用工具：%s — 当前会话为纯聊天模式，未启用工具执行，因此没有文本回复）",
			strings.Join(d.toolNames, ", ")), true
	}
	return "", true
}

// sanitizeAgentReply turns a run reply that arrived as a decision envelope
// into user-facing text: final_response verbatim; an execute-only decision
// (action_calls without text) as an honest notice — the pure-chat session
// has no tool runtime, so there is nothing else to show. Plain replies pass
// through unchanged.
func sanitizeAgentReply(reply string) string {
	d, ok := extractAgentDecision(reply)
	if !ok {
		return reply
	}
	if d.final != nil && *d.final != "" {
		return *d.final
	}
	if len(d.toolNames) > 0 {
		return fmt.Sprintf("（模型请求调用工具：%s — 当前会话为纯聊天模式，未启用工具执行，因此没有文本回复）",
			strings.Join(d.toolNames, ", "))
	}
	return ""
}

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
	type callbacksReader interface {
		Callbacks() *agentpkg.AgentCallbacks
	}
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
