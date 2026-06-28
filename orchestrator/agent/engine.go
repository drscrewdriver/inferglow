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
	"github.com/inferglow/model"
	"github.com/inferglow/orchestrator/actionruntime"
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

	// cacheBudgetHook is called after each LLM response with the
	// cached_tokens count from UsageInfo. nil disables (default).
	// Propagated from Agent.Run via runConfig.cacheBudgetUpdater.
	cacheBudgetHook func(cachedTokens int)

	// inputQueue is the bounded FIFO queue for user inputs submitted while
	// the agent is busy. nil disables queue draining (default).
	// Propagated from Agent.inputQueue.
	inputQueue *InputQueue

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
	data, err := e.modelReq.GenerateRequestData(ctx, synthReq)
	if err != nil {
		return "", err
	}
	synthCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	stream, err := e.modelReq.RequestModel(synthCtx, data)
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

// executeLoop runs the PLAN → EXECUTE loop until the LLM returns a response
// or maxRounds is reached.
func (e *Engine) executeLoop(ctx context.Context, userMessage string, maxRounds int, systemPrompt string) (dec *actionruntime.Decision, err error) {
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
	e.session.AddUserMessage(userMessage)

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

	round := 0
	toolCallRounds := 0
	maxTCR := e.maxToolCallRounds
	if maxTCR <= 0 {
		maxTCR = DefaultMaxToolCallRounds
	}
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
				Round:       round,
				ActionCalls: prevCalls,
				LastOutput:  prevOutput,
				TotalTokens: totalTokens,
				StartedAt:   runStart,
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
				return nil, fmt.Errorf("agent cancelled")
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
				if hasTools {
					// With tools, skip force_json to allow native function calling.
					// response_format conflicts with tool_calls in OpenAI-compatible APIs.
					// Increase max_tokens for agent loops to handle large tool arguments
					// (e.g., code_executor with multi-KB source code).
					return map[string]any{"max_tokens": 16384}
				}
				return map[string]any{"force_json": true}
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
		data, err := e.modelReq.GenerateRequestData(ctx, req)
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
		stream, err := e.modelReq.RequestModel(timeoutCtx, data)
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
				// G1-02: accumulate reasoning content for DeepSeek/MiMo passback.
				if chunk.Reasoning != "" {
					reasoning.WriteString(chunk.Reasoning)
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
				return nil, fmt.Errorf("agent preempted: %s", reason)
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
				retryData, rErr := e.modelReq.GenerateRequestData(ctx, req)
				if rErr != nil {
					return nil, rErr
				}
				retryTimeoutCtx, cancelRetry := context.WithTimeout(ctx, streamTimeout)
				defer cancelRetry()
				retryStream, rErr := e.modelReq.RequestModel(retryTimeoutCtx, retryData)
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

		// Approximate token accumulation: prefer provider-reported UsageInfo
		// when available (more accurate), fall back to len-based estimate.
		if lastUsage != nil && lastUsage.CompletionTokens > 0 {
			totalTokens += lastUsage.CompletionTokens
		} else {
			totalTokens += len(content.String())
		}

		// Fire OnLLMCallEnd callback.
		fireOnLLMCallEnd(e.callbacks, ctx, round, len(content.String()))

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
			_, _ = e.auditHook.Append(&audit.AuditEntry{
				Timestamp: time.Now(),
				Source:    "agent",
				Action:    "decision",
				Input:     input,
				Output:    decision,
				Metadata: map[string]string{
					"round": strconv.Itoa(round),
				},
			})
		}

		prevDecision = decision
		prevOutput = content.String()

		// Check if we should continue
		if !actionruntime.ShouldContinue(*decision, round, maxRounds) {
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

		// Execute actions. We always use NewActionDispatcherWithAudit so the
		// per-action audit entries flow through the same hook as decisions.
		// With a NoOpHook or nil hook this is zero overhead.
		dispatcher := actionruntime.NewActionDispatcherWithAudit(e.actionExt.GetRegistry(), e.auditHook)

		// Fire OnToolCallStart callbacks for each action.
		for _, ac := range decision.ActionCalls {
			fireOnToolCallStart(e.callbacks, ctx, ac.Name)
		}

		// Execute actions, using ExecuteInterruptible when a preempt channel
		// is available so that a force-cancel can abort in-flight tools.
		var results []*action.ActionResult
		var toolPreempted bool
		if preemptCh != nil {
			results, toolPreempted = dispatcher.ExecuteInterruptible(ctx, decision.ActionCalls, preemptCh)
		} else {
			results = dispatcher.Execute(ctx, decision.ActionCalls)
		}

		// When a preempt was triggered during tool execution, capture
		// the snapshot and exit the loop.
		if toolPreempted {
			e.capturePreemptState(round, toolCallRounds)
			reason := ""
			if e.turnLoop != nil {
				reason = e.turnLoop.PreemptReason()
			}
			return nil, fmt.Errorf("agent preempted during tool execution: %s", reason)
		}

		// Fire OnToolCallEnd callbacks for each action.
		for i, ac := range decision.ActionCalls {
			var toolErr error
			if i < len(results) && results[i] != nil && !results[i].OK {
				toolErr = fmt.Errorf("%s", results[i].Error)
			}
			fireOnToolCallEnd(e.callbacks, ctx, ac.Name, toolErr)
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

		// Tool-call dedup: detect when the model is stuck calling the same
		// tool with the same arguments. Build a signature from the current
		// batch and compare with the previous one.
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
			if staleCount == toolCallStaleThreshold {
				nudge := fmt.Sprintf(
					"[system nudge] You have called the same tool with the same arguments %d times in a row. "+
						"This is likely unproductive. Please change your approach: either use different tools, "+
						"different arguments, or provide your final response/summary now.",
					staleCount,
				)
				log.Printf("[agent] stale tool-call detected (%d consecutive identical calls); injecting nudge", staleCount)
				// Use user message instead of system to avoid "System message must be at the beginning" API error.
				e.session.AddUserMessage(nudge)
			}
			if staleCount > 0 && staleCount%toolCallStaleThreshold == 0 && staleCount > toolCallStaleThreshold {
				// Escalating nudge for persistent loops
				nudge := fmt.Sprintf(
					"[system nudge] WARNING: You are stuck in a loop. You have made %d identical tool-call rounds. "+
						"STOP calling tools and provide your FINAL RESPONSE now based on what you already know.",
					staleCount,
				)
				log.Printf("[agent] persistent stale loop (%d rounds); injecting escalation", staleCount)
				// Use user message instead of system to avoid "System message must be at the beginning" API error.
				e.session.AddUserMessage(nudge)
			}
		}

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
				return nil, fmt.Errorf("agent cancelled after tool calls")
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
				// Send the response back via the request's channel
				// after the next iteration produces it. For now,
				// just continue the loop; the ResponseCh will be
				// sent to when the turn completes.
				_ = req // ResponseCh is handled at turn completion
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
const defaultToolResultMaxBytes = 4096

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
