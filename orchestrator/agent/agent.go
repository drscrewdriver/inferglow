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

	"github.com/inferglow/model"
	"github.com/inferglow/security/pii"
	"github.com/inferglow/session"
)

// ErrNoFinalResponse indicates the LLM decided to execute actions but provided no final response.
var ErrNoFinalResponse = errors.New("LLM decided to execute actions but provided no final_response")

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
	// returns.
	piiMasker *pii.Masker
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
	piiMasker *pii.Masker
	// rateLimitHook optionally checks rate limits before RequestModel.
	rateLimitHook RateLimitHook
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

// WithPIIMasker installs a PII masker on the Agent. When passed to New it
// is persisted as the Agent default; when passed to Run it overrides the
// default for that call. The masker is propagated to the session so that
// user input is redacted via MaskInput before it enters the conversation
// history, and the final response is redacted via MaskOutput before Run
// returns it. The masker's own ApplyOn config controls which sides are
// actually masked. Pass nil to disable PII masking.
//
// *pii.Masker satisfies the session.MessageMasker interface; this option
// is the high-level wrapper that wires it into both the session (input)
// and the return path (output).
func WithPIIMasker(m *pii.Masker) RunOption {
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

	return &Agent{
		session:       sessionExt,
		actionExt:     actionExt,
		engine:        engine,
		maxRounds:     c.maxRounds,
		systemPrompt:  c.systemPrompt,
		streamTimeout: c.streamTimeout,
		outputHook:    c.outputHook,
		piiMasker:     c.piiMasker,
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
	for _, opt := range opts {
		opt(c)
	}

	// Propagate the configured stream timeout to the engine for this run.
	a.engine.streamTimeout = c.streamTimeout

	// Propagate the PII masker to the session so that AddUserMessage
	// (called inside executeLoop) redacts input via MaskInput. Only set
	// when a masker is configured; this preserves any masker the caller
	// may have installed directly on the session.
	if c.piiMasker != nil {
		a.session.SetMessageMasker(c.piiMasker)
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
		// the real text.
		if c.outputHook != nil {
			if err := c.outputHook.CheckOutput(decision.FinalResponse); err != nil {
				return "", err
			}
		}
		response := decision.FinalResponse
		// PII output masking: redact sensitive data from the final
		// response before it is stored in the session and returned to
		// the caller. MaskOutput returns the text unchanged when the
		// masker's ApplyOn does not include MaskOnOutput.
		if c.piiMasker != nil {
			response = c.piiMasker.MaskOutput(response)
		}
		a.session.AddAssistantMessage(response)
		return response, nil
	}

	// LLM decided to execute but has no final response — this is an error
	return "", ErrNoFinalResponse
}
