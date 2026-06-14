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
)

// AgentHandler is the function signature for a single step in the agent
// middleware chain. Each handler receives the context and user message,
// and returns the agent's response or an error.
type AgentHandler func(ctx context.Context, userMessage string) (string, error)

// Middleware is a function that wraps an AgentHandler to add cross-cutting
// behavior (logging, auth, rate limiting, etc.). Middlewares compose in a
// chain: the first middleware in the list is the outermost wrapper.
type Middleware func(next AgentHandler) AgentHandler

// WithMiddleware installs one or more middlewares for this Run call.
// Middlewares wrap the core executeLoop/executeFlow path. When no
// middlewares are configured the core path is called directly (zero
// overhead).
func WithMiddleware(mw ...Middleware) RunOption {
	return func(c *runConfig) {
		c.middlewares = append(c.middlewares, mw...)
	}
}

// chainMiddleware composes a slice of middlewares into a single middleware.
// The returned middleware applies them in order: the first middleware in the
// slice is the outermost wrapper (executed first on entry, last on exit).
func chainMiddleware(mws []Middleware) Middleware {
	return func(final AgentHandler) AgentHandler {
		// Apply in reverse so the first middleware in the list wraps last.
		for i := len(mws) - 1; i >= 0; i-- {
			final = mws[i](final)
		}
		return final
	}
}
