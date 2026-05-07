package agent

import (
	"context"
	"errors"
	"time"

	"github.com/inferglow/model"
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
}

// RunOption configures Agent.Run behavior.
type RunOption func(*runConfig)

type runConfig struct {
	maxRounds     int
	systemPrompt  string
	streamTimeout time.Duration
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
	for _, opt := range opts {
		opt(c)
	}

	// Propagate the configured stream timeout to the engine for this run.
	a.engine.streamTimeout = c.streamTimeout

	decision, err := a.engine.executeLoop(ctx, userMessage, c.maxRounds, c.systemPrompt)
	if err != nil {
		return "", err
	}

	if decision.NextAction == "response" {
		a.session.AddAssistantMessage(decision.FinalResponse)
		return decision.FinalResponse, nil
	}

	// LLM decided to execute but has no final response — this is an error
	return "", ErrNoFinalResponse
}
