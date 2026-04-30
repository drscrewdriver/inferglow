package agent

import (
	"context"
	"errors"

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
}

// RunOption configures Agent.Run behavior.
type RunOption func(*runConfig)

type runConfig struct {
	maxRounds    int
	systemPrompt string
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

// New creates an Agent from the given components.
func New(sess *session.Session, actionExt *ActionExtension, modelReq model.ModelRequester, opts ...RunOption) *Agent {
	sessionExt := NewSessionExtension(sess)
	engine := NewEngine(sessionExt, actionExt, modelReq)

	c := &runConfig{maxRounds: 10}
	for _, opt := range opts {
		opt(c)
	}

	return &Agent{
		session:   sessionExt,
		actionExt: actionExt,
		engine:    engine,
	}
}

// Run executes the full PLAN → EXECUTE loop and returns the final response.
func (a *Agent) Run(ctx context.Context, userMessage string, opts ...RunOption) (string, error) {
	c := &runConfig{maxRounds: 10}
	for _, opt := range opts {
		opt(c)
	}

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
