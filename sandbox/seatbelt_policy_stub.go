//go:build !darwin

package sandbox

// SeatbeltConfig is a stub on non-darwin platforms.
type SeatbeltConfig struct{}

// buildSBPLProfile returns an empty string on non-darwin platforms.
func buildSBPLProfile(cfg SeatbeltConfig, policy *ExecutionPolicy) string {
	return ""
}

// realPath returns an empty string on non-darwin platforms.
func realPath(path string) string {
	return ""
}

// parseSeatbeltConfig returns an empty config on non-darwin platforms.
func parseSeatbeltConfig(m map[string]any) SeatbeltConfig {
	return SeatbeltConfig{}
}

// writeSBPLProfile returns an error on non-darwin platforms.
func writeSBPLProfile(profile string) (string, func(), error) {
	return "", nil, ErrProviderUnavailable
}
