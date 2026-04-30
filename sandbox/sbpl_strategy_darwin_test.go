//go:build darwin

package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- darwin 实现测试 ----------

func TestSeatbeltConfigDefaults(t *testing.T) {
	cfg := SeatbeltConfig{}
	if cfg.Timeout != 0 {
		t.Errorf("expected default Timeout 0, got %d", cfg.Timeout)
	}
	if cfg.NetworkAllowOutbound {
		t.Error("expected default NetworkAllowOutbound false")
	}
	if cfg.PythonBinary != "" {
		t.Errorf("expected default PythonBinary empty, got %q", cfg.PythonBinary)
	}
}

func TestBuildSBPLProfileContainsDefaultDeny(t *testing.T) {
	cfg := SeatbeltConfig{}
	pol := DefaultPolicy()
	profile := buildSBPLProfile(cfg, &pol)
	if !strings.Contains(profile, "(deny default)") {
		t.Error("expected SBPL profile to contain '(deny default)'")
	}
}

func TestBuildSBPLProfileContainsBasicCapabilities(t *testing.T) {
	cfg := SeatbeltConfig{}
	pol := DefaultPolicy()
	profile := buildSBPLProfile(cfg, &pol)

	required := []string{
		"(allow process-exec",
		"(allow fork",
		"(allow signal",
		"(allow sysctl",
		"(allow mach-lookup",
	}
	for _, want := range required {
		if !strings.Contains(profile, want) {
			t.Errorf("expected SBPL profile to contain %q", want)
		}
	}
}

func TestBuildSBPLProfileIncludesWritablePaths(t *testing.T) {
	cfg := SeatbeltConfig{
		WritablePaths: []string{"/tmp/test-sandbox"},
	}
	pol := DefaultPolicy()
	profile := buildSBPLProfile(cfg, &pol)

	if !strings.Contains(profile, "(allow file-write*") {
		t.Error("expected SBPL profile to contain '(allow file-write*'")
	}
	if !strings.Contains(profile, "/tmp/test-sandbox") {
		t.Error("expected SBPL profile to include the writable path")
	}
}

func TestBuildSBPLProfileIncludesProtectedPathsDeny(t *testing.T) {
	cfg := SeatbeltConfig{
		ProtectedPaths: []string{"/System"},
	}
	pol := DefaultPolicy()
	profile := buildSBPLProfile(cfg, &pol)

	if !strings.Contains(profile, "(deny file-write*") {
		t.Error("expected SBPL profile to contain '(deny file-write*'")
	}
	if !strings.Contains(profile, "/System") {
		t.Error("expected SBPL profile to include the protected path")
	}
}

func TestBuildSBPLProfileIncludesDenyReadPaths(t *testing.T) {
	cfg := SeatbeltConfig{
		DenyReadPaths: []string{"/etc/shadow"},
	}
	pol := DefaultPolicy()
	profile := buildSBPLProfile(cfg, &pol)

	if !strings.Contains(profile, "/etc/shadow") {
		t.Error("expected SBPL profile to include deny-read path")
	}
}

func TestBuildSBPLProfileNetworkPolicyAllowOutbound(t *testing.T) {
	cfg := SeatbeltConfig{
		NetworkAllowOutbound: true,
	}
	pol := DefaultPolicy()
	profile := buildSBPLProfile(cfg, &pol)

	if !strings.Contains(profile, "(allow network-outbound") {
		t.Error("expected SBPL profile to allow outbound network when configured")
	}
}

func TestBuildSBPLProfileNetworkPolicyDenyOutbound(t *testing.T) {
	cfg := SeatbeltConfig{
		NetworkAllowOutbound: false,
	}
	pol := DefaultPolicy()
	profile := buildSBPLProfile(cfg, &pol)

	// 当 NetworkAllowOutbound=false 时，应该包含 deny 规则
	if !strings.Contains(profile, "(deny network-outbound") {
		t.Error("expected SBPL profile to deny outbound network when not allowed")
	}
}

func TestBuildSBPLProfileIncludesExtraRules(t *testing.T) {
	cfg := SeatbeltConfig{
		ExtraSBPLRules: "\n(allow posix-mutex-create)\n",
	}
	pol := DefaultPolicy()
	profile := buildSBPLProfile(cfg, &pol)

	if !strings.Contains(profile, "(allow posix-mutex-create)") {
		t.Error("expected SBPL profile to include extra rules")
	}
}

func TestBuildSBPLProfileMergesExecutionPolicyPaths(t *testing.T) {
	cfg := SeatbeltConfig{
		WritablePaths: []string{"/custom/writable"},
	}
	pol := ExecutionPolicy{
		FilesystemAccess: FilesystemPolicy{
			AllowedPaths: []string{"/policy/allowed"},
			DeniedPaths:  []string{"/policy/denied"},
		},
	}
	profile := buildSBPLProfile(cfg, &pol)

	// 应该包含 config 和 policy 中的路径
	if !strings.Contains(profile, "/custom/writable") {
		t.Error("expected profile to include config writable path")
	}
	if !strings.Contains(profile, "/policy/allowed") {
		t.Error("expected profile to include policy AllowedPaths")
	}
	if !strings.Contains(profile, "/policy/denied") {
		t.Error("expected profile to include policy DeniedPaths")
	}
}

func TestBuildSBPLProfileIncludesTempDirectoryWritable(t *testing.T) {
	cfg := SeatbeltConfig{}
	pol := DefaultPolicy()
	profile := buildSBPLProfile(cfg, &pol)

	// 临时目录应该始终可写
	tmp := os.TempDir()
	if !strings.Contains(profile, tmp) {
		t.Error("expected SBPL profile to make temp directory writable")
	}
}

func TestBuildSBPLProfileIncludesDeviceAllow(t *testing.T) {
	cfg := SeatbeltConfig{}
	pol := DefaultPolicy()
	profile := buildSBPLProfile(cfg, &pol)

	if !strings.Contains(profile, "/dev/") {
		t.Error("expected SBPL profile to allow device access")
	}
}

func TestBuildSBPLProfileFileReadAllow(t *testing.T) {
	cfg := SeatbeltConfig{}
	pol := DefaultPolicy()
	profile := buildSBPLProfile(cfg, &pol)

	if !strings.Contains(profile, "(allow file-read*") {
		t.Error("expected SBPL profile to contain '(allow file-read*'")
	}
}

// ---------- realPath 测试 ----------

func TestRealPathResolvesSymlinks(t *testing.T) {
	// 在临时目录中创建一个符号链接来测试 realPath
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Skipf("cannot write temp file: %v", err)
	}

	// 如果平台支持符号链接，测试 realPath
	linkPath := filepath.Join(tmpDir, "test_link")
	if err := os.Symlink(testFile, linkPath); err == nil {
		// realPath 应该解析到实际路径
		resolved := realPath(linkPath)
		if resolved == "" {
			t.Error("expected realPath to resolve symlink")
		}
		if resolved != testFile {
			// 在 macOS 上，路径可能不完全匹配（因为 /tmp 可能是 /private/tmp 的链接）
			// 我们只检查解析结果非空且是一个有效路径
			if _, err := os.Stat(resolved); err != nil {
				t.Errorf("resolved path %q does not exist: %v", resolved, err)
			}
		}
	} else {
		t.Skipf("symlink not supported on this platform: %v", err)
	}
}

func TestRealPathReturnsEmptyForNonexistent(t *testing.T) {
	// 对于不存在的路径，realPath 应该返回空字符串
	resolved := realPath("/nonexistent/path/that/does/not/exist")
	if resolved != "" {
		t.Errorf("expected empty string for nonexistent path, got %q", resolved)
	}
}

func TestWriteSBPLProfileCreatesValidFile(t *testing.T) {
	profile := buildSBPLProfile(SeatbeltConfig{}, &DefaultPolicy())
	path, cleanup, err := writeSBPLProfile(profile)
	if err != nil {
		t.Fatalf("writeSBPLProfile failed: %v", err)
	}
	defer cleanup()

	// 文件应该存在
	if _, err := os.Stat(path); err != nil {
		t.Errorf("policy file does not exist: %v", err)
	}

	// 内容应该包含 profile
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read policy file: %v", err)
	}
	if !strings.Contains(string(content), "(deny default)") {
		t.Error("policy file content does not contain '(deny default)'")
	}
}

func TestParseSeatbeltConfigFromMap(t *testing.T) {
	m := map[string]any{
		"timeout":                60,
		"network_allow_outbound": true,
		"writable_paths":         []string{"/tmp", "/var/tmp"},
		"protected_paths":        []string{"/System"},
		"deny_read_paths":        []string{"/etc/shadow"},
		"extra_sbpl_rules":       "(allow test)\n",
		"python_binary":          "/usr/bin/python3",
	}
	cfg := parseSeatbeltConfig(m)

	if cfg.Timeout != 60 {
		t.Errorf("expected Timeout 60, got %d", cfg.Timeout)
	}
	if !cfg.NetworkAllowOutbound {
		t.Error("expected NetworkAllowOutbound true")
	}
	if len(cfg.WritablePaths) != 2 {
		t.Errorf("expected 2 writable paths, got %d", len(cfg.WritablePaths))
	}
	if len(cfg.ProtectedPaths) != 1 {
		t.Errorf("expected 1 protected path, got %d", len(cfg.ProtectedPaths))
	}
	if len(cfg.DenyReadPaths) != 1 {
		t.Errorf("expected 1 deny-read path, got %d", len(cfg.DenyReadPaths))
	}
	if cfg.PythonBinary != "/usr/bin/python3" {
		t.Errorf("expected PythonBinary /usr/bin/python3, got %q", cfg.PythonBinary)
	}
}

func TestParseSeatbeltConfigNilMap(t *testing.T) {
	cfg := parseSeatbeltConfig(nil)
	if cfg.Timeout != 0 {
		t.Errorf("expected default Timeout 0 for nil map, got %d", cfg.Timeout)
	}
}
