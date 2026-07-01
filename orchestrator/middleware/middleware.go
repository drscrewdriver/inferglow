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

// Package middleware provides a unified Handler/Middleware type signature for
// the orchestration layer. It is inspired by net/http.Handler — zero learning
// cost for Go developers.
//
// The types defined here are intentionally lightweight and dependency-free:
// they do not import session, model, or any other InferGlow module. This
// allows agent, team, and workflow packages to share the same middleware
// chain without pulling in heavy dependencies.
package middleware

import (
	"context"
)

// Handler is the unified function signature for a single processing step in
// the orchestration layer. It receives context and a structured Input, and
// returns a structured Output or an error.
//
// Analogous to net/http.HandlerFunc — the fundamental building block for
// composable request/response pipelines.
type Handler func(ctx context.Context, input *Input) (*Output, error)

// Middleware wraps a Handler to add cross-cutting behavior (logging, tracing,
// rate limiting, recovery, etc.). Middlewares compose in a chain: the first
// middleware in the list is the outermost wrapper.
type Middleware func(next Handler) Handler

// Message is a lightweight message carrier. It avoids importing session or
// provider packages to keep this module dependency-free.
type Message struct {
	Role    string
	Content string
}

// Input is the structured input passed to a Handler.
type Input struct {
	// Messages is the conversation history (or request messages).
	Messages []Message
	// SessionID identifies the session for correlation.
	SessionID string
	// Metadata carries arbitrary key-value pairs for extensibility.
	Metadata map[string]any
}

// Output is the structured output returned by a Handler.
type Output struct {
	// Messages is the response messages produced by the handler.
	Messages []Message
	// Metadata carries arbitrary key-value pairs (e.g. usage info, cost).
	Metadata map[string]any
}

// Chain composes a slice of middlewares into a single middleware.
// The returned middleware applies them in order: the first middleware in the
// list is the outermost wrapper (executed first on entry, last on exit).
//
// When no middlewares are provided, Chain returns a pass-through middleware
// that simply calls the next handler directly (zero overhead).
func Chain(mws ...Middleware) Middleware {
	return func(final Handler) Handler {
		// Apply in reverse so the first middleware in the list wraps last.
		for i := len(mws) - 1; i >= 0; i-- {
			final = mws[i](final)
		}
		return final
	}
}
