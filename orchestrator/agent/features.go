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

// Features toggles optional orchestrator capabilities. The zero-value
// Features disables every feature; use DefaultFeatures to obtain the
// recommended baseline (core safety on, advanced capabilities off).
//
// Features are validated by Validate. Run rejects configurations that
// request a feature the binary was not compiled to support (e.g. Sandbox
// without the with_sandbox build tag) before the execute loop starts.
type Features struct {
	// PIIMasking enables PII redaction on user input (via the session)
	// and on the final response (via MaskOutput) when a PIIMasker is
	// configured. When false, an installed masker is ignored.
	PIIMasking bool
	// PromptInjection enables output-side prompt-injection scanning via
	// an OutputSecurityHook. When false, an installed hook is ignored.
	PromptInjection bool
	// Sandbox enables sandboxed action execution. Requires building the
	// action module with the with_sandbox build tag; Validate rejects
	// Sandbox=true otherwise.
	Sandbox bool
	// OpenTelemetry enables OpenTelemetry tracing/metrics propagation.
	// Reserved for future use; currently informational.
	OpenTelemetry bool
	// AuditChain enables tamper-evident audit chaining of actions.
	// Reserved for future use; currently informational.
	AuditChain bool
}

// DefaultFeatures returns the recommended feature baseline: core safety
// features (PII masking and prompt-injection detection) enabled, advanced
// capabilities (sandbox, OpenTelemetry, audit chain) disabled.
func DefaultFeatures() Features {
	return Features{
		PIIMasking:      true,
		PromptInjection: true,
		Sandbox:         false,
		OpenTelemetry:   false,
		AuditChain:      false,
	}
}

// Validate reports whether the requested feature set is satisfiable by
// the current binary. It currently verifies that Sandbox is only
// requested when the binary was built with the with_sandbox build tag.
func (f Features) Validate() error {
	if f.Sandbox && !hasSandboxBuild() {
		return errors.New("feature Sandbox requires building with -tags with_sandbox")
	}
	return nil
}

// WithFeatures is a RunOption that installs a Features configuration.
// When passed to New it is persisted as the Agent default (used by
// subsequent Run calls); when passed to Run it overrides the default for
// that call. When WithFeatures is not supplied to New the Agent uses
// DefaultFeatures. The effective features are validated at the start of
// Run; an invalid configuration causes Run to return an error before the
// execute loop runs.
func WithFeatures(features Features) RunOption {
	return func(c *runConfig) {
		c.features = features
		c.featuresSet = true
	}
}
