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

package resource

import (
	"context"
	"fmt"
)

// NoopResource is a minimal Resource implementation used for testing
// and as a placeholder when no real backend is available.
type NoopResource struct {
	id        string
	resType   string
	closed    bool
	execCount int
}

// NewNoopResource creates a NoopResource with the given type.
func NewNoopResource(resType string) *NoopResource {
	return &NoopResource{
		id:      generateID(),
		resType: resType,
	}
}

func (r *NoopResource) ID() string   { return r.id }
func (r *NoopResource) Type() string { return r.resType }

func (r *NoopResource) Execute(_ context.Context, cmd any) (*ResourceResult, error) {
	if r.closed {
		return nil, ErrResourceClosed
	}
	r.execCount++
	return &ResourceResult{
		OK:     true,
		Status: "success",
		Output: fmt.Sprintf("noop executed: %v", cmd),
	}, nil
}

func (r *NoopResource) HealthCheck(_ context.Context) error {
	if r.closed {
		return ErrResourceClosed
	}
	return nil
}

func (r *NoopResource) Close() error {
	r.closed = true
	return nil
}

// ExecCount returns how many times Execute has been called.
func (r *NoopResource) ExecCount() int { return r.execCount }

// NoopProvider is a ResourceProvider that creates NoopResource instances.
type NoopProvider struct {
	resType      string
	capabilities []string
}

// NewNoopProvider creates a NoopProvider for the given resource type.
func NewNoopProvider(resType string, capabilities ...string) *NoopProvider {
	return &NoopProvider{
		resType:      resType,
		capabilities: capabilities,
	}
}

func (p *NoopProvider) Type() string          { return p.resType }
func (p *NoopProvider) Capabilities() []string { return p.capabilities }

func (p *NoopProvider) Create(_ context.Context, _ ResourceConfig) (Resource, error) {
	return NewNoopResource(p.resType), nil
}

func (p *NoopProvider) Probe(_ context.Context) error { return nil }
