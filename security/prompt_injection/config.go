package promptinjection

import "regexp"

// DetectionLevel controls how aggressively the detector fires and what
// Action the integration layer should take by default.
type DetectionLevel int

const (
	// LevelOff disables detection entirely. All text is allowed.
	LevelOff DetectionLevel = iota
	// LevelStrict treats any detection as a high-priority event; the
	// default Action for a detection is Block.
	LevelStrict
	// LevelRelaxed only blocks on High severity; Low/Medium severities
	// are flagged but allowed through.
	LevelRelaxed
)

// Action is the policy decision returned for a detection result.
type Action int

const (
	// ActionAllow lets the text through unchanged.
	ActionAllow Action = iota
	// ActionFlag lets the text through but records the detection so
	// upstream callers can log/alert/annotate.
	ActionFlag
	// ActionBlock rejects the text (the message is not appended / the
	// output is not returned to the caller).
	ActionBlock
)

// Config configures a Detector and the policy mapping from a
// DetectionResult to an Action. The zero value is NOT a safe default;
// use NewDefaultConfig instead.
type Config struct {
	// Level selects the detection strictness and the default Action
	// applied when OnDetect is nil.
	Level DetectionLevel
	// CustomPatterns are additional user-supplied regexes evaluated
	// alongside the built-in patterns. A match contributes to the
	// result with Medium severity.
	CustomPatterns []*regexp.Regexp
	// OnDetect, when non-nil, overrides the default Level → Action
	// mapping. It is invoked with the DetectionResult and must return
	// the Action to apply. Returning ActionAllow is always honored.
	OnDetect func(*DetectionResult) Action
}

// NewDefaultConfig returns the recommended baseline configuration:
// LevelStrict with no custom patterns and no OnDetect override, so any
// detection blocks the text.
func NewDefaultConfig() *Config {
	return &Config{
		Level:           LevelStrict,
		CustomPatterns:  nil,
		OnDetect:        nil,
	}
}

// NewRelaxedConfig returns a Relaxed configuration: only High severity
// blocks; Low/Medium severities are flagged but allowed.
func NewRelaxedConfig() *Config {
	return &Config{
		Level: LevelRelaxed,
	}
}

// NewOffConfig returns a configuration with detection disabled.
func NewOffConfig() *Config {
	return &Config{
		Level: LevelOff,
	}
}

// ResolveAction maps a DetectionResult to an Action according to the
// configured Level (or OnDetect override). A nil or non-detected result
// always yields ActionAllow.
func (c *Config) ResolveAction(result *DetectionResult) Action {
	if c == nil || result == nil || !result.Detected {
		return ActionAllow
	}
	if c.Level == LevelOff {
		return ActionAllow
	}
	if c.OnDetect != nil {
		return c.OnDetect(result)
	}
	switch c.Level {
	case LevelStrict:
		// Any detection blocks under Strict.
		return ActionBlock
	case LevelRelaxed:
		// Only High severity blocks; Low/Medium are flagged.
		if result.Severity >= SeverityHigh {
			return ActionBlock
		}
		return ActionFlag
	default:
		return ActionAllow
	}
}
