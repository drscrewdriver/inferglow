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

// Package resource provides lifecycle-managed execution resources for
// the inferglow orchestrator. It is the Go equivalent of Agently's
// ExecutionResource layer: a unified abstraction over stateful runtime
// environments such as bash shells, Python interpreters, SQLite handles,
// browser sessions, and MCP server processes.
//
// Resources are created by ResourceProvider implementations, tracked via
// ResourceHandle instances, and managed centrally by a ResourceManager
// that supports capability-based provider selection, handle reuse,
// health checking, and scope-based release.
package resource

import (
	"context"
	"time"
)

// Resource represents a single lifecycle-managed execution resource
// instance. Implementations wrap concrete runtimes (bash process, Python
// interpreter, SQLite connection, browser session, MCP server, etc.).
type Resource interface {
	// ID returns the unique identifier for this resource instance.
	ID() string

	// Type returns the resource type (e.g. "bash", "python", "sqlite",
	// "browser", "mcp").
	Type() string

	// Execute runs a command against this resource. The cmd parameter
	// is type-specific to the resource implementation. Returns a
	// ResourceResult with structured output.
	Execute(ctx context.Context, cmd any) (*ResourceResult, error)

	// HealthCheck verifies the resource is still operational. Returns
	// nil if healthy, an error describing the problem otherwise.
	HealthCheck(ctx context.Context) error

	// Close releases the resource and any underlying system resources.
	// After Close returns, Execute and HealthCheck must return errors.
	Close() error
}

// ResourceResult holds the structured output of a Resource.Execute call.
type ResourceResult struct {
	// OK indicates whether execution succeeded.
	OK bool `json:"ok"`

	// Status is a human-readable status string (e.g. "success", "error",
	// "timeout").
	Status string `json:"status"`

	// Output holds the primary execution output.
	Output any `json:"output,omitempty"`

	// Error holds error details when OK is false.
	Error string `json:"error,omitempty"`

	// Metadata carries implementation-specific key-value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`

	// Duration records how long the execution took.
	Duration time.Duration `json:"duration,omitempty"`
}
