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

	"github.com/inferglow/orchestrator/middleware"
)

// AgentHandler is the function signature for a single step in the agent
// middleware chain. Each handler receives the context and user message,
// and returns the agent's response or an error.
//
// Deprecated: Use middleware.Handler from github.com/inferglow/orchestrator/middleware
// instead. This type is retained for backward compatibility and will be removed
// in v7.0. Use WithUnifiedMiddleware to install new-style middlewares.
type AgentHandler func(ctx context.Context, userMessage string) (string, error)

// Middleware is a function that wraps an AgentHandler to add cross-cutting
// behavior (logging, auth, rate limiting, etc.). Middlewares compose in a
// chain: the first middleware in the list is the outermost wrapper.
//
// Deprecated: Use middleware.Middleware from github.com/inferglow/orchestrator/middleware
// instead. This type is retained for backward compatibility and will be removed
// in v7.0. Use WithUnifiedMiddleware to install new-style middlewares.
type Middleware func(next AgentHandler) AgentHandler

// WithMiddleware installs one or more middlewares for this Run call.
// Middlewares wrap the core executeLoop/executeFlow path. When no
// middlewares are configured the core path is called directly (zero
// overhead).
//
// Deprecated: Use WithUnifiedMiddleware with middleware.Middleware instead.
func WithMiddleware(mw ...Middleware) RunOption {
	return func(c *runConfig) {
		c.middlewares = append(c.middlewares, mw...)
	}
}

// WithUnifiedMiddleware installs one or more unified middlewares for this
// Run call. Unified middlewares use the middleware.Handler signature which
// is shared across agent, team, and workflow packages.
//
// The unified middlewares are adapted to the legacy AgentHandler signature
// internally. When both legacy and unified middlewares are installed, the
// unified ones are applied first (outermost).
func WithUnifiedMiddleware(mw ...middleware.Middleware) RunOption {
	return func(c *runConfig) {
		c.unifiedMiddlewares = append(c.unifiedMiddlewares, mw...)
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

// adaptUnifiedToLegacy converts a unified middleware.Middleware to the legacy
// agent.Middleware signature. The adapter extracts the last user message from
// Input.Messages and passes it as the userMessage string to the legacy handler.
func adaptUnifiedToLegacy(mw middleware.Middleware) Middleware {
	return func(next AgentHandler) AgentHandler {
		return func(ctx context.Context, userMessage string) (string, error) {
			// Build a unified handler that calls the legacy next handler.
			unifiedNext := func(ctx context.Context, input *middleware.Input) (*middleware.Output, error) {
				msg := ""
				if len(input.Messages) > 0 {
					msg = input.Messages[len(input.Messages)-1].Content
				}
				result, err := next(ctx, msg)
				if err != nil {
					return nil, err
				}
				return &middleware.Output{
					Messages: []middleware.Message{{Role: "assistant", Content: result}},
				}, nil
			}
			// Apply the unified middleware to get a wrapped handler.
			wrapped := mw(unifiedNext)
			// Call the wrapped handler with the user message.
			out, err := wrapped(ctx, &middleware.Input{
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
	}
}
