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
// LIABILITY, WHETHER IN AN ACTION OR CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package agent

import (
	"context"
	"time"

	"github.com/inferglow/model"
	"github.com/inferglow/session"
)

// ChatModelAgent is a ready-to-use agent that wires Session, ActionExtension,
// ModelRequester, and Engine into a single convenient entry point. Users
// create one with NewChatModelAgent and call Run for a one-line agent
// invocation with sensible defaults (maxRounds=10, streamTimeout=5min).
//
// ChatModelAgent is a thin convenience wrapper over the existing Agent and
// Engine; it does not re-implement the PLAN → EXECUTE loop. All heavy
// lifting (session management, tool dispatch, PII masking, stream timeout)
// is delegated to the underlying Agent.
type ChatModelAgent struct {
	session   *session.Session     // 会话管理
	actionExt *ActionExtension     // 工具扩展
	modelReq  model.ModelRequester // LLM 请求器
	engine    *Engine              // 执行引擎 (与内部 agent 共享同一引用)
	config    ChatModelAgentConfig // 配置

	// agent is the underlying Agent that owns the engine, session extension,
	// and persisted RunOption defaults. Run delegates to it so that all
	// existing logic (PII output masking, security hooks, stream timeout
	// propagation) is reused rather than re-implemented.
	agent *Agent
}

// ChatModelAgentConfig holds the persisted configuration for a
// ChatModelAgent. The zero value is not used directly; NewChatModelAgent
// applies defaults (MaxRounds=10, StreamTimeout=5min) before user options.
type ChatModelAgentConfig struct {
	MaxRounds     int           // 默认 10
	StreamTimeout time.Duration // 默认 5 分钟
	SystemPrompt  string        // 可选系统提示
	PIIMasker     PIIMasker     // 可选 PII 脱敏 (nil 禁用)
}

// ChatModelAgentOption configures a ChatModelAgent at construction time.
// The With* option functions in agent.go (WithMaxRounds, WithSystemPrompt,
// WithStreamTimeout, WithPIIMasker) operate on runConfig and return RunOption;
// they cannot be reused here because the return type differs. The WithAgent*
// functions below are the ChatModelAgent-level equivalents.
type ChatModelAgentOption func(*ChatModelAgentConfig)

// WithAgentMaxRounds sets the maximum number of PLAN → EXECUTE loop
// iterations. Defaults to 10 when unset.
func WithAgentMaxRounds(n int) ChatModelAgentOption {
	return func(c *ChatModelAgentConfig) { c.MaxRounds = n }
}

// WithAgentSystemPrompt sets the system prompt for the LLM.
func WithAgentSystemPrompt(prompt string) ChatModelAgentOption {
	return func(c *ChatModelAgentConfig) { c.SystemPrompt = prompt }
}

// WithAgentStreamTimeout sets the per-stream timeout (how long executeLoop
// waits for the next chunk from the model stream). Defaults to 5 minutes.
func WithAgentStreamTimeout(d time.Duration) ChatModelAgentOption {
	return func(c *ChatModelAgentConfig) { c.StreamTimeout = d }
}

// WithAgentPIIMasker installs a PII masker. When non-nil, the masker is
// propagated to the underlying Agent (and thus to the session for input
// masking and to the final response for output masking). Pass nil to
// disable PII masking.
func WithAgentPIIMasker(m PIIMasker) ChatModelAgentOption {
	return func(c *ChatModelAgentConfig) { c.PIIMasker = m }
}

// NewChatModelAgent creates a ready-to-use ChatModelAgent with sensible
// defaults (maxRounds=10, streamTimeout=5min). The supplied opts override
// the defaults and are persisted on the returned agent; subsequent Run
// calls use them without needing per-call options.
//
// This is the "one-line" constructor: callers pass the three required
// components (session, actionExt, modelReq) and immediately get an agent
// whose Run method performs the full PLAN → EXECUTE loop.
func NewChatModelAgent(sess *session.Session, actionExt *ActionExtension, modelReq model.ModelRequester, opts ...ChatModelAgentOption) *ChatModelAgent {
	c := ChatModelAgentConfig{
		MaxRounds:     10,
		StreamTimeout: 5 * time.Minute,
	}
	for _, opt := range opts {
		opt(&c)
	}

	// Translate the ChatModelAgentConfig into the RunOption pipeline that
	// the underlying Agent understands. This reuses the existing
	// WithMaxRounds / WithSystemPrompt / WithStreamTimeout / WithPIIMasker
	// RunOption functions from agent.go rather than duplicating their logic.
	var runOpts []RunOption
	if c.MaxRounds > 0 {
		runOpts = append(runOpts, WithMaxRounds(c.MaxRounds))
	}
	if c.SystemPrompt != "" {
		runOpts = append(runOpts, WithSystemPrompt(c.SystemPrompt))
	}
	if c.StreamTimeout > 0 {
		runOpts = append(runOpts, WithStreamTimeout(c.StreamTimeout))
	}
	if c.PIIMasker != nil {
		runOpts = append(runOpts, WithPIIMasker(c.PIIMasker))
	}

	a := New(sess, actionExt, modelReq, runOpts...)
	return &ChatModelAgent{
		session:   sess,
		actionExt: actionExt,
		modelReq:  modelReq,
		engine:    a.engine,
		config:    c,
		agent:     a,
	}
}

// Run executes the full PLAN → EXECUTE loop and returns the final response
// string. This is the convenience "one-line" entry point: the caller does
// not need to create an Engine, set RunOptions, or manage the session —
// everything is pre-wired from the ChatModelAgentConfig.
func (a *ChatModelAgent) Run(ctx context.Context, userMessage string) (string, error) {
	return a.agent.Run(ctx, userMessage)
}

// RunStream returns a StreamReader that emits StreamChunks as the LLM
// generates output. The stream delivers raw LLM output in real-time and
// does not wait for action execution. The returned reader must be closed
// when the caller is done (typically via defer).
//
// The configured SystemPrompt is applied to the stream request. When a
// PIIMasker is configured it is propagated to the session so that user
// input is redacted via MaskInput before entering the conversation history.
func (a *ChatModelAgent) RunStream(ctx context.Context, userMessage string) (*model.StreamReader[*model.StreamChunk], error) {
	var opts []RunOption
	if a.config.SystemPrompt != "" {
		opts = append(opts, WithSystemPrompt(a.config.SystemPrompt))
	}
	// Propagate the PII masker to the session so that AddUserMessage
	// (called inside StreamRun) redacts input via MaskInput.
	if a.config.PIIMasker != nil {
		a.agent.session.SetMessageMasker(a.config.PIIMasker)
	}

	stream, err := a.agent.StreamRun(ctx, userMessage, opts...)
	if err != nil {
		return nil, err
	}
	return model.StreamReaderFromChannel[*model.StreamChunk](stream), nil
}

// RunWithOpts executes the agent with per-call RunOption values that
// override the agent's configured defaults for this call only. It is an
// escape hatch for callers who need per-call control (e.g. a tighter
// maxRounds for a specific request) while still using the convenience
// constructor.
func (a *ChatModelAgent) RunWithOpts(ctx context.Context, userMessage string, opts ...RunOption) (string, error) {
	return a.agent.Run(ctx, userMessage, opts...)
}

// Config returns the effective configuration of the agent (with defaults
// applied). The returned value is a copy; mutating it does not affect the
// agent.
func (a *ChatModelAgent) Config() ChatModelAgentConfig {
	return a.config
}
