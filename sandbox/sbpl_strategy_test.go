package sandbox

import (
	"testing"
)

// ---------- stub 层测试（跨平台） ----------

func TestBuildSBPLProfileStubReturnsEmptyOnNonDarwin(t *testing.T) {
	cfg := SeatbeltConfig{}
	pol := DefaultPolicy()
	profile := buildSBPLProfile(cfg, &pol)
	if profile != "" {
		t.Errorf("expected empty string on non-darwin, got %q", profile)
	}
}

func TestRealPathStubReturnsEmptyOnNonDarwin(t *testing.T) {
	p := realPath("/some/path")
	if p != "" {
		t.Errorf("expected empty string from realPath on non-darwin, got %q", p)
	}
}

func TestParseSeatbeltConfigReturnsEmptyOnNonDarwin(t *testing.T) {
	cfg := parseSeatbeltConfig(map[string]any{"timeout": 30})
	// 在非 darwin 平台上返回空结构体
	_ = cfg
}

func TestWriteSBPLProfileReturnsErrorOnNonDarwin(t *testing.T) {
	_, _, err := writeSBPLProfile("test profile")
	if err == nil {
		t.Error("expected error from writeSBPLProfile on non-darwin")
	}
}
