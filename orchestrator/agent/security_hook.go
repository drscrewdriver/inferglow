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

import "errors"

// ErrOutputInjectionBlocked is returned by an OutputSecurityHook
// implementation when a detection in the LLM's final response resolves to
// ActionBlock. Run surfaces this error to the caller instead of returning
// the offending response. The concrete implementation that returns this
// error (OutputInjectionHook) now lives in
// github.com/inferglow/security/agenthook.
var ErrOutputInjectionBlocked = errors.New("prompt injection detected in output: response blocked")

// OutputSecurityHook scans the LLM's final response for prompt-injection
// content before it is returned to the caller. A non-nil error blocks the
// response; a nil error allows it. Implementations must be safe for
// concurrent use.
//
// The prompt-injection-backed implementation (OutputInjectionHook) has been
// moved to github.com/inferglow/security/agenthook to keep the orchestrator
// free of a direct security dependency. Callers construct it via
// agenthook.NewOutputInjectionHook and pass it to WithOutputSecurityHook.
type OutputSecurityHook interface {
	// CheckOutput inspects the model's final response text. A non-nil
	// error prevents Run from returning the response.
	CheckOutput(text string) error
}

// WithOutputSecurityHook is a RunOption that installs an output-side
// security hook. When passed to New it is persisted as the Agent default;
// when passed to Run it overrides the default for that call. Pass nil to
// disable an existing hook.
func WithOutputSecurityHook(hook OutputSecurityHook) RunOption {
	return func(c *runConfig) {
		c.outputHook = hook
	}
}
