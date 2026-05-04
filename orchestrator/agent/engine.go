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
	"github.com/inferglow/orchestrator/actionruntime"
)

// Engine orchestrates the PLAN → EXECUTE loop.
type Engine struct {
	session   *SessionExtension
	actionExt *ActionExtension
	modelReq  model.ModelRequester
	auditHook audit.AuditHook
	loopGuard *LoopGuard

	// toolDefsHash is the SHA-256 of the byte-stable serialization of the
	// current tool definitions (sorted by Name). It exists so callers can
	// detect when the tool set has changed and invalidate prefix caches.
	// Populated lazily by buildToolDefinitions.
	toolDefsHash string
}

// NewEngine creates an Engine with the given components. The returned Engine
// has a NoOpHook (zero-overhead) and no LoopGuard, matching the pre-audit
// behavior exactly.
func NewEngine(sess *SessionExtension, actExt *ActionExtension, mr model.ModelRequester) *Engine {
	return &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mr,
		auditHook: &audit.NoOpHook{},
		loopGuard: nil,
	}
}

// NewEngineWithAudit creates an Engine that appends decision audit entries
// via hook. The loopGuard is nil. A nil hook is replaced with NoOpHook.
func NewEngineWithAudit(sess *SessionExtension, actExt *ActionExtension, mr model.ModelRequester, hook audit.AuditHook) *Engine {
	if hook == nil {
		hook = &audit.NoOpHook{}
	}
	return &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mr,
		auditHook: hook,
		loopGuard: nil,
	}
}

// NewEngineWithLoopGuard creates an Engine that consults guard before each
// LLM call. The audit hook defaults to NoOpHook.
func NewEngineWithLoopGuard(sess *SessionExtension, actExt *ActionExtension, mr model.ModelRequester, guard *LoopGuard) *Engine {
	return &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mr,
		auditHook: &audit.NoOpHook{},
		loopGuard: guard,
	}
}

// NewEngineWithAuditAndLoopGuard creates an Engine with both an AuditHook
// and a LoopGuard. Either may be nil to disable that feature; a nil hook is
// replaced with NoOpHook.
func NewEngineWithAuditAndLoopGuard(sess *SessionExtension, actExt *ActionExtension, mr model.ModelRequester, hook audit.AuditHook, guard *LoopGuard) *Engine {
	if hook == nil {
		hook = &audit.NoOpHook{}
	}
	return &Engine{
		session:   sess,
		actionExt: actExt,
		modelReq:  mr,
		auditHook: hook,
		loopGuard: guard,
	}
}

// executeLoop runs the PLAN → EXECUTE loop until the LLM returns a response
// or maxRounds is reached.
func (e *Engine) executeLoop(ctx context.Context, userMessage string, maxRounds int, systemPrompt string) (*actionruntime.Decision, error) {
	// Add user message to session
	e.session.AddUserMessage(userMessage)

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
								"name": map[string]any{"type": "string"},
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
		}

		// Call LLM
		data, err := e.modelReq.GenerateRequestData(ctx, req)
		if err != nil {
			return nil, err
		}

		stream, err := e.modelReq.RequestModel(ctx, data)
		if err != nil {
			return nil, err
		}

		// Collect response content
		var content strings.Builder
		for chunk := range stream {
			content.WriteString(chunk.Delta)
		}

		// Approximate token accumulation: count characters in this round's
		// LLM output as a simple proxy. The model package's StreamChunk.Usage
		// is *UsageInfo and may be nil; this char-count proxy keeps the
		// engine decoupled from provider-specific usage shapes.
		totalTokens += len(content.String())

		// Parse decision
		decision, err := actionruntime.ParseDecision(content.String())
		if err != nil {
			return nil, err
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
