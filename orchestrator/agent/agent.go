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
	"time"

	"github.com/inferglow/flow"
	"github.com/inferglow/model"
	"github.com/inferglow/observability/otel"
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
	tracer *otel.Tracer
	// flow 是可选的 flow 编排定义。nil 时使用 executeLoop（oneshot 模式）；
	// 非 nil 时使用 executeFlow（flow 编排模式）。
	// 通过 WithFlow RunOption 设置。
	flow *flow.Flow
	// outputSchema is the optional L4 output schema used for post-validation
	// of the LLM response. Set via WithOutputSchema on New; overridden by a
	// per-call WithOutputSchema on Run. nil disables L4 validation (default).
	outputSchema *model.OutputSchema
}

// RunOption configures Agent.Run behavior.
type RunOption func(*runConfig)

type runConfig struct {
	maxRounds     int
	systemPrompt  string
	streamTimeout time.Duration
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
	tracer *otel.Tracer
	// flow 可选的 flow 编排定义（per-call 覆盖）。
	flow *flow.Flow
	// outputSchema is the optional L4 output schema for post-validation
	// of the LLM response. Populated from the Agent default and optionally
	// overridden by a per-call WithOutputSchema. nil disables L4 validation.
	outputSchema *model.OutputSchema
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
func WithTracer(t *otel.Tracer) RunOption {
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

// New creates an Agent from the given components. Options applied here
// (e.g. WithMaxRounds, WithSystemPrompt, WithStreamTimeout) are persisted
// on the Agent and used by subsequent Run calls unless overridden by a
// per-call option.
func New(sess *session.Session, actionExt *ActionExtension, modelReq model.ModelRequester, opts ...RunOption) *Agent {
	sessionExt := NewSessionExtension(sess)
	engine := NewEngine(sessionExt, actionExt, modelReq)

	c := &runConfig{maxRounds: 10}
	for _, opt := range opts {
		opt(c)
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
	if effectiveFlow != nil {
		// A1: executeFlow 现在返回 (*flow.Execution, string, error)。
		// 非 daemon 调用方（Agent.Run）传 nil opts 和零值 RunMeta 以保持向后兼容。
		// 当 flow 暂停时返回 ("", nil) —— daemon 会通过 ResumeFlow 续跑。
		exec, resp, err := a.engine.executeFlow(ctx, effectiveFlow, userMessage, c.systemPrompt, c, nil, RunMeta{})
		if err != nil {
			return "", err
		}
		if exec != nil && exec.State.Status == flow.StatusPaused {
			return "", nil
		}
		return resp, nil
	}

	decision, err := a.engine.executeLoop(ctx, userMessage, c.maxRounds, c.systemPrompt)
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
		a.session.AddAssistantMessage(response)
		return response, nil
	}

	// LLM decided to execute but has no final response — this is an error
	return "", ErrNoFinalResponse
}
