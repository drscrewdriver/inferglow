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

import "context"

// AgentCallbacks provides lifecycle hooks for observing Agent execution.
// All fields are optional; nil fields are silently skipped (zero overhead).
// Use WithCallbacks to install callbacks on an Agent.Run call.
type AgentCallbacks struct {
	// OnRunStart is called at the beginning of Agent.Run, before executeLoop.
	OnRunStart func(ctx context.Context, userMessage string)
	// OnRunEnd is called when Agent.Run completes (success or error).
	OnRunEnd func(ctx context.Context, response string, err error)
	// OnLLMCallStart is called before each LLM invocation in executeLoop.
	// round is the current iteration number (0-based).
	OnLLMCallStart func(ctx context.Context, round int)
	// OnLLMCallEnd is called after each LLM invocation completes.
	// tokens is the approximate token count of the response.
	OnLLMCallEnd func(ctx context.Context, round int, tokens int)
	// OnToolCallStart is called before a tool/action is executed.
	OnToolCallStart func(ctx context.Context, toolName string)
	// OnToolCallEnd is called after a tool/action completes.
	OnToolCallEnd func(ctx context.Context, toolName string, err error)
	// OnToken is called for each text delta chunk from the LLM stream.
	// Enables real-time token-by-token display in CLI/TUI frontends.
	OnToken func(ctx context.Context, delta string)
	// OnReasoning is called for each reasoning delta chunk (DeepSeek/MiMo).
	// Enables real-time reasoning display in CLI/TUI frontends.
	OnReasoning func(ctx context.Context, delta string)
	// OnApprovalRequired is called when a tool is blocked by approval policy.
	OnApprovalRequired func(ctx context.Context, toolName, recordID string)
	// OnCompression is called when context compression occurs.
	OnCompression func(ctx context.Context, stepsCompressed int)
}

// WithCallbacks installs lifecycle callbacks for this Run call.
// Pass nil to explicitly disable callbacks. Each callback field is
// independently optional; nil fields are skipped.
func WithCallbacks(cb *AgentCallbacks) RunOption {
	return func(c *runConfig) {
		c.callbacks = cb
	}
}

// fireOnRunStart invokes OnRunStart if non-nil. Safe to call with nil callbacks.
func fireOnRunStart(cb *AgentCallbacks, ctx context.Context, userMessage string) {
	if cb != nil && cb.OnRunStart != nil {
		cb.OnRunStart(ctx, userMessage)
	}
}

// fireOnRunEnd invokes OnRunEnd if non-nil. Safe to call with nil callbacks.
func fireOnRunEnd(cb *AgentCallbacks, ctx context.Context, response string, err error) {
	if cb != nil && cb.OnRunEnd != nil {
		cb.OnRunEnd(ctx, response, err)
	}
}

// fireOnLLMCallStart invokes OnLLMCallStart if non-nil.
func fireOnLLMCallStart(cb *AgentCallbacks, ctx context.Context, round int) {
	if cb != nil && cb.OnLLMCallStart != nil {
		cb.OnLLMCallStart(ctx, round)
	}
}

// fireOnLLMCallEnd invokes OnLLMCallEnd if non-nil.
func fireOnLLMCallEnd(cb *AgentCallbacks, ctx context.Context, round int, tokens int) {
	if cb != nil && cb.OnLLMCallEnd != nil {
		cb.OnLLMCallEnd(ctx, round, tokens)
	}
}

// fireOnToolCallStart invokes OnToolCallStart if non-nil.
func fireOnToolCallStart(cb *AgentCallbacks, ctx context.Context, toolName string) {
	if cb != nil && cb.OnToolCallStart != nil {
		cb.OnToolCallStart(ctx, toolName)
	}
}

// fireOnToolCallEnd invokes OnToolCallEnd if non-nil.
func fireOnToolCallEnd(cb *AgentCallbacks, ctx context.Context, toolName string, err error) {
	if cb != nil && cb.OnToolCallEnd != nil {
		cb.OnToolCallEnd(ctx, toolName, err)
	}
}

// fireOnToken invokes OnToken if non-nil. Called for each text delta chunk.
func fireOnToken(cb *AgentCallbacks, ctx context.Context, delta string) {
	if cb != nil && cb.OnToken != nil {
		cb.OnToken(ctx, delta)
	}
}

// fireOnReasoning invokes OnReasoning if non-nil. Called for each reasoning delta.
func fireOnReasoning(cb *AgentCallbacks, ctx context.Context, delta string) {
	if cb != nil && cb.OnReasoning != nil {
		cb.OnReasoning(ctx, delta)
	}
}

// fireOnApprovalRequired invokes OnApprovalRequired if non-nil.
func fireOnApprovalRequired(cb *AgentCallbacks, ctx context.Context, toolName, recordID string) {
	if cb != nil && cb.OnApprovalRequired != nil {
		cb.OnApprovalRequired(ctx, toolName, recordID)
	}
}

// fireOnCompression invokes OnCompression if non-nil.
func fireOnCompression(cb *AgentCallbacks, ctx context.Context, stepsCompressed int) {
	if cb != nil && cb.OnCompression != nil {
		cb.OnCompression(ctx, stepsCompressed)
	}
}
