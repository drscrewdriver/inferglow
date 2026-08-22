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
	"errors"
	"fmt"
	"time"

	"github.com/inferglow/audit"
	"github.com/inferglow/flow"
	"github.com/inferglow/model"
	"github.com/inferglow/orchestrator/middleware"
	"github.com/inferglow/session"
)

// ErrNoFinalResponse indicates the LLM decided to execute actions but provided no final response.
var ErrNoFinalResponse = errors.New("LLM decided to execute actions but provided no final_response")

// PIIMasker redacts sensitive (PII) content from text on both the input
// side (before it enters the session history) and the output side (before
// the final response is returned to the caller). Implementations are
// expected to inspect their own configuration and return the text
// unchanged when masking is disabled for the relevant side.
//
// The method set of PIIMasker matches session.MessageMasker so a value
// satisfying PIIMasker can be wired into the session for input masking
// via SessionExtension.SetMessageMasker. The concrete implementation
// backed by github.com/inferglow/security/pii lives in
// github.com/inferglow/security/agenthook to keep the orchestrator free
// of a direct security dependency.
type PIIMasker interface {
	// MaskInput transforms a message before it enters the session
	// history. Returning the input unchanged disables input masking.
	MaskInput(text string) string
	// MaskOutput transforms the final response before it is returned to
	// the caller. Returning the input unchanged disables output
	// masking.
	MaskOutput(text string) string
}

// CacheBudgetUpdater receives cached_tokens feedback from LLM responses
// and adjusts context management thresholds accordingly. The minimal
// interface avoids a direct import of the context package.
type CacheBudgetUpdater interface {
	UpdateCacheBudget(cachedTokens int)
}

// Agent is the user-facing entry point for interacting with inferglow.
// It encapsulates Session, Action management, and LLM orchestration.
type Agent struct {
	session   *SessionExtension
	actionExt *ActionExtension
	engine    *Engine
	// maxRounds is the persisted default for executeLoop. It is set via
	// WithMaxRounds passed to New and used by Run when the caller does
	// not override it with a per-call WithMaxRounds. A zero value means
	// "use the runConfig default of 10".
	maxRounds int
	// systemPrompt is the persisted default system prompt. It is set via
	// WithSystemPrompt passed to New and used by Run when the caller
	// does not override it with a per-call WithSystemPrompt.
	systemPrompt string
	// streamTimeout is the persisted default for the per-stream timeout
	// in executeLoop. Set via WithStreamTimeout on New; overridden by a
	// per-call WithStreamTimeout on Run. Zero means "use the engine's
	// default 5-minute timeout".
	streamTimeout time.Duration
	// outputHook is the persisted default output-side security hook.
	// Set via WithOutputSecurityHook on New; overridden by a per-call
	// WithOutputSecurityHook on Run. nil disables output scanning.
	outputHook OutputSecurityHook
	// piiMasker is the persisted default PII masker. Set via
	// WithPIIMasker on New; overridden by a per-call WithPIIMasker on
	// Run. nil disables PII masking. When non-nil it is propagated to
	// the session (input masking via MaskInput) and consulted on the
	// final response (output masking via MaskOutput) before Run
	// returns. Ignored when features.PIIMasking is false.
	piiMasker PIIMasker
	// features toggles optional orchestrator capabilities (PII masking,
	// prompt-injection scanning, sandbox, OpenTelemetry, audit chain).
	// Set via WithFeatures on New; overridden by a per-call WithFeatures
	// on Run. When WithFeatures is not supplied to New the Agent uses
	// DefaultFeatures. The effective features are validated at the start
	// of Run; an invalid configuration (e.g. Sandbox without the
	// with_sandbox build tag) causes Run to return an error before the
	// execute loop runs.
	features Features
	// tracer 是可选的 OpenTelemetry tracer。nil 时禁用 flow/agent 内的
	// span 埋点。通过 WithTracer on New 持久化为 Agent 默认值；通过
	// WithTracer on Run 做 per-call 覆盖。executeFlow 会用该 tracer 创建
	// SpanFlowExecute / SpanPause / SpanResume 等语义 span，并把它注入
	// flowContextImpl 让 step 可以自建 SpanKindStep / SpanKindTool span。
	tracer SpanStarter
	// flow 是可选的 flow 编排定义。nil 时使用 executeLoop（oneshot 模式）；
	// 非 nil 时使用 executeFlow（flow 编排模式）。
	// 通过 WithFlow RunOption 设置。
	flow *flow.Flow
	// outputSchema is the optional L4 output schema used for post-validation
	// of the LLM response. Set via WithOutputSchema on New; overridden by a
	// per-call WithOutputSchema on Run. nil disables L4 validation (default).
	outputSchema *model.OutputSchema
	// rateLimitHook is the persisted default rate limit hook. Set via
	// WithRateLimitHook on New; overridden by a per-call WithRateLimitHook
	// on Run. nil disables rate limiting (default).
	rateLimitHook RateLimitHook
	// callbacks is the persisted default lifecycle callbacks. Set via
	// WithCallbacks on New; overridden by a per-call WithCallbacks on Run.
	// nil disables callbacks (default).
	callbacks *AgentCallbacks
	// rollout is the persisted default session-level Rollout recorder.
	// Set via WithRollout on New; overridden by a per-call WithRollout on
	// Run. nil disables rollout recording (default, zero overhead).
	rollout *session.RolloutRecorder
	// middlewares is the persisted list of unified middlewares.
	// Set via WithMiddleware on New; overridden by a per-call WithMiddleware
	// on Run. Empty means the core handler is called directly (zero overhead).
	middlewares []middleware.Middleware
	// cacheBudgetUpdater receives cached_tokens feedback from LLM responses
	// and adjusts context management sweet-spot thresholds. nil disables.
	cacheBudgetUpdater CacheBudgetUpdater
	// compactHook is called after each LLM turn with promptTokens
	// to trigger ModeSummary compaction. nil disables (default).
	compactHook func(promptTokens int)
	// inputQueue is the bounded FIFO queue for user inputs submitted while
	// the agent is busy. nil disables queue mode (default).
	inputQueue *InputQueue
}

// RunOption configures Agent.Run behavior.
type RunOption func(*runConfig)

type runConfig struct {
	maxRounds     int
	systemPrompt  string
	streamTimeout time.Duration
	// auditHook optionally records decision audit entries. nil means
	// use the default NoOpHook (zero overhead).
	auditHook audit.AuditHook
	// outputHook optionally scans the LLM's final response for
	// prompt-injection content before Run returns it.
	outputHook OutputSecurityHook
	// piiMasker optionally redacts PII from user input (via the session)
	// and from the final response (via MaskOutput) before Run returns.
	// Ignored when features.PIIMasking is false.
	piiMasker PIIMasker
	// rateLimitHook optionally checks rate limits before RequestModel.
	rateLimitHook RateLimitHook
	// features toggles optional capabilities for this run. Populated
	// from the Agent default (or DefaultFeatures when unset on New) and
	// optionally overridden by a per-call WithFeatures.
	features Features
	// featuresSet records whether features was explicitly set via
	// WithFeatures, so New can distinguish "use default" from an
	// explicit zero-value Features.
	featuresSet bool
	// tracer 是可选的 OpenTelemetry tracer。nil 禁用 span 埋点。
	// 由 executeFlow 用来创建 SpanFlowExecute / SpanPause / SpanResume
	// 并注入 flowContextImpl 供 step 自建 span。
	tracer SpanStarter
	// flow 可选的 flow 编排定义（per-call 覆盖）。
	flow *flow.Flow
	// outputSchema is the optional L4 output schema for post-validation
	// of the LLM response. Populated from the Agent default and optionally
	// overridden by a per-call WithOutputSchema. nil disables L4 validation.
	outputSchema *model.OutputSchema
	// middlewares is the list of unified middlewares. Empty means
	// the core handler is called directly (zero overhead).
	middlewares []middleware.Middleware
	// callbacks provides lifecycle hooks for observability. nil disables.
	callbacks *AgentCallbacks
	// rollout 是可选的会话级 Rollout 记录器。nil 禁用（默认，零开销，
	// 零行为变化）。经 RunOption WithRollout 注入。
	rollout *session.RolloutRecorder
	// cacheBudgetUpdater receives cached_tokens feedback. nil disables.
	cacheBudgetUpdater CacheBudgetUpdater
	// compactHook is called after each LLM turn for ModeSummary compaction.
	compactHook func(promptTokens int)
	// contentBlocks carries multimodal input (image/audio/video) for the
	// user message of this run. Empty disables (pure-text, zero behavior
	// change). Populated via WithContentBlocks.
	contentBlocks []model.ContentBlock
}

// WithMaxRounds sets the maximum number of PLAN → EXECUTE loop iterations.
func WithMaxRounds(n int) RunOption {
	return func(c *runConfig) {
		c.maxRounds = n
	}
}

// WithSystemPrompt sets the system prompt for the LLM.
func WithSystemPrompt(prompt string) RunOption {
	return func(c *runConfig) {
		c.systemPrompt = prompt
	}
}

// WithContentBlocks attaches multimodal content blocks (image/audio/video)
// to the user message of this run. When non-empty they are combined with the
// text userMessage into a single multimodal user message; the model layer
// (model.ChatMessage.ContentBlocks + provider serialization) handles encoding.
func WithContentBlocks(blocks []model.ContentBlock) RunOption {
	return func(c *runConfig) {
		c.contentBlocks = blocks
	}
}

// WithStreamTimeout sets the maximum duration executeLoop will wait for
// the next chunk from the model stream. A zero value means "use the
// engine default (5 minutes)". Setting a shorter timeout protects callers
// from stuck streams; setting a longer timeout accommodates slow providers.
func WithStreamTimeout(d time.Duration) RunOption {
	return func(c *runConfig) {
		c.streamTimeout = d
	}
}

// WithFlow 设置 flow 编排定义。设置后 Agent.Run 将使用 flow 编排模式
// 而非默认的 PLAN→EXECUTE 循环。flow 步骤可通过 flow.FlowContextFrom(ctx)
// 获取 FlowContext，访问 Action 执行、Model 调用、Session 读写等横切能力。
func WithFlow(f *flow.Flow) RunOption {
	return func(c *runConfig) {
		c.flow = f
	}
}

// WithTracer 安装一个 OpenTelemetry tracer。当传给 New 时持久化为 Agent
// 默认；当传给 Run 时对该次调用做覆盖。tracer 非 nil 时 executeFlow 会在
// 入口创建 SpanFlowExecute span、在暂停点创建 SpanPause span；ResumeFlow
// 在入口创建 SpanResume span；flowContextImpl 也会持有该 tracer，让 step
// 可以通过 FlowContext.StartSpan 自建 SpanKindStep / SpanKindTool span。
// 传 nil 可显式禁用既有 tracer。
func WithTracer(t SpanStarter) RunOption {
	return func(c *runConfig) {
		c.tracer = t
	}
}

// WithOutputSchema sets the output schema for L4 post-validation.
// When set, executeLoop validates the LLM response against this schema
// with up to 2 retries. nil disables L4 validation (default).
func WithOutputSchema(s *model.OutputSchema) RunOption {
	return func(c *runConfig) {
		c.outputSchema = s
	}
}

// WithPIIMasker installs a PII masker on the Agent. When passed to New it
// is persisted as the Agent default; when passed to Run it overrides the
// default for that call. The masker is propagated to the session so that
// user input is redacted via MaskInput before it enters the conversation
// history, and the final response is redacted via MaskOutput before Run
// returns it. The masker's own configuration controls which sides are
// actually masked. Pass nil to disable PII masking.
//
// Any value satisfying the PIIMasker interface may be passed; the
// concrete implementation backed by github.com/inferglow/security/pii is
// provided by github.com/inferglow/security/agenthook.NewPIIMasker. This
// option is the high-level wrapper that wires the masker into both the
// session (input) and the return path (output).
func WithPIIMasker(m PIIMasker) RunOption {
	return func(c *runConfig) {
		c.piiMasker = m
	}
}

// WithContextManager installs a CacheBudgetUpdater that receives
// cached_tokens feedback from each LLM response and adjusts context
// management sweet-spot thresholds to maximize prefix cache hits.
// Pass nil to disable (default).
func WithContextManager(u CacheBudgetUpdater) RunOption {
	return func(c *runConfig) {
		c.cacheBudgetUpdater = u
	}
}

// WithCompactHook installs a callback invoked after each LLM turn with
// promptTokens. Used by ModeSummary to trigger session compaction when
// the prompt approaches the context window limit.
func WithCompactHook(hook func(promptTokens int)) RunOption {
	return func(c *runConfig) {
		c.compactHook = hook
	}
}

// WithAuditHook installs an audit hook that records decision audit
// entries. When non-nil, the engine is created with NewEngineWithAudit
// instead of the default NoOpHook. Pass nil to disable (default).
func WithAuditHook(hook audit.AuditHook) RunOption {
	return func(c *runConfig) {
		c.auditHook = hook
	}
}

// WithRollout 安装会话级 Rollout 记录器（R3）。nil 禁用（默认，零开销，
// 零行为变化）。对 ephemeral 会话请用空目录构造 recorder
// （session.NewRolloutRecorder("", sessionID)），该 recorder 为 no-op。
func WithRollout(r *session.RolloutRecorder) RunOption {
	return func(c *runConfig) {
		c.rollout = r
	}
}

// New creates an Agent from the given components. Options applied here
// (e.g. WithMaxRounds, WithSystemPrompt, WithStreamTimeout) are persisted
// on the Agent and used by subsequent Run calls unless overridden by a
// per-call option.
func New(sess *session.Session, actionExt *ActionExtension, modelReq model.ModelRequester, opts ...RunOption) *Agent {
	sessionExt := NewSessionExtension(sess)

	c := &runConfig{maxRounds: 10}
	for _, opt := range opts {
		opt(c)
	}

	// Use NewEngineWithAudit when an audit hook is provided, otherwise
	// use the default NewEngine (NoOpHook, zero overhead).
	var engine *Engine
	if c.auditHook != nil {
		engine = NewEngineWithAudit(sessionExt, actionExt, modelReq, c.auditHook)
	} else {
		engine = NewEngine(sessionExt, actionExt, modelReq)
	}

	// Default to DefaultFeatures when WithFeatures was not supplied so
	// that an explicit zero-value Features (all disabled) is respected.
	features := c.features
	if !c.featuresSet {
		features = DefaultFeatures()
	}

	return &Agent{
		session:       sessionExt,
		actionExt:     actionExt,
		engine:        engine,
		maxRounds:     c.maxRounds,
		systemPrompt:  c.systemPrompt,
		streamTimeout: c.streamTimeout,
		outputHook:    c.outputHook,
		piiMasker:     c.piiMasker,
		features:      features,
		tracer:        c.tracer,
		flow:          c.flow,
		outputSchema:  c.outputSchema,
		rateLimitHook: c.rateLimitHook,
		callbacks:          c.callbacks,
		middlewares:        c.middlewares,
		rollout:            c.rollout,
		cacheBudgetUpdater: c.cacheBudgetUpdater,
	}
}

// Run executes the full PLAN → EXECUTE loop and returns the final response.
// The Agent's persisted maxRounds, systemPrompt, and streamTimeout (set via
// the corresponding With* options on New) are used as defaults; explicit
// per-call options override them.
func (a *Agent) Run(ctx context.Context, userMessage string, opts ...RunOption) (string, error) {
	c := &runConfig{maxRounds: 10}
	// Apply persisted Agent-level defaults first so per-call opts can
	// still override them. A zero persisted maxRounds keeps the default
	// of 10; an empty persisted systemPrompt keeps the default empty
	// prompt; a zero persisted streamTimeout keeps the engine default.
	if a.maxRounds != 0 {
		c.maxRounds = a.maxRounds
	}
	c.systemPrompt = a.systemPrompt
	c.streamTimeout = a.streamTimeout
	c.outputHook = a.outputHook
	c.piiMasker = a.piiMasker
	c.features = a.features
	c.tracer = a.tracer
	c.outputSchema = a.outputSchema
	c.rateLimitHook = a.rateLimitHook
	c.callbacks = a.callbacks
	c.rollout = a.rollout
	c.middlewares = a.middlewares
	c.featuresSet = true
	for _, opt := range opts {
		opt(c)
	}

	// Validate the effective feature set before executing. This rejects
	// configurations that request a feature the binary was not compiled
	// to support (e.g. Sandbox without the with_sandbox build tag).
	if err := c.features.Validate(); err != nil {
		return "", err
	}

	// Propagate the configured stream timeout to the engine for this run.
	a.engine.streamTimeout = c.streamTimeout

	// Propagate the output schema to the engine so executeLoop can perform
	// L3 prompt injection and L4 post-validation. nil disables both.
	a.engine.outputSchema = c.outputSchema

	// A4: propagate the tracer to the engine so ResumeFlow (an Engine method)
	// can emit SpanResume spans without taking a separate config argument.
	// executeFlow reads the tracer from runConfig directly; this assignment
	// only affects Engine-level methods that don't receive a runConfig.
	a.engine.tracer = c.tracer

	// Propagate the rate limit hook to the engine so executeLoop can
	// enforce rate limits before each RequestModel call. nil disables.
	a.engine.rateLimitHook = c.rateLimitHook

	// Propagate callbacks to the engine for lifecycle observability.
	a.engine.callbacks = c.callbacks

	// Propagate the rollout recorder to the engine for R3 session-level
	// rollout recording. nil disables (zero overhead, no behavior change).
	a.engine.rollout = c.rollout

	// Propagate the cache budget hook so executeLoop feeds cached_tokens
	// back to the context manager for sweet-spot adjustment. nil disables.
	if c.cacheBudgetUpdater != nil {
		a.engine.cacheBudgetHook = func(cached int) {
			c.cacheBudgetUpdater.UpdateCacheBudget(cached)
		}
	} else {
		a.engine.cacheBudgetHook = nil
	}

	// Propagate the compact hook so executeLoop can trigger ModeSummary
	// compaction after each LLM turn. nil disables.
	a.engine.compactHook = c.compactHook

	// Propagate the input queue to the engine so executeLoop can drain it
	// at turn boundaries.
	a.engine.inputQueue = a.inputQueue

	// Propagate multimodal content blocks for the initial user message this
	// run (see executeLoop AddUserContentBlocks). Reset after each run.
	a.engine.initialContentBlocks = c.contentBlocks

	// Propagate the PII masker to the session so that AddUserMessage
	// (called inside executeLoop) redacts input via MaskInput. Only set
	// when a masker is configured and PIIMasking is enabled; this
	// preserves any masker the caller may have installed directly on the
	// session.
	if c.features.PIIMasking && c.piiMasker != nil {
		a.session.SetMessageMasker(c.piiMasker)
	}

	// Flow 编排模式：若配置了 flow，走 executeFlow 路径。
	effectiveFlow := a.flow
	if c.flow != nil {
		effectiveFlow = c.flow
	}

	// Build the core handler: this is the function that middlewares wrap.
	// When middlewares are configured, the core handler is wrapped by the
	// chain before invocation. When no middlewares are present, the core
	// handler is called directly (zero overhead).
	coreHandler := func(ctx context.Context, userMsg string) (string, error) {
		if effectiveFlow != nil {
			// A1: executeFlow 现在返回 (*flow.Execution, string, error)。
			// 非 daemon 调用方（Agent.Run）传 nil opts 和零值 RunMeta 以保持向后兼容。
			// 当 flow 暂停时返回 ("", nil) —— daemon 会通过 ResumeFlow 续跑。
			exec, resp, err := a.engine.executeFlow(ctx, effectiveFlow, userMsg, c.systemPrompt, c, nil, RunMeta{})
			if err != nil {
				return "", err
			}
			if exec != nil && exec.State.Status == flow.StatusPaused {
				return "", nil
			}
			return resp, nil
		}

		decision, err := a.engine.executeLoop(ctx, userMsg, c.maxRounds, c.systemPrompt)
		if err != nil {
			return "", err
		}

		if decision.NextAction == "response" {
			// Output-side security check: scan the LLM's final response for
			// prompt-injection content before returning it to the caller.
			// A non-nil error blocks the response (the message is not added
			// to the session either, so the injected content never persists).
			// The check runs on the unmasked response so the detector sees
			// the real text. Skipped when features.PromptInjection is false.
			if c.features.PromptInjection && c.outputHook != nil {
				if err := c.outputHook.CheckOutput(decision.FinalResponse); err != nil {
					return "", err
				}
			}
			response := decision.FinalResponse
			// PII output masking: redact sensitive data from the final
			// response before it is stored in the session and returned to
			// the caller. MaskOutput returns the text unchanged when the
			// masker's ApplyOn does not include MaskOnOutput. Skipped when
			// features.PIIMasking is false.
			if c.features.PIIMasking && c.piiMasker != nil {
				response = c.piiMasker.MaskOutput(response)
			}
			// G1-02: pass reasoning through to the session so the next
			// multi-turn request includes reasoning_content for DeepSeek/MiMo.
			a.session.AddAssistantMessageWithReasoning(response, decision.Reasoning)
			return response, nil
		}

		// LLM decided to execute but has no final response — attempt synthesis
		// to produce a summary from the conversation so far.
		if decision.FinalResponse == "" && decision.NextAction == "execute" {
			synthResp, synthErr := a.engine.synthesiseResponse(ctx, c.systemPrompt)
			if synthErr != nil {
				return "", fmt.Errorf("agent: synthesis call failed: %w", synthErr)
			}
			if synthResp != "" {
				a.session.AddAssistantMessage(synthResp)
				return synthResp, nil
			}
		}
		return "", ErrNoFinalResponse
	}

	// Apply middleware chain when middlewares are configured.
	// When no middlewares are present, the core handler is called directly
	// (zero overhead).
	if len(c.middlewares) > 0 {
		// Wrap coreHandler into a middleware.Handler.
		core := func(ctx context.Context, input *middleware.Input) (*middleware.Output, error) {
			msg := ""
			if len(input.Messages) > 0 {
				msg = input.Messages[len(input.Messages)-1].Content
			}
			resp, err := coreHandler(ctx, msg)
			if err != nil {
				return nil, err
			}
			return &middleware.Output{
				Messages: []middleware.Message{{Role: "assistant", Content: resp}},
			}, nil
		}
		handler := middleware.Chain(c.middlewares...)(core)
		out, err := handler(ctx, &middleware.Input{
			Messages: []middleware.Message{{Role: "user", Content: userMessage}},
		})
		if err != nil {
			return "", err
		}
		if len(out.Messages) > 0 {
			return out.Messages[len(out.Messages)-1].Content, nil
		}
		return "", nil
	}
	return coreHandler(ctx, userMessage)
}

// SetInputQueue installs an InputQueue on the Agent for queue-mode input.
// When set, SubmitInput can enqueue messages while the agent is busy.
func (a *Agent) SetInputQueue(q *InputQueue) {
	a.inputQueue = q
}

// InputQueue returns the queue installed by SetInputQueue, or nil.
func (a *Agent) InputQueue() *InputQueue {
	return a.inputQueue
}

// Callbacks returns the agent's persisted lifecycle callbacks.
// This allows callers to merge additional callbacks without replacing the originals.
func (a *Agent) Callbacks() *AgentCallbacks {
	return a.callbacks
}

// TurnPhase returns the current turn-loop phase as a human-readable string.
// Possible values: "idle", "planning", "active", "unknown".
// Returns "unknown" if the engine or turn-loop is not initialized.
func (a *Agent) TurnPhase() string {
	if a.engine == nil || a.engine.turnLoop == nil {
		return "unknown"
	}
	switch a.engine.turnLoop.Phase() {
	case TurnPhaseIdle:
		return "idle"
	case TurnPhasePlanning:
		return "planning"
	case TurnPhaseActive:
		return "active"
	default:
		return "unknown"
	}
}

// SubmitInput submits user input to the agent. If the agent is idle, it runs
// immediately via Run. If busy, the request is enqueued according to mode:
//   - PreemptQueue: enqueue without cancellation
//   - PreemptSafePoint: enqueue and issue a safe-point cancel
//   - PreemptForce: enqueue and issue an immediate cancel
//
// The returned channel receives the result once the agent processes the input.
// The caller blocks on the channel; for async usage, wrap in a goroutine.
func (a *Agent) SubmitInput(ctx context.Context, msg string, mode PreemptMode) (<-chan InputResponse, error) {
	ch := make(chan InputResponse, 1)

	// If the agent is idle, run directly in a goroutine.
	if a.engine.turnLoop != nil && a.engine.turnLoop.Phase() == TurnPhaseIdle {
		go func() {
			resp, err := a.Run(ctx, msg)
			ch <- InputResponse{Response: resp, Error: err}
		}()
		return ch, nil
	}

	// Agent is busy — enqueue.
	if a.inputQueue == nil {
		return nil, errors.New("agent: input queue not configured; call SetInputQueue first")
	}

	req := InputRequest{
		Message:    msg,
		Mode:       mode,
		ResponseCh: ch,
		Ctx:        ctx,
	}
	if err := a.inputQueue.Enqueue(req); err != nil {
		return nil, err
	}

	// For non-queue modes, issue the cancel now.
	if mode != PreemptQueue && a.engine.cancelManager != nil {
		a.engine.cancelManager.CancelWithMode(mode, "user input: "+mode.String())
	}

	return ch, nil
}
