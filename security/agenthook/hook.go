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

package agenthook

import (
	"sync"

	"github.com/inferglow/orchestrator/agent"
	promptinjection "github.com/inferglow/security/prompt_injection"
)

// Compile-time guard: *OutputInjectionHook must satisfy
// agent.OutputSecurityHook so it can be passed to
// agent.WithOutputSecurityHook.
var _ agent.OutputSecurityHook = (*OutputInjectionHook)(nil)

// OutputFlagRecord captures a detection in the model output that was
// flagged (rather than blocked) so callers can audit it.
type OutputFlagRecord struct {
	Text   string
	Result *promptinjection.DetectionResult
}

// OutputInjectionHook is the prompt-injection-backed
// agent.OutputSecurityHook. It runs the L1 detector over the model's final
// response and maps the result to an Action via the configured Config:
//
//   - ActionBlock: CheckOutput returns agent.ErrOutputInjectionBlocked.
//   - ActionFlag:  the detection is recorded in Flags() and CheckOutput
//     returns nil (the response is allowed through).
//   - ActionAllow: the response is allowed through unchanged.
//
// OutputInjectionHook is safe for concurrent use.
type OutputInjectionHook struct {
	detector *promptinjection.Detector
	config   *promptinjection.Config

	// OnFlag, when non-nil, is invoked synchronously after a detection
	// is flagged. It must not block on agent internals. May be nil.
	OnFlag func(text string, result *promptinjection.DetectionResult)

	mu    sync.Mutex
	flags []OutputFlagRecord
}

// NewOutputInjectionHook builds an OutputInjectionHook from a
// prompt-injection Config. A nil cfg is replaced with the default
// Strict config.
func NewOutputInjectionHook(cfg *promptinjection.Config) *OutputInjectionHook {
	if cfg == nil {
		cfg = promptinjection.NewDefaultConfig()
	}
	return &OutputInjectionHook{
		detector: promptinjection.NewDetectorWithConfig(cfg),
		config:   cfg,
	}
}

// CheckOutput implements agent.OutputSecurityHook.
func (h *OutputInjectionHook) CheckOutput(text string) error {
	if h == nil || h.detector == nil {
		return nil
	}
	result := h.detector.Detect(text)
	action := h.config.ResolveAction(result)
	switch action {
	case promptinjection.ActionBlock:
		return agent.ErrOutputInjectionBlocked
	case promptinjection.ActionFlag:
		h.recordFlag(text, result)
		if h.OnFlag != nil {
			h.OnFlag(text, result)
		}
		return nil
	default:
		return nil
	}
}

// Flags returns a copy of all flagged output detections recorded so far.
func (h *OutputInjectionHook) Flags() []OutputFlagRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]OutputFlagRecord, len(h.flags))
	copy(out, h.flags)
	return out
}

func (h *OutputInjectionHook) recordFlag(text string, result *promptinjection.DetectionResult) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.flags = append(h.flags, OutputFlagRecord{Text: text, Result: result})
}
