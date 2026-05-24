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

//go:build !with_sandbox

package action

import (
	"context"
)

// SandboxExecutorConfig is a stub used when the action package is built
// without the with_sandbox tag. The sandbox-typed fields available in the
// real configuration are unavailable; build with -tags with_sandbox to use
// the full SandboxExecutorConfig.
type SandboxExecutorConfig struct{}

// SandboxExecutor is a stub that returns an error from Execute when the
// action package is built without the with_sandbox tag. Build with
// -tags with_sandbox to enable real sandbox-backed execution.
type SandboxExecutor struct{}

// NewSandboxExecutor constructs a stub SandboxExecutor when built without
// the with_sandbox tag. The returned executor always reports an error from
// Execute indicating the build tag is required.
func NewSandboxExecutor(cfg SandboxExecutorConfig) *SandboxExecutor {
	return &SandboxExecutor{}
}

// Execute implements ActionExecutor. Without the with_sandbox build tag it
// always returns an error-shaped ActionResult instructing the caller to
// rebuild with -tags with_sandbox.
func (e *SandboxExecutor) Execute(ctx context.Context, input map[string]any) (*ActionResult, error) {
	return &ActionResult{
		OK:     false,
		Status: "error",
		Error:  "sandbox executor requires building with -tags with_sandbox",
	}, nil
}

// 编译期断言
var _ ActionExecutor = (*SandboxExecutor)(nil)
