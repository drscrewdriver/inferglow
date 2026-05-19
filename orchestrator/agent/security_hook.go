package agent

import (
	"errors"
	"sync"

	promptinjection "github.com/inferglow/security/prompt_injection"
)

// ErrOutputInjectionBlocked is returned by OutputInjectionHook.CheckOutput
// when the configured detector resolves a detection in the LLM's final
// response to ActionBlock. Run surfaces this error to the caller instead
// of returning the offending response.
var ErrOutputInjectionBlocked = errors.New("prompt injection detected in output: response blocked")

// OutputSecurityHook scans the LLM's final response for prompt-injection
// content before it is returned to the caller. A non-nil error blocks the
// response; a nil error allows it. Implementations must be safe for
// concurrent use.
type OutputSecurityHook interface {
	// CheckOutput inspects the model's final response text. A non-nil
	// error prevents Run from returning the response.
	CheckOutput(text string) error
}

// OutputFlagRecord captures a detection in the model output that was
// flagged (rather than blocked) so callers can audit it.
type OutputFlagRecord struct {
	Text   string
	Result *promptinjection.DetectionResult
}

// OutputInjectionHook is the prompt-injection-backed
// OutputSecurityHook. It runs the L1 detector over the model's final
// response and maps the result to an Action via the configured Config:
//
//   - ActionBlock: CheckOutput returns ErrOutputInjectionBlocked.
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

// CheckOutput implements OutputSecurityHook.
func (h *OutputInjectionHook) CheckOutput(text string) error {
	if h == nil || h.detector == nil {
		return nil
	}
	result := h.detector.Detect(text)
	action := h.config.ResolveAction(result)
	switch action {
	case promptinjection.ActionBlock:
		return ErrOutputInjectionBlocked
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

// WithOutputSecurityHook is a RunOption that installs an output-side
// security hook. When passed to New it is persisted as the Agent default;
// when passed to Run it overrides the default for that call. Pass nil to
// disable an existing hook.
func WithOutputSecurityHook(hook OutputSecurityHook) RunOption {
	return func(c *runConfig) {
		c.outputHook = hook
	}
}
