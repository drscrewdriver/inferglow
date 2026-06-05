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
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/inferglow/audit"
	"github.com/inferglow/model"
	"github.com/inferglow/observability/otel"
	"github.com/inferglow/orchestrator/actionruntime"
)

// Engine orchestrates the PLAN → EXECUTE loop.
type Engine struct {
	session   *SessionExtension
	actionExt *ActionExtension
	modelReq  model.ModelRequester
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
	tracer *otel.Tracer
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
func NewEngine(sess *SessionExtension, actExt *ActionExtension, mr model.ModelRequester) *Engine {
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
	return decision.FinalResponse, nil
}

// NewEngineWithAudit creates an Engine that appends decision audit entries
// via hook. The loopGuard is nil. A nil hook is replaced with NoOpHook.
func NewEngineWithAudit(sess *SessionExtension, actExt *ActionExtension, mr model.ModelRequester, hook audit.AuditHook) *Engine {
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
func NewEngineWithLoopGuard(sess *SessionExtension, actExt *ActionExtension, mr model.ModelRequester, guard *LoopGuard) *Engine {
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
func NewEngineWithAuditAndLoopGuard(sess *SessionExtension, actExt *ActionExtension, mr model.ModelRequester, hook audit.AuditHook, guard *LoopGuard) *Engine {
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

// executeLoop runs the PLAN → EXECUTE loop until the LLM returns a response
// or maxRounds is reached.
func (e *Engine) executeLoop(ctx context.Context, userMessage string, maxRounds int, systemPrompt string) (*actionruntime.Decision, error) {
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
		req := &model.ModelRequest{
			System:      systemPrompt,
			ChatHistory: e.session.PreparePrompt(),
			Tools:       tools,
			Output: &model.OutputSchema{
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
			},
			// O-CRITICAL-1: hint to OpenAI-compatible providers that we
			// expect a JSON object so they set response_format in the
			// request body. Providers that don't support response_format
			// (Anthropic, Ollama) simply ignore this option.
			Options: map[string]any{
				"force_json": true,
			},
		}

		// L3 prompt injection: when outputSchema is configured and the
		// provider cannot enforce json_schema-level response_format (no
		// response_format set, or explicitly degraded to json_object),
		// append a schema description to the system prompt so the model
		// has prompt-level guidance for the expected JSON structure.
		if e.outputSchema != nil && shouldInjectSchemaPrompt(req) {
			req.System += formatSchemaInstruction(req.Output)
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

		// Collect response content
		var content strings.Builder
	streamLoop:
		for {
			select {
			case chunk, ok := <-stream:
				if !ok {
					break streamLoop
				}
				content.WriteString(chunk.Delta)
			case <-timeoutCtx.Done():
				cancelTimeout()
				return nil, timeoutCtx.Err()
			case <-preemptCh:
				// Point 3: preempted (CancelImmediate or timeout escalation).
				// Complete any pending cancel so its handle unblocks, then
				// surface the preempt as an error. CompleteCancel preserves
				// an ErrCancelTimeout already set by CheckTimeoutEscalation.
				cancelTimeout()
				if e.cancelManager != nil && e.cancelManager.HasPendingCancel() {
					e.cancelManager.CompleteCancel(nil)
				}
				reason := ""
				if e.turnLoop != nil {
					reason = e.turnLoop.PreemptReason()
				}
				return nil, fmt.Errorf("agent preempted: %s", reason)
			}
		}
		cancelTimeout()

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

		// Approximate token accumulation: count characters in this round's
		// LLM output as a simple proxy. The model package's StreamChunk.Usage
		// is *UsageInfo and may be nil; this char-count proxy keeps the
		// engine decoupled from provider-specific usage shapes.
		totalTokens += len(content.String())

		// Parse decision
		decision, err := actionruntime.ParseDecision(content.String())
		if err != nil {
			// O-MEDIUM-1: Planning fallback strategy. When the LLM emits
			// content that cannot be parsed as a structured decision (pure
			// prose, empty, or irreparably malformed JSON), degrade to a
			// "response" decision whose FinalResponse is the raw LLM output
			// so the loop can still terminate and surface the model's reply
			// to the user instead of failing the whole Run.
			decision = &actionruntime.Decision{
				NextAction:    "response",
				FinalResponse: content.String(),
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
		results := dispatcher.Execute(ctx, decision.ActionCalls)

		// Add results to session
		for i, call := range decision.ActionCalls {
			if i < len(results) {
				e.session.AddActionResult(call.Name, results[i])
			}
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

		round++
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
		e.toolDefsHash = fmt.Sprintf("%x", sha256.Sum256(stableBytes))
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
