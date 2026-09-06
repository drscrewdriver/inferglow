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

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/inferglow/action"
	"github.com/inferglow/audit"
	"github.com/inferglow/flow"
	"github.com/inferglow/model"
	"github.com/inferglow/orchestrator/actionruntime"
	"github.com/inferglow/session"
)

// ErrToolCallCapReached is returned when the agent hits the hard cap on
// consecutive tool-execution rounds without the model producing a final
// response. The caller should treat this as a runaway tool loop.
var ErrToolCallCapReached = errors.New("agent: tool-call round cap reached without final response")

// DefaultMaxToolCallRounds is the hard cap on consecutive tool-execution
// rounds within a single executeLoop run. Prevents infinite loops when the
// model keeps calling tools without producing a final response. Tool rounds
// do NOT count against maxRounds (the main round counter), but this separate
// limit ensures the agent cannot run away.
//
// Set to 80 to accommodate complex tasks that require extensive file
// exploration (10-15 reads) followed by multi-file code generation (5-10
// writes) and verification (3-5 bash calls). Smaller values (e.g. 25)
// caused the agent to hit the cap during exploration without ever writing
// code, resulting in empty synthesis responses.
const DefaultMaxToolCallRounds = 80

// toolCallStaleThreshold is the number of consecutive identical tool-call
// batches before the agent injects a nudge message. When the model calls
// the same tool with the same arguments multiple times in a row, it is
// likely stuck in a loop. The nudge tells it to move on.
const toolCallStaleThreshold = 3

// Engine orchestrates the PLAN → EXECUTE loop.
type Engine struct {
	session   *SessionExtension
	actionExt *ActionExtension
	modelReq  model.StreamRequester
	// modelReqOverride, when non-nil, replaces modelReq for the current run
	// (RF-1: per-run /model switching). Propagated from Agent.Run via
	// WithModelRequester; reset per run. nil keeps modelReq.
	modelReqOverride model.StreamRequester
	// modelOptions, when non-nil, are merged into every ModelRequest.Options
	// of the current run (RF-2: /effort injection). Caller keys win over
	// engine-built keys. Propagated from Agent.Run via WithModelOptions.
	modelOptions map[string]any
	auditHook audit.AuditHook
	loopGuard *LoopGuard

	// streamTimeout caps how long executeLoop will wait for the model
	// stream channel to deliver the next chunk. Zero means "use the
	// default 5-minute timeout". Configurable from Agent.Run via the
	// WithStreamTimeout RunOption. Protects against stuck streams that
	// would otherwise block the loop forever (BUG-8).
	streamTimeout time.Duration

	// toolDefsHash is the SHA-256 of the byte-stable serialization of the
	// current tool definitions (sorted by Name). It exists so callers can
	// detect when the tool set has changed and invalidate prefix caches.
	// Populated lazily by buildToolDefinitions.
	toolDefsHash string

	// cachedTools stores the last built tool definitions. When the action
	// registry has not changed (same toolDefsHash), buildToolDefinitions
	// returns the cached slice instead of rebuilding. nil means no cache.
	cachedTools []model.ToolDefinition
	// cachedToolsHash is the hash that was used to populate cachedTools.
	// When it matches the newly computed hash, the cache is valid.
	cachedToolsHash string

	// turnLoop tracks the PLAN → EXECUTE phase and supports preemption of
	// an in-flight turn. Always initialized by the constructors; nil when
	// an Engine is constructed via a struct literal without fields (e.g.
	// in legacy tests), in which case all turn-loop integration is skipped.
	turnLoop *TurnLoop
	// cancelManager coordinates cancel requests with executeLoop safe-points.
	// Always initialized by the constructors; nil when an Engine is
	// constructed via a struct literal without fields, in which case all
	// cancel checks are skipped.
	cancelManager *CancelManager

	// outputSchema is the optional L4 output schema used for post-validation
	// of the LLM response. When non-nil, executeLoop also injects the L3
	// schema prompt into the system prompt as a fallback for providers that
	// cannot enforce json_schema-level response_format. Set by Agent.Run from
	// the runConfig.outputSchema (populated via WithOutputSchema). nil
	// disables both L3 and L4 (default).
	outputSchema *model.OutputSchema

	// tracer 是可选的 OpenTelemetry tracer，供 ResumeFlow 等 Engine 方法
	// 创建语义 span（如 SpanResume）。Agent.Run 在执行 executeFlow 前会把
	// runConfig.tracer 写入该字段，使后续 ResumeFlow 调用也能产出 span。
	// nil 时所有 Engine 层 span 退化为 no-op。
	tracer SpanStarter

	// maxToolCallRounds caps the number of tool-execution rounds per
	// executeLoop run. Zero means use DefaultMaxToolCallRounds.
	maxToolCallRounds int

	// rateLimitHook is called before each RequestModel invocation to
	// enforce rate limiting. nil disables rate limit checks (default).
	// Propagated from Agent.Run via runConfig.rateLimitHook.
	rateLimitHook RateLimitHook

	// callbacks provides lifecycle hooks for observability. nil disables
	// all callback invocations (zero overhead). Propagated from Agent.Run
	// via runConfig.callbacks.
	callbacks *AgentCallbacks

	// rollout 是可选会话级 Rollout 记录器（R3）。nil 时 executeLoop 的所有
	// 记录点零开销、零行为变化（向后兼容硬约束）。非 nil 时按发生顺序把
	// user_message / tool_call / tool_result / assistant_message 追加为
	// JSONL。通常经 RunOption WithRollout 注入；对 ephemeral 会话由调用方
	// 以空目录构造 recorder 实现 no-op。
	rollout *session.RolloutRecorder

	// cacheBudgetHook is called after each LLM response with the
	// cached_tokens count from UsageInfo. nil disables (default).
	// Propagated from Agent.Run via runConfig.cacheBudgetUpdater.
	cacheBudgetHook func(cachedTokens int)

	// compactHook is called after each LLM turn with promptTokens
	// to trigger ModeSummary compaction. nil disables (default).
	// Propagated from Agent.Run via runConfig.compactHook.
	compactHook func(promptTokens int)

	// inputQueue is the bounded FIFO queue for user inputs submitted while
	// the agent is busy. nil disables queue draining (default).
	// Propagated from Agent.inputQueue.
	inputQueue *InputQueue

	// initialContentBlocks carries multimodal content for the first user
	// message of a run (image/audio/video). Consumed and reset by executeLoop.
	initialContentBlocks []model.ContentBlock

	// pendingInterleave holds the InputRequest currently being processed
	// from the input queue. When the turn completes, the response is sent
	// on pendingInterleave.ResponseCh. nil when no interleave is active.
	pendingInterleave *InputRequest

	// depth tracks the nesting level for sub-agents. 0 = top-level.
	// Incremented by cloneEngineForParallel. Used to enforce MaxDepth.
	depth int

	// lastPreemptState stores the TurnState snapshot from the most recent
	// preempt exit path. nil when no preempt has occurred.
	lastPreemptState *TurnState
}

// newTurnLoopAndCancel creates a TurnLoop and its paired CancelManager. Used
// by every constructor so the two fields stay consistent.
func newTurnLoopAndCancel() (*TurnLoop, *CancelManager) {
	tl := NewTurnLoop()
	return tl, NewCancelManager(tl)
}

// NewEngine creates an Engine with the given components. The returned Engine
// has a NoOpHook (zero-overhead) and no LoopGuard, matching the pre-audit
// behavior exactly. A TurnLoop and CancelManager are initialized so the
// engine is ready to accept cancel requests.
func NewEngine(sess *SessionExtension, actExt *ActionExtension, mr model.StreamRequester) *Engine {
	tl, cm := newTurnLoopAndCancel()
	return &Engine{
		session:       sess,
		actionExt:     actExt,
		modelReq:      mr,
		auditHook:     &audit.NoOpHook{},
		loopGuard:     nil,
		turnLoop:      tl,
		cancelManager: cm,
	}
}

// activeRequester returns the per-run override when set, otherwise the
// engine's construction-time requester (RF-1). Never returns nil when the
// engine was built through a constructor.
func (e *Engine) activeRequester() model.StreamRequester {
	if e.modelReqOverride != nil {
		return e.modelReqOverride
	}
	return e.modelReq
}

// RunLoop executes a complete PLAN→EXECUTE agent loop and returns the final
// response text. It is the public entry point for external callers (e.g.
// inferflow's FlowContext) that need multi-turn agent capabilities without
// depending on the internal executeLoop signature or the actionruntime.Decision
// type.
func (e *Engine) RunLoop(ctx context.Context, userMessage string, maxRounds int, systemPrompt string) (string, error) {
	decision, err := e.executeLoop(ctx, userMessage, maxRounds, systemPrompt)
	if err != nil {
		return "", err
	}
	if decision == nil {
		return "", fmt.Errorf("agent: RunLoop returned nil decision")
	}

	// When the loop exits with an "execute" decision (e.g. maxRounds reached
	// while the model was still using tools), FinalResponse is empty. Make
	// one final synthesis call without tools so the model summarises the
	// conversation instead of leaving the caller with an empty string.
	if decision.FinalResponse == "" && decision.NextAction == "execute" {
		synthResp, synthErr := e.synthesiseResponse(ctx, systemPrompt)
		if synthErr != nil {
			return "", fmt.Errorf("agent: synthesis call failed: %w", synthErr)
		}
		decision.FinalResponse = synthResp
	}

	// Deliver the response to a pending interleave request (message queue
	// drain). The caller that submitted the message via SubmitInput blocks
	// on ResponseCh; send the final response so it unblocks.
	if e.pendingInterleave != nil {
		if e.pendingInterleave.ResponseCh != nil {
			e.pendingInterleave.ResponseCh <- InputResponse{Response: decision.FinalResponse}
		}
		e.pendingInterleave = nil
	}

	return decision.FinalResponse, nil
}

// synthesiseResponse makes a single LLM call without tools, asking the model
// to summarise the conversation so far. Used when the agent loop exits
// (e.g. maxRounds) while the model was still in "execute" mode.
func (e *Engine) synthesiseResponse(ctx context.Context, systemPrompt string) (string, error) {
	synthReq := &model.ModelRequest{
		System:      systemPrompt + "\n\nYou have finished using tools. Now provide a comprehensive summary of what was accomplished. Do NOT call any tools.",
		ChatHistory: e.session.PreparePrompt(),
		Options:     map[string]any{"force_json": false},
	}
	data, err := e.activeRequester().GenerateRequestData(ctx, synthReq)
	if err != nil {
		return "", err
	}
	synthCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	stream, err := e.activeRequester().RequestModel(synthCtx, data)
	if err != nil {
		return "", err
	}
	var content strings.Builder
	for chunk := range stream {
		content.WriteString(chunk.Delta)
	}
	return strings.TrimSpace(content.String()), nil
}

// NewEngineWithAudit creates an Engine that appends decision audit entries
// via hook. The loopGuard is nil. A nil hook is replaced with NoOpHook.
func NewEngineWithAudit(sess *SessionExtension, actExt *ActionExtension, mr model.StreamRequester, hook audit.AuditHook) *Engine {
	if hook == nil {
		hook = &audit.NoOpHook{}
	}
	tl, cm := newTurnLoopAndCancel()
	return &Engine{
		session:       sess,
		actionExt:     actExt,
		modelReq:      mr,
		auditHook:     hook,
		loopGuard:     nil,
		turnLoop:      tl,
		cancelManager: cm,
	}
}

// NewEngineWithLoopGuard creates an Engine that consults guard before each
// LLM call. The audit hook defaults to NoOpHook.
func NewEngineWithLoopGuard(sess *SessionExtension, actExt *ActionExtension, mr model.StreamRequester, guard *LoopGuard) *Engine {
	tl, cm := newTurnLoopAndCancel()
	return &Engine{
		session:       sess,
		actionExt:     actExt,
		modelReq:      mr,
		auditHook:     &audit.NoOpHook{},
		loopGuard:     guard,
		turnLoop:      tl,
		cancelManager: cm,
	}
}

// NewEngineWithAuditAndLoopGuard creates an Engine with both an AuditHook
// and a LoopGuard. Either may be nil to disable that feature; a nil hook is
// replaced with NoOpHook.
func NewEngineWithAuditAndLoopGuard(sess *SessionExtension, actExt *ActionExtension, mr model.StreamRequester, hook audit.AuditHook, guard *LoopGuard) *Engine {
	if hook == nil {
		hook = &audit.NoOpHook{}
	}
	tl, cm := newTurnLoopAndCancel()
	return &Engine{
		session:       sess,
		actionExt:     actExt,
		modelReq:      mr,
		auditHook:     hook,
		loopGuard:     guard,
		turnLoop:      tl,
		cancelManager: cm,
	}
}

// CancelManager returns the engine's CancelManager, or nil if the Engine was
// constructed without one (e.g. via a struct literal). Callers use it to
// request cancellation of an in-flight executeLoop run.
func (e *Engine) CancelManager() *CancelManager {
	return e.cancelManager
}

// LastPreemptState returns the TurnState snapshot from the most recent
// preempt exit, or nil if no preempt has occurred during this engine's
// lifetime.
func (e *Engine) LastPreemptState() *TurnState {
	return e.lastPreemptState
}

// preemptDrainNext handles the exit of a preempted turn: it delivers an
// ErrTurnInterrupted response to the in-flight interleave (non-blocking,
// so a timed-out caller cannot stall the loop), then dequeues the next
// highest-priority input so the loop can continue with it. It reports
// whether a queued input was found (caller should continue the loop).
func (e *Engine) preemptDrainNext() bool {
	if e.pendingInterleave != nil {
		if ch := e.pendingInterleave.ResponseCh; ch != nil {
			select {
			case ch <- InputResponse{Error: ErrTurnInterrupted}:
			default: // caller already gone (timeout/ctx) — do not block
			}
		}
		e.pendingInterleave = nil
	}
	if e.inputQueue == nil {
		return false
	}
	req, ok := e.inputQueue.Dequeue()
	if !ok {
		return false
	}
	e.session.AddUserMessage(req.Message)
	e.pendingInterleave = &req
	return true
}

// executeLoop runs the PLAN → EXECUTE loop until the LLM returns a response
// or maxRounds is reached.
func (e *Engine) executeLoop(ctx context.Context, userMessage string, maxRounds int, systemPrompt string) (dec *actionruntime.Decision, err error) {
	// 裸聊天环（Agent.Run 不经 flow 编排）此前不带 flow 上下文，依赖
	// flow.Context 的内建 action（如 spawn_agent）会因 ContextFrom 失败而
	// 不可用。此处统一安装：已有上下文（executeFlow / RunAgent 嵌套路径）
	// 时不覆盖。
	if _, ok := flow.ContextFrom(ctx); !ok {
		modelReq := e.modelReq
		if e.modelReqOverride != nil {
			modelReq = e.modelReqOverride
		}
		fc := &flowContextImpl{
			session:   e.session,
			actionExt: e.actionExt,
			modelReq:  modelReq,
			auditHook: e.auditHook,
			tracer:    e.tracer,
			engine:    e,
		}
		ctx = flow.WithFlowContext(ctx, fc)
	}

	// Fire OnRunStart callback.
	fireOnRunStart(e.callbacks, ctx, userMessage)

	// Fire OnRunEnd callback on exit.
	defer func() {
		resp := ""
		if dec != nil {
			resp = dec.FinalResponse
		}
		fireOnRunEnd(e.callbacks, ctx, resp, err)
	}()

	// Add user message to session
	if len(e.initialContentBlocks) > 0 {
		e.session.AddUserContentBlocks(userMessage, e.initialContentBlocks)
		// Consumed per-run; reset so it never leaks into later turns on a
		// long-lived Engine.
		e.initialContentBlocks = nil
	} else {
		e.session.AddUserMessage(userMessage)
	}
	// R3：记录本轮用户消息入口。
	e.recordRollout(session.RolloutItem{Type: session.RolloutUserMessage, Content: userMessage})

	// Ensure the TurnLoop is left in the idle phase no matter how the loop
	// exits (normal return, error, preempt, or cancel). Preempt already
	// transitions to idle, so the defer is a no-op in that case. Engines
	// constructed without a TurnLoop (legacy struct literals) skip this.
	if e.turnLoop != nil {
		defer func() {
			if e.turnLoop.Phase() != TurnPhaseIdle {
				e.turnLoop.EnterIdle()
			}
		}()
	}

	runStart := time.Now()
	var totalTokens int
	var prevDecision *actionruntime.Decision
	var prevOutput string
	var prevReasoning string // tracks last round's reasoning for LoopGuard

	round := 0
	toolCallRounds := 0
	maxTCR := e.maxToolCallRounds
	if maxTCR <= 0 {
		maxTCR = DefaultMaxToolCallRounds
	}
	// B5: Generate task group for this turn. All steps within this
	// executeLoop invocation share the same task group identifier.
	// This can be propagated to context manager via callbacks or session metadata.
	_ = fmt.Sprintf("turn_%d_%d", time.Now().Unix(), round) // taskGroup for causal tracking
	// Tool-call dedup: detect when the model is stuck calling the same
	// tool with the same arguments repeatedly.
	var lastToolSig string
	var staleCount int
	halfwayWarned := false
	prefixSet := false // tracks whether SetImmutablePrefix has been called this loop
	for {
		// LoopGuard check before the LLM call. State reflects what's known
		// at this point: round index, previous round's ActionCalls (nil on
		// first round), previous LLM output, accumulated tokens, and run
		// start time.
		if e.loopGuard != nil {
			var prevCalls []actionruntime.ActionCall
			if prevDecision != nil {
				prevCalls = prevDecision.ActionCalls
			}
			state := LoopGuardState{
				Round:         round,
				ActionCalls:   prevCalls,
				LastOutput:    prevOutput,
				LastReasoning: prevReasoning,
				TotalTokens:   totalTokens,
				StartedAt:     runStart,
			}
			verdict, _ := e.loopGuard.Check(state)
			if verdict != nil {
				switch verdict.Action {
				case VerdictBreak:
					return nil, fmt.Errorf("%w: %s", ErrLoopDetected, verdict.Reason)
				case VerdictDegrade:
					systemPrompt = systemPrompt + "\n" + verdict.Reason
				}
			}
		}

		// Point 1: Check for a pending immediate cancel at loop start
		// (before entering planning). A safe-point cancel that has been
		// escalated to immediate by CheckTimeoutEscalation also fires here.
		// CancelImmediate matches every safe-point, so CheckCancel returns
		// true for any active immediate request.
		if e.cancelManager != nil {
			e.cancelManager.CheckTimeoutEscalation()
			if e.cancelManager.HasPendingCancel() && e.cancelManager.CheckCancel(CancelImmediate) {
				e.cancelManager.CompleteCancel(nil)
				e.capturePreemptState(round, toolCallRounds)
				if e.preemptDrainNext() {
					prefixSet = false
					halfwayWarned = false
					continue // new turn processes the queued input
				}
				return nil, fmt.Errorf("%w", ErrTurnInterrupted)
			}
		}

		// Point 2: enter the planning phase and obtain the preempt channel.
		// preemptCh is nil when there is no TurnLoop; selecting on a nil
		// channel blocks forever, so the streamLoop case is inert.
		var preemptCh chan struct{}
		if e.turnLoop != nil {
			preemptCh = e.turnLoop.EnterPlanning()
		}

		// Build ModelRequest
		tools := e.buildToolDefinitions()
		hasTools := len(tools) > 0

		// Set the immutable prefix (Zone 1) on the first iteration so the
		// backend can maximize prefix cache hits. Only done once per loop
		// since the system prompt and tool definitions are stable.
		if !prefixSet {
			// Convert tools to []any for the zone API.
			toolsAny := make([]any, len(tools))
			for i, t := range tools {
				toolsAny[i] = t
			}
			if err := e.session.SetImmutablePrefix(systemPrompt, toolsAny); err != nil {
				log.Printf("[agent] SetImmutablePrefix failed (non-fatal): %v", err)
			}
			prefixSet = true
		}

		req := &model.ModelRequest{
			System:      systemPrompt,
			ChatHistory: e.session.PreparePrompt(),
			Tools:       tools,
			ToolChoice: func() any {
				if hasTools {
					return "auto"
				}
				return nil
			}(),
			Options: func() map[string]any {
				var opts map[string]any
				if hasTools {
					// With tools, skip force_json to allow native function calling.
					// response_format conflicts with tool_calls in OpenAI-compatible APIs.
					// Increase max_tokens for agent loops to handle large tool arguments
					// (e.g., code_executor with multi-KB source code).
					opts = map[string]any{"max_tokens": 16384}
				} else {
					opts = map[string]any{"force_json": true}
				}
				// RF-2: merge per-run options (e.g. /effort reasoning_effort).
				// Caller keys win over engine-built keys. nil map is a no-op.
				for k, v := range e.modelOptions {
					opts[k] = v
				}
				return opts
			}(),
		}
		// Only set Output schema when there are NO tools. When tools are
		// present, the model uses native function calling instead of a
		// custom JSON schema for action dispatch.
		if !hasTools {
			req.Output = &model.OutputSchema{
				Type: "object",
				Properties: map[string]any{
					"next_action": map[string]any{
						"type":        "string",
						"description": "\"execute\" or \"response\"",
					},
					"action_calls": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"name":   map[string]any{"type": "string"},
								"params": map[string]any{"type": "object"},
							},
						},
					},
					"final_response": map[string]any{
						"type":        "string",
						"description": "The response to return when next_action is \"response\"",
					},
				},
			}
		}

		// L3 prompt injection: when outputSchema is configured and the
		// provider cannot enforce json_schema-level response_format (no
		// response_format set, or explicitly degraded to json_object),
		// append a schema description to the system prompt so the model
		// has prompt-level guidance for the expected JSON structure.
		if e.outputSchema != nil && shouldInjectSchemaPrompt(req) {
			req.System += formatSchemaInstruction(req.Output)
		}

		// Fire OnLLMCallStart callback.
		fireOnLLMCallStart(e.callbacks, ctx, round)

		// RateLimitHook: check rate limits before calling RequestModel.
		// nil hook means no rate limiting (zero overhead).
		if e.rateLimitHook != nil {
			if rlErr := e.rateLimitHook.Acquire(ctx, "default", 0); rlErr != nil {
				return nil, fmt.Errorf("rate limit exceeded: %w", rlErr)
			}
		}

		// Call LLM
		data, err := e.activeRequester().GenerateRequestData(ctx, req)
		if err != nil {
			return nil, err
		}

		// BUG-8: cap stream consumption with a timeout so a stuck stream
		// cannot block executeLoop forever. Default 5 minutes; overridable
		// via Engine.streamTimeout (set by Agent.Run from WithStreamTimeout).
		// The timeoutCtx is passed to RequestModel so the provider's stream
		// goroutine observes cancellation and stops producing chunks. This
		// also avoids goroutine leaks when the stream stalls.
		streamTimeout := e.streamTimeout
		if streamTimeout <= 0 {
			streamTimeout = 5 * time.Minute
		}
		timeoutCtx, cancelTimeout := context.WithTimeout(ctx, streamTimeout)
		stream, err := e.activeRequester().RequestModel(timeoutCtx, data)
		if err != nil {
			cancelTimeout()
			return nil, err
		}

		// Collect response content, reasoning, and native tool calls
		var content strings.Builder
		var reasoning strings.Builder // G1-02: accumulate reasoning for passback
		var nativeToolCalls []model.ToolCall
		var lastUsage *model.UsageInfo // track provider-reported token usage
	streamLoop:
		for {
			select {
			case chunk, ok := <-stream:
				if !ok {
					break streamLoop
				}
				content.WriteString(chunk.Delta)
				// Emit token delta for real-time display.
				if chunk.Delta != "" {
					fireOnToken(e.callbacks, ctx, chunk.Delta)
				}
				// G1-02: accumulate reasoning content for DeepSeek/MiMo passback.
				if chunk.Reasoning != "" {
					reasoning.WriteString(chunk.Reasoning)
					fireOnReasoning(e.callbacks, ctx, chunk.Reasoning)
				}
				// Collect native tool calls from the stream.
				if len(chunk.Tools) > 0 {
					nativeToolCalls = append(nativeToolCalls, chunk.Tools...)
				}
				// Track the latest UsageInfo from the stream.
				if chunk.Usage != nil {
					lastUsage = chunk.Usage
				}
			case <-timeoutCtx.Done():
				cancelTimeout()
				return nil, timeoutCtx.Err()
			case <-preemptCh:
				cancelTimeout()
				if e.cancelManager != nil && e.cancelManager.HasPendingCancel() {
					e.cancelManager.CompleteCancel(nil)
				}
				reason := ""
				if e.turnLoop != nil {
					reason = e.turnLoop.PreemptReason()
				}
				e.capturePreemptState(round, toolCallRounds)
				if e.preemptDrainNext() {
					prefixSet = false
					halfwayWarned = false
					continue // new turn processes the queued input
				}
				return nil, fmt.Errorf("agent preempted: %s: %w", reason, ErrTurnInterrupted)
			}
		}
		cancelTimeout()

		// Debug: log what the LLM returned
		log.Printf("[agent-debug] round=%d contentLen=%d nativeToolCalls=%d content_preview=%q",
			round, content.Len(), len(nativeToolCalls), truncate(content.String(), 200))

		// L4 post-validation: when outputSchema is configured, validate the
		// collected content against the schema. On validation failure, retry
		// by re-calling the model (up to MaxRetries times). The first
		// attempt reuses the already-collected content; subsequent attempts
		// issue a fresh model call with the same req.
		if e.outputSchema != nil {
			validator := model.NewOutputValidator(e.outputSchema)
			validator.MaxRetries = 2

			firstContent := content.String()
			isFirst := true

			validatedResp, valErr := validator.ValidateAndRetryWithFetch(ctx, func(ctx context.Context) (*model.ModelResponse, error) {
				if isFirst {
					isFirst = false
					return &model.ModelResponse{Content: firstContent}, nil
				}
				retryData, rErr := e.activeRequester().GenerateRequestData(ctx, req)
				if rErr != nil {
					return nil, rErr
				}
				retryTimeoutCtx, cancelRetry := context.WithTimeout(ctx, streamTimeout)
				defer cancelRetry()
				retryStream, rErr := e.activeRequester().RequestModel(retryTimeoutCtx, retryData)
				if rErr != nil {
					return nil, rErr
				}
				var retryContent strings.Builder
				for chunk := range retryStream {
					retryContent.WriteString(chunk.Delta)
				}
				return &model.ModelResponse{Content: retryContent.String()}, nil
			})
			if valErr != nil {
				return nil, fmt.Errorf("L4 output validation failed: %w", valErr)
			}
			content.Reset()
			content.WriteString(validatedResp.Content)
		}

		// Feed cached_tokens back to context manager for sweet-spot adjustment.
		if e.cacheBudgetHook != nil && lastUsage != nil {
			if cached := lastUsage.PromptTokensDetails["cached_tokens"]; cached > 0 {
				e.cacheBudgetHook(cached)
			}
		}

		// Trigger ModeSummary compaction check after each LLM turn.
		if e.compactHook != nil && lastUsage != nil && lastUsage.PromptTokens > 0 {
			e.compactHook(lastUsage.PromptTokens)
			// Fire compression callback for TUI visualization.
			fireOnCompression(e.callbacks, ctx, 1)
		}

		// Approximate token accumulation: prefer provider-reported UsageInfo
		// when available (more accurate), fall back to len-based estimate.
		if lastUsage != nil && lastUsage.CompletionTokens > 0 {
			totalTokens += lastUsage.CompletionTokens
		} else {
			totalTokens += len(content.String())
		}

		// Fire OnLLMCallEnd callback with accurate token count.
		// Prefer provider-reported CompletionTokens when available.
		endTokens := len(content.String())
		if lastUsage != nil && lastUsage.CompletionTokens > 0 {
			endTokens = lastUsage.CompletionTokens
		}
		fireOnLLMCallEnd(e.callbacks, ctx, round, endTokens, lastUsage)

		// Build decision: prefer native tool calls over custom JSON schema.
		var decision *actionruntime.Decision
		if len(nativeToolCalls) > 0 {
			// Native function calling: model returned tool_calls.
			// G1-02: pass reasoning for DeepSeek/MiMo multi-turn passback.
			e.session.AddAssistantToolCalls(nativeToolCalls, reasoning.String())

			actionCalls := make([]actionruntime.ActionCall, 0, len(nativeToolCalls))
			for _, tc := range nativeToolCalls {
				actionCalls = append(actionCalls, actionruntime.ActionCall{
					Name:   tc.Name,
					Params: tc.Arguments,
				})
			}
			decision = &actionruntime.Decision{
				NextAction:  "execute",
				ActionCalls: actionCalls,
				Reasoning:   reasoning.String(),
			}
		} else {
			// No native tool calls — parse the text content as a decision.
			var parseErr error
			decision, parseErr = actionruntime.ParseDecision(content.String())
			if parseErr != nil {
				decision = &actionruntime.Decision{
					NextAction:    "response",
					FinalResponse: content.String(),
					Reasoning:     reasoning.String(),
				}
			} else {
				// ParseDecision succeeded — still attach reasoning (G1-02).
				decision.Reasoning = reasoning.String()
			}
		}

		// Append decision audit entry. Only when the hook reports IsEnabled
		// so NoOpHook (and nil-then-defaulted NoOpHook) keep zero overhead.
		if e.auditHook != nil && e.auditHook.IsEnabled() {
			input := userMessage
			if round > 0 && prevDecision != nil {
				input = prevDecision.FinalResponse
			}

			metadata := map[string]string{
				"round": strconv.Itoa(round),
			}

			// Inject token usage metadata when available from the LLM response.
			if lastUsage != nil {
				modelName := req.Model
				if modelName == "" {
					modelName = data.Model
				}
				providerName := e.activeRequester().Name()
				if providerName == "" {
					providerName = "unknown"
				}
				if modelName == "" {
					modelName = providerName
				}

				metadata["model"] = modelName
				metadata["provider"] = providerName
				metadata["input_tokens"] = strconv.Itoa(lastUsage.PromptTokens)
				metadata["output_tokens"] = strconv.Itoa(lastUsage.CompletionTokens)

				cachedTokens := 0
				if lastUsage.PromptTokensDetails != nil {
					cachedTokens = lastUsage.PromptTokensDetails["cached_tokens"]
				}
				metadata["cached_tokens"] = strconv.Itoa(cachedTokens)

				metadata["reasoning_tokens"] = strconv.Itoa(lastUsage.ReasoningTokens())
			}

			_, _ = e.auditHook.Append(&audit.AuditEntry{
				Timestamp: time.Now(),
				Source:    "agent",
				Action:    "decision",
				Input:     input,
				Output:    decision,
				Metadata:  metadata,
			})
		}

		prevDecision = decision
		prevOutput = content.String()
		prevReasoning = reasoning.String()

		// Check if we should continue
		if !actionruntime.ShouldContinue(*decision, round, maxRounds) {
			// R3：产出最终回复时记录 assistant_message（发生在所有
			// tool_call / tool_result 之后，吻合 user→tool_call→
			// tool_result→assistant 的会话级顺序）。
			if decision.FinalResponse != "" {
				e.recordRollout(session.RolloutItem{Type: session.RolloutAssistantMessage, Content: decision.FinalResponse})
			}
			return decision, nil
		}

		// Point 4: CancelAfterChatModel safe-point. The LLM call has finished
		// and the decision is parsed; honor a pending safe-point cancel before
		// executing any tools. CancelImmediate also matches here. The current
		// decision is returned so the caller sees the model's output.
		if e.cancelManager != nil {
			e.cancelManager.CheckTimeoutEscalation()
			if e.cancelManager.HasPendingCancel() && e.cancelManager.CheckCancel(CancelAfterChatModel) {
				e.cancelManager.CompleteCancel(nil)
				return decision, nil
			}
		}

		// Point 5: enter the active phase (tool execution). preemptCh is
		// reassigned for state-tracking; tool execution is synchronous so it
		// is not selected on here.
		if e.turnLoop != nil {
			preemptCh = e.turnLoop.EnterActive()
		}

		// Tool-call dedup (pre-execution): detect when the model is stuck
		// calling the same tool with the same arguments. On the 3rd
		// consecutive identical batch, recognize it as a stuck loop and
		// trigger the synthesis fallback instead of executing the tool again
		// or erroring. (ErrLoopDetected is now raised only by loopGuard at
		// L411-433 for policy-level stuck detection.)
		if len(decision.ActionCalls) > 0 {
			var sigParts []string
			for _, ac := range decision.ActionCalls {
				paramJSON, _ := json.Marshal(ac.Params)
				sigParts = append(sigParts, ac.Name+":"+string(paramJSON))
			}
			curSig := strings.Join(sigParts, "|")
			if curSig == lastToolSig {
				staleCount++
			} else {
				staleCount = 1
				lastToolSig = curSig
			}
			if staleCount >= toolCallStaleThreshold {
				log.Printf("[agent] stale tool-call detected: %d consecutive identical calls (tool=%s); triggering synthesis",
					staleCount, decision.ActionCalls[0].Name)
				// Recognize the stuck loop: return an empty execute decision so
				// RunLoop/Agent.Run triggers the synthesis fallback immediately
				// (same semantics as the tool-call cap at L1133-1136). Do NOT
				// wait for maxToolCallRounds and do NOT re-execute the identical
				// tool this round.
				return &actionruntime.Decision{
					NextAction:    "execute",
					FinalResponse: "",
				}, nil
			}
		}

		// Execute actions. We always use NewActionDispatcherWithAudit so the
		// per-action audit entries flow through the same hook as decisions.
		// With a NoOpHook or nil hook this is zero overhead.
		dispatcher := actionruntime.NewActionDispatcherWithAudit(e.actionExt.GetRegistry(), e.auditHook)

		// PreToolCall 干预：在派发给 dispatcher 前，对每个调用逐个调用
		// PreToolCall 钩子。未安装钩子时整体跳过（pendingCalls 直接复用
		// decision.ActionCalls），行为与现状完全一致。
		// pendingCalls 是实际派发的调用（参数可能被 RewriteParams 改写）；
		// pendingIdx 记录派发调用在 decision.ActionCalls 中的原始下标；
		// preBlocked 与 decision.ActionCalls 对齐，被 Block 的调用直接
		// 填充与 approval 拦截同形的 blocked 结果，不进入 dispatcher；
		// preContexts 与 decision.ActionCalls 对齐，记录 Pre 附加上下文。
		hasPreHook := e.callbacks != nil && e.callbacks.PreToolCall != nil
		var pendingCalls []actionruntime.ActionCall
		var pendingIdx []int
		var preBlocked []*action.ActionResult
		var preContexts []string
		if hasPreHook {
			n := len(decision.ActionCalls)
			preBlocked = make([]*action.ActionResult, n)
			preContexts = make([]string, n)
			for i, ac := range decision.ActionCalls {
				d := firePreToolCall(e.callbacks, ctx, ac.Name, ac.Params)
				if d != nil {
					if len(d.RewriteParams) > 0 {
						// 改写参数前把原始调用记入审计（若审计钩子存在且启用）。
						if e.auditHook != nil && e.auditHook.IsEnabled() {
							_, _ = e.auditHook.Append(&audit.AuditEntry{
								Timestamp: time.Now(),
								Source:    "agent",
								Action:    "pre_tool_call_rewrite",
								Input:     ac,
								Output:    d.RewriteParams,
								Metadata:  map[string]string{"action_name": ac.Name},
							})
						}
						ac.Params = d.RewriteParams
					}
					if d.AppendContext != "" {
						preContexts[i] = d.AppendContext
					}
					if d.Block {
						reason := d.BlockReason
						if reason == "" {
							reason = "blocked by PreToolCall hook"
						}
						// 与 approval 拦截同形的 blocked 结果：OK=false、
						// Status="blocked"、Error=阻断原因（模型可读）。
						preBlocked[i] = &action.ActionResult{
							OK:     false,
							Status: "blocked",
							Error:  reason,
						}
						continue
					}
				}
				pendingCalls = append(pendingCalls, ac)
				pendingIdx = append(pendingIdx, i)
			}
		} else {
			pendingCalls = decision.ActionCalls
		}

		// Fire OnToolCallStart callbacks for each action.
		for _, ac := range decision.ActionCalls {
			fireOnToolCallStart(e.callbacks, ctx, ac.Name)
		}

		// R3：在派发前记录每个 tool_call。audit_record_id 在派发执行阶段
		// 才生成、不回流到本处，故此处留空（见下方 tool_result 处：仅当
		// 结果携带 recordID 时填充）。
		for _, ac := range decision.ActionCalls {
			e.recordRollout(session.RolloutItem{Type: session.RolloutToolCall, ToolName: ac.Name, Params: ac.Params})
		}

		// Execute actions, using ExecuteInterruptible when a preempt channel
		// is available so that a force-cancel can abort in-flight tools.
		var results []*action.ActionResult
		var toolPreempted bool
		if preemptCh != nil {
			results, toolPreempted = dispatcher.ExecuteInterruptible(ctx, pendingCalls, preemptCh)
		} else {
			results = dispatcher.Execute(ctx, pendingCalls)
		}

		// 把 Pre 阶段的 blocked 结果与派发结果按原始顺序合并，保持
		// results 与 decision.ActionCalls 的下标对齐（后续的回调循环与
		// session 写入均依赖该对齐）。toolPreempted 路径在上方已返回。
		if hasPreHook {
			aligned := make([]*action.ActionResult, len(decision.ActionCalls))
			copy(aligned, preBlocked)
			for j, idx := range pendingIdx {
				if j < len(results) {
					aligned[idx] = results[j]
				}
			}
			results = aligned
		}

		// When a preempt was triggered during tool execution, capture
		// the snapshot and exit the loop.
		if toolPreempted {
			e.capturePreemptState(round, toolCallRounds)
			reason := ""
			if e.turnLoop != nil {
				reason = e.turnLoop.PreemptReason()
			}
			if e.preemptDrainNext() {
				prefixSet = false
				halfwayWarned = false
				continue // new turn processes the queued input
			}
			return nil, fmt.Errorf("agent preempted during tool execution: %s: %w", reason, ErrTurnInterrupted)
		}

		// Fire OnToolCallEnd callbacks for each action.
		// When a result is blocked, fire OnApprovalRequired instead.
		for i, ac := range decision.ActionCalls {
			if i < len(results) && results[i] != nil {
				res := results[i]
				if res.Status == "blocked" {
					// Extract recordID from error or metadata.
					recordID := ""
					if res.Metadata != nil {
						if rid, ok := res.Metadata["recordID"]; ok {
							recordID = fmt.Sprintf("%v", rid)
						}
					}
					if recordID == "" {
						recordID = res.Error
					}
					fireOnApprovalRequired(e.callbacks, ctx, ac.Name, recordID)
				}
				var toolErr error
				if !res.OK {
					toolErr = fmt.Errorf("%s", res.Error)
				}
				fireOnToolCallEnd(e.callbacks, ctx, ac.Name, toolErr)
			} else {
				fireOnToolCallEnd(e.callbacks, ctx, ac.Name, nil)
			}
		}

		// PostToolCall 干预：对每个结果调用 PostToolCall 钩子，并把
		// Pre/Post 的附加上下文拼接到结果内容（nil/零值 = 不干预、
		// 不拼接），使上下文随工具结果进入下一轮 LLM 输入。
		hasPostHook := e.callbacks != nil && e.callbacks.PostToolCall != nil
		if hasPreHook || hasPostHook {
			for i, ac := range decision.ActionCalls {
				if i >= len(results) || results[i] == nil {
					continue
				}
				var ctxParts []string
				if hasPreHook && preContexts[i] != "" {
					ctxParts = append(ctxParts, preContexts[i])
				}
				if hasPostHook {
					if fb := firePostToolCall(e.callbacks, ctx, ac.Name, results[i]); fb != nil && fb.AppendContext != "" {
						ctxParts = append(ctxParts, fb.AppendContext)
					}
				}
				if len(ctxParts) > 0 {
					results[i] = appendResultContext(results[i], strings.Join(ctxParts, "\n"))
				}
			}
		}

		// R3：记录每个 tool_result。此时 results 已按原始下标对齐到
		// decision.ActionCalls，并已包含 Pre/Post 拼接的附加上下文。
		// audit_record_id 仅在结果本身携带 recordID 时填充——普通派发路径
		// 的 audit 记录 ID 不回流到引擎，故留空（如 approval / Pre 阻断
		// 路径通过结果 metadata 带出 recordID 时则记录）。
		for i, ac := range decision.ActionCalls {
			if i >= len(results) || results[i] == nil {
				continue
			}
			res := results[i]
			item := session.RolloutItem{Type: session.RolloutToolResult, ToolName: ac.Name}
			if res.Error != "" {
				item.Error = res.Error
			} else {
				item.Result = formatToolResult(res)
			}
			if res.Metadata != nil {
				if rid, ok := res.Metadata["recordID"]; ok {
					item.AuditRecordID = fmt.Sprintf("%v", rid)
				}
			}
			e.recordRollout(item)
		}

		// Add results to session using native tool message format when
		// native tool calls were used, or legacy system-message format
		// for the custom-decision fallback.
		if len(nativeToolCalls) > 0 {
			// Native function calling: add role="tool" messages with tool_call_id.
			for i, tc := range nativeToolCalls {
				if i < len(results) {
					resultContent := formatToolResult(results[i])
					e.session.AddToolResultNamed(tc.ID, tc.Name, resultContent)
				}
			}
		} else {
			// Legacy custom-decision: add as system messages.
			for i, call := range decision.ActionCalls {
				if i < len(results) {
					e.session.AddActionResult(call.Name, results[i])
				}
			}
		}

		// Tool-call stale tracking is now handled pre-execution above.
		// (Post-execution nudge logic removed: 3rd identical call is a hard stop.)

		// Halfway warning: when tool-call rounds reach 50%% of the hard cap,
		// inject a system message encouraging the model to wrap up.
		if !halfwayWarned && toolCallRounds >= maxTCR/2 {
			halfwayWarned = true
			warning := fmt.Sprintf(
				"[system] You have used %d of %d allowed tool-call rounds. "+
					"If you have gathered enough context, begin writing files or "+
					"provide your final summary now. Do NOT continue exploring.",
				toolCallRounds, maxTCR,
			)
			log.Printf("[agent] halfway warning at %d/%d tool-call rounds", toolCallRounds, maxTCR)
			// Use user message instead of system to avoid "System message must be at the beginning" API error.
			e.session.AddUserMessage(warning)
		}

		// Point 6: CancelAfterToolCalls safe-point. The tool batch has
		// completed and results are in the session; honor a pending
		// safe-point cancel before the next iteration. CancelImmediate also
		// matches here.
		if e.cancelManager != nil {
			e.cancelManager.CheckTimeoutEscalation()
			if e.cancelManager.HasPendingCancel() && e.cancelManager.CheckCancel(CancelAfterToolCalls) {
				e.cancelManager.CompleteCancel(nil)
				e.capturePreemptState(round, toolCallRounds)
				if e.preemptDrainNext() {
					prefixSet = false
					halfwayWarned = false
					continue // new turn processes the queued input
				}
				return nil, fmt.Errorf("%w", ErrTurnInterrupted)
			}
		}

		// Point 7: return to idle at the end of the round. The defer at the
		// top of executeLoop also handles this for early-return paths.
		if e.turnLoop != nil {
			e.turnLoop.EnterIdle()
		}

		// Drain input queue at turn boundary. When a queued request is
		// found, add its message to the session and continue the loop
		// so the agent processes it in a new turn.
		if e.inputQueue != nil {
			if req, ok := e.inputQueue.Dequeue(); ok {
				e.session.AddUserMessage(req.Message)
				// Reset per-loop state for the new turn.
				prefixSet = false
				halfwayWarned = false
				// Save the request so we can send the response back
				// via ResponseCh when this turn completes.
				e.pendingInterleave = &req
				continue
			}
		}

		// Clear Zone 3 (volatile scratch) so per-round reasoning state does
		// not leak into the next round. No-op for backends without zones.
		e.session.ClearVolatileScratch()

		// Only increment the main round counter for "response" decisions.
		// Tool-execution rounds (native function calls or custom action
		// dispatch) do NOT count against maxRounds — the agent may need
		// many tool calls to fulfil a single user request, and capping
		// the total rounds would prematurely truncate legitimate workflows.
		if decision.NextAction != "execute" {
			round++
		} else {
			// Tool-execution round: count toward the hard cap.
			toolCallRounds++
			if toolCallRounds >= maxTCR {
				log.Printf("[agent] tool-call cap reached (%d rounds); triggering synthesis",
					toolCallRounds)
				// Return an empty-execute decision so RunLoop/Agent.Run
				// triggers the synthesis fallback instead of erroring.
				return &actionruntime.Decision{
					NextAction:    "execute",
					FinalResponse: "",
				}, nil
			}
		}
	}
}

// buildToolDefinitions creates ToolDefinition list from registered actions.
// All map lookups use the comma-ok pattern so a missing or wrongly-typed
// field cannot panic the loop; malformed entries are skipped (name empty
// after coercion) or fall back to safe zero values.
//
// The returned slice is sorted by Name and serialized via model.MarshalStable
// so the byte representation is deterministic across calls (critical for
// prefix cache hits). The SHA-256 of the stable bytes is cached on the
// Engine as toolDefsHash for cache invalidation detection.
//
// When the computed hash matches the previously cached hash, the cached
// tool definitions are returned directly, avoiding redundant allocation
// and sorting on every loop iteration.
func (e *Engine) buildToolDefinitions() []model.ToolDefinition {
	actions := e.actionExt.ListActions()
	tools := make([]model.ToolDefinition, 0, len(actions))
	for _, a := range actions {
		name, _ := a["name"].(string)
		if name == "" {
			// Without a name the tool is uncallable; skip rather than
			// emit a tool the model cannot meaningfully reference.
			continue
		}
		description, _ := a["description"].(string)
		schema, _ := a["schema"].(map[string]any)
		tools = append(tools, model.ToolDefinition{
			Name:        name,
			Description: description,
			Parameters:  schema,
		})
	}
	// Sort by name for stable ordering. ListActions already returns names in
	// sorted order, but sorting here is defensive and makes the invariant
	// explicit so future refactors of ListActions cannot break cache stability.
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	// Compute stable hash for cache invalidation detection. The hash is the
	// SHA-256 of the byte-stable JSON serialization of the tools slice.
	if stableBytes, err := model.MarshalStable(tools); err == nil {
		newHash := fmt.Sprintf("%x", sha256.Sum256(stableBytes))
		e.toolDefsHash = newHash
		// Cache invalidation: if the hash matches the cached version,
		// return the cached tools to avoid redundant allocation.
		if newHash == e.cachedToolsHash && e.cachedTools != nil {
			return e.cachedTools
		}
		e.cachedTools = tools
		e.cachedToolsHash = newHash
	}
	return tools
}

// ToolDefsHash returns the SHA-256 hash of the most recently built tool
// definitions (byte-stable serialization). Returns empty string if
// buildToolDefinitions has not been called yet. Two engines with the same
// hash can share a prefix cache for Zone 1 (immutable prefix).
func (e *Engine) ToolDefsHash() string {
	return e.toolDefsHash
}

// defaultToolResultMaxBytes is the maximum byte size of a tool result
// content before it is truncated. 4096 bytes keeps individual tool results
// (especially file_read) from dominating the context window.
// 16KB: directory listings of real workspaces run 200+ entries (~15KB JSON);
// a 4KB cap silently dropped half the listing and the model "could not see"
// files that exist. Tool results are per-call; the session window compresses.
const defaultToolResultMaxBytes = 16 << 10

// truncateToolResult shortens s to at most maxBytes bytes. When truncation
// occurs the head (first half) and tail (last quarter) are preserved with a
// marker indicating the original size. This prevents a single large tool
// result (e.g. a 27 KB file_read) from consuming the entire context budget.
func truncateToolResult(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	head := s[:maxBytes/2]
	tail := s[len(s)-maxBytes/4:]
	return head + "\n... [truncated, " + strconv.Itoa(len(s)) + " bytes total] ...\n" + tail
}

// formatToolResult converts an *action.ActionResult into a human-readable
// string suitable for sending back to the model as a tool message content.
// Results exceeding defaultToolResultMaxBytes are truncated to keep the
// context window compact.
func formatToolResult(result *action.ActionResult) string {
	var raw string
	if result == nil {
		raw = "null"
	} else if result.Error != "" {
		raw = fmt.Sprintf("error: %s", result.Error)
	} else if b, err := json.Marshal(result.Result); err == nil {
		raw = string(b)
	} else {
		raw = fmt.Sprintf("%v", result.Result)
	}
	return truncateToolResult(raw, defaultToolResultMaxBytes)
}

// appendResultContext 返回追加了附加上下文的结果副本（不修改原对象）。
// 上下文随结果内容进入 session，供下一轮 LLM 调用读取。
func appendResultContext(res *action.ActionResult, context string) *action.ActionResult {
	if res == nil || context == "" {
		return res
	}
	cp := *res
	if cp.Error != "" {
		cp.Error = cp.Error + "\n" + context
	} else {
		cp.Result = fmt.Sprintf("%v\n%s", cp.Result, context)
	}
	return &cp
}

// truncate returns the first n characters of s, or s itself if shorter.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// capturePreemptState takes a snapshot of the current TurnLoop state and
// stores it in lastPreemptState. Safe to call even when turnLoop is nil.
func (e *Engine) capturePreemptState(round, toolCallRounds int) {
	if e.turnLoop == nil {
		return
	}
	st := e.turnLoop.Snapshot(round, toolCallRounds, 0)
	e.lastPreemptState = &st
}

// recordRollout 把一条 Rollout item 追加到会话级记录器，绕过时零成本。
// 记录器为 nil 时该方法立即返回（向后兼容硬约束，不产生任何副作用）。
// SessionID / Seq / Timestamp 由 recorder.Record 内部填充，这里只传类型
// 与业务字段。属于 R3 的弱耦合接线点，失败（如落盘错误）不打断主流程。
func (e *Engine) recordRollout(item session.RolloutItem) {
	if e.rollout == nil {
		return
	}
	_ = e.rollout.Record(item)
}
