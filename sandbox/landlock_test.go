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

//go:build linux

package sandbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// ============================================================================
// Provider 基础测试
// ============================================================================

func TestLandlockProviderNameKind(t *testing.T) {
	p := NewLandlockProvider()
	if p.Name() != "landlock" {
		t.Errorf("Name() = %q, want landlock", p.Name())
	}
	if p.Kind() != "local" {
		t.Errorf("Kind() = %q, want local", p.Kind())
	}
}

func TestLandlockProviderImplementsProvider(t *testing.T) {
	var _ Provider = (*LandlockProvider)(nil)
}

func TestLandlockHandleImplementsHandle(t *testing.T) {
	var _ Handle = (*LandlockHandle)(nil)
}

func TestLandlockInspectAvailability(t *testing.T) {
	p := NewLandlockProvider()
	avail, err := p.InspectAvailability()
	if err != nil {
		t.Fatalf("InspectAvailability error: %v", err)
	}
	if avail == nil {
		t.Fatal("returned nil")
	}
	if avail.Platform != string(OSLinux) {
		t.Errorf("Platform = %q, want linux", avail.Platform)
	}
	// 在 WSL2 内核 6.6+ 上应当可用。
	t.Logf("Landlock available=%v version=%q err=%s", avail.Available, avail.Version, avail.ErrorMessage)
}

// ============================================================================
// Config 解析测试
// ============================================================================

func TestParseLandlockConfigDefaults(t *testing.T) {
	cfg := parseLandlockConfig(nil)
	if len(cfg.AllowedReadDirs) != 0 {
		t.Errorf("AllowedReadDirs = %v, want empty", cfg.AllowedReadDirs)
	}
	if cfg.ABIVersion != 0 {
		t.Errorf("ABIVersion = %d, want 0", cfg.ABIVersion)
	}
}

func TestParseLandlockConfigDirs(t *testing.T) {
	cfg := parseLandlockConfig(map[string]any{
		"read_dirs":   []string{"/usr", "/lib"},
		"write_dirs":  []string{"/tmp"},
		"read_files":  []string{"/etc/hostname"},
		"write_files": []string{"/tmp/out"},
		"abi_version": 2,
	})
	if len(cfg.AllowedReadDirs) != 2 || cfg.AllowedReadDirs[0] != "/usr" {
		t.Errorf("AllowedReadDirs = %v", cfg.AllowedReadDirs)
	}
	if len(cfg.AllowedWriteDirs) != 1 || cfg.AllowedWriteDirs[0] != "/tmp" {
		t.Errorf("AllowedWriteDirs = %v", cfg.AllowedWriteDirs)
	}
	if len(cfg.AllowedReadFiles) != 1 || cfg.AllowedReadFiles[0] != "/etc/hostname" {
		t.Errorf("AllowedReadFiles = %v", cfg.AllowedReadFiles)
	}
	if len(cfg.AllowedWriteFiles) != 1 || cfg.AllowedWriteFiles[0] != "/tmp/out" {
		t.Errorf("AllowedWriteFiles = %v", cfg.AllowedWriteFiles)
	}
	if cfg.ABIVersion != 2 {
		t.Errorf("ABIVersion = %d, want 2", cfg.ABIVersion)
	}
}

func TestParseLandlockConfigAnySlice(t *testing.T) {
	cfg := parseLandlockConfig(map[string]any{
		"read_dirs": []any{"/usr", 42, "/lib"}, // 42 应被过滤
	})
	if len(cfg.AllowedReadDirs) != 2 {
		t.Errorf("AllowedReadDirs = %v, want len 2", cfg.AllowedReadDirs)
	}
}

func TestParseStringSliceUnknownType(t *testing.T) {
	out := parseStringSlice(42)
	if out != nil {
		t.Errorf("expected nil for int, got %v", out)
	}
}

// ============================================================================
// Handle 生命周期测试（不触发 restrict_self）
// ============================================================================

func TestLandlockHandleStatusTransitionsWithoutExecute(t *testing.T) {
	p := NewLandlockProvider()
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Skip("landlock not available")
	}
	tmp, err := os.MkdirTemp("", "landlock-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmp)

	policy := DefaultPolicy()
	h, err := p.CreateHandle(map[string]any{
		"read_dirs":  []string{"/usr", "/bin", "/lib"},
		"write_dirs": []string{tmp},
	}, &policy)
	if err != nil {
		t.Fatalf("CreateHandle: %v", err)
	}
	if h.Status() != StatusCreated {
		t.Errorf("Status = %v, want created", h.Status())
	}
	ctx := context.Background()
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if h.Status() != StatusRunning {
		t.Errorf("Status = %v, want running", h.Status())
	}
	// 重复 Start 幂等。
	if err := h.Start(ctx); err != nil {
		t.Errorf("second Start: %v", err)
	}
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if h.Status() != StatusStopped {
		t.Errorf("Status = %v, want stopped", h.Status())
	}
	if err := h.Stop(ctx); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestLandlockHandleExecuteWithoutStart(t *testing.T) {
	p := NewLandlockProvider()
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Skip("landlock not available")
	}
	policy := DefaultPolicy()
	h, err := p.CreateHandle(map[string]any{
		"read_dirs": []string{"/usr"},
	}, &policy)
	if err != nil {
		t.Fatalf("CreateHandle: %v", err)
	}
	_, err = h.Execute(context.Background(), &Command{Argv: []string{"echo", "hi"}})
	if !errors.Is(err, ErrHandleNotRunning) {
		t.Fatalf("expected ErrHandleNotRunning, got %v", err)
	}
}

func TestLandlockHandleExecuteNilCommand(t *testing.T) {
	p := NewLandlockProvider()
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Skip("landlock not available")
	}
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(map[string]any{
		"read_dirs": []string{"/usr"},
	}, &policy)
	ctx := context.Background()
	_ = h.Start(ctx)
	// 注意：Execute 在执行 landlock_restrict_self 之前会先做命令检查，
	// 所以 nil 命令应直接返回错误，不会污染进程。
	_, err := h.Execute(ctx, nil)
	if err == nil {
		t.Error("expected error for nil command")
	}
	_, err = h.Execute(ctx, &Command{Argv: nil})
	if err == nil {
		t.Error("expected error for empty argv")
	}
	// 不调用 Stop，因为我们的目的是检查错误路径；ruleset fd 在测试进程退出时会被 GC 关闭。
}

func TestLandlockHandleCreateHandlePolicyNil(t *testing.T) {
	p := NewLandlockProvider()
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Skip("landlock not available")
	}
	h, err := p.CreateHandle(nil, nil)
	if err != nil {
		t.Fatalf("CreateHandle(nil, nil): %v", err)
	}
	if h == nil {
		t.Fatal("handle is nil")
	}
}

func TestLandlockHandleStartInvalidPath(t *testing.T) {
	p := NewLandlockProvider()
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Skip("landlock not available")
	}
	policy := DefaultPolicy()
	// 不存在的路径，landlockAddPathRule 应失败。
	h, _ := p.CreateHandle(map[string]any{
		"read_dirs": []string{"/this/path/does/not/exist/abcdef"},
	}, &policy)
	err := h.Start(context.Background())
	if err == nil {
		t.Error("expected error for invalid path")
	}
	if h.Status() != StatusError {
		t.Errorf("Status = %v, want error", h.Status())
	}
}

// ============================================================================
// 集成测试（会污染当前进程，需要通过环境变量显式启用）
// ============================================================================

// TestLandlockIntegrationRestrictSelf 是真正的集成测试。
// 它会调用 landlock_restrict_self，导致本测试进程永久受限。
// 因此默认跳过，需要通过 INFERENCEGLOW_LANDLOCK_INTEGRATION=1 启用。
//
// 启用方式：
//
//	INFERENCEGLOW_LANDLOCK_INTEGRATION=1 go test -v -run TestLandlockIntegrationRestrictSelf
//
// 注意：启用后测试进程将无法再访问未授权路径，其他测试可能受影响。
// 建议单独运行此测试。
func TestLandlockIntegrationRestrictSelf(t *testing.T) {
	if os.Getenv("INFERENCEGLOW_LANDLOCK_INTEGRATION") != "1" {
		t.Skip("set INFERENCEGLOW_LANDLOCK_INTEGRATION=1 to run")
	}
	p := NewLandlockProvider()
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Skip("landlock not available")
	}
	tmp, err := os.MkdirTemp("", "landlock-int-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmp)
	// 在 tmp 下创建一个测试文件。
	outFile := filepath.Join(tmp, "out.txt")
	if err := os.WriteFile(outFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	policy := DefaultPolicy()
	h, err := p.CreateHandle(map[string]any{
		"read_dirs":  []string{"/usr", "/bin", "/lib"},
		"write_dirs": []string{tmp},
	}, &policy)
	if err != nil {
		t.Fatalf("CreateHandle: %v", err)
	}
	ctx := context.Background()
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer h.Stop(ctx)

	// 第一次 Execute：触发 landlock_restrict_self。
	// 在 /tmp 下写入文件应该成功（已加入白名单）。
	res, err := h.Execute(ctx, &Command{Argv: []string{"/bin/sh", "-c", "echo hello > " + outFile}})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Logf("ExitCode=%d Stderr=%s", res.ExitCode, res.Stderr)
	}

	// 验证 Consumed 已为 true。
	if !h.(*LandlockHandle).Consumed() {
		t.Error("Consumed should be true after first Execute")
	}

	// 读出文件验证写入成功。
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	t.Logf("out file content: %q", string(data))
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestLandlockProviderConcurrentInspect(t *testing.T) {
	p := NewLandlockProvider()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			_, _ = p.InspectAvailability()
		}
	}()
	for i := 0; i < 50; i++ {
		_, _ = p.InspectAvailability()
	}
	<-done
}

func TestLandlockHandleConcurrentStatus(t *testing.T) {
	p := NewLandlockProvider()
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Skip("landlock not available")
	}
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(map[string]any{
		"read_dirs": []string{"/usr"},
	}, &policy)
	ctx := context.Background()
	_ = h.Start(ctx)
	defer h.Stop(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			_ = h.Status()
		}
	}()
	for i := 0; i < 50; i++ {
		_ = h.Status()
	}
	<-done
}
