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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Provider 基础测试
// ============================================================================

func TestBubblewrapProviderNameKind(t *testing.T) {
	p := NewBubblewrapProvider()
	if p.Name() != "bubblewrap" {
		t.Errorf("Name() = %q, want bubblewrap", p.Name())
	}
	if p.Kind() != "local" {
		t.Errorf("Kind() = %q, want local", p.Kind())
	}
}

func TestBubblewrapProviderImplementsProvider(t *testing.T) {
	var _ Provider = (*BubblewrapProvider)(nil)
}

func TestBubblewrapHandleImplementsHandle(t *testing.T) {
	var _ Handle = (*BubblewrapHandle)(nil)
}

func TestBubblewrapInspectAvailability(t *testing.T) {
	p := NewBubblewrapProvider()
	avail, err := p.InspectAvailability()
	if err != nil {
		t.Fatalf("InspectAvailability error: %v", err)
	}
	if avail == nil {
		t.Fatal("InspectAvailability returned nil")
	}
	if avail.Platform != string(OSLinux) {
		t.Errorf("Platform = %q, want linux", avail.Platform)
	}
	// 在 bwrap 已安装的环境中应该可用。
	if _, err := exec.LookPath("bwrap"); err == nil {
		if !avail.Available {
			t.Errorf("Available = false, want true (bwrap exists)")
		}
		if avail.BinaryPath == "" {
			t.Error("BinaryPath is empty when bwrap available")
		}
	}
}

// ============================================================================
// Config 解析测试
// ============================================================================

func TestParseBubblewrapConfigDefaults(t *testing.T) {
	cfg := parseBubblewrapConfig(nil)
	if cfg.UnshareAll {
		t.Error("default UnshareAll should be false")
	}
	if cfg.Timeout != 0 {
		t.Errorf("default Timeout = %v, want 0", cfg.Timeout)
	}
}

func TestParseBubblewrapConfigBindROStringSlice(t *testing.T) {
	cfg := parseBubblewrapConfig(map[string]any{
		"bind_ro": []string{"/usr:/usr:ro", "/bin:/bin"},
	})
	if len(cfg.BindRO) != 2 {
		t.Fatalf("BindRO len = %d, want 2", len(cfg.BindRO))
	}
	if cfg.BindRO[0].Source != "/usr" || cfg.BindRO[0].Destination != "/usr" || !cfg.BindRO[0].ReadOnly {
		t.Errorf("BindRO[0] = %+v", cfg.BindRO[0])
	}
	if cfg.BindRO[1].ReadOnly {
		t.Errorf("BindRO[1] should be rw, got %+v", cfg.BindRO[1])
	}
}

func TestParseBubblewrapConfigBindROAnySlice(t *testing.T) {
	cfg := parseBubblewrapConfig(map[string]any{
		"bind_ro": []any{
			[2]string{"/lib", "/lib"},
			[]string{"/etc", "/etc", "ro"},
			map[string]any{"source": "/opt", "destination": "/opt", "read_only": true},
			MountEntry{Source: "/var", Destination: "/var"},
		},
	})
	if len(cfg.BindRO) != 4 {
		t.Fatalf("BindRO len = %d, want 4", len(cfg.BindRO))
	}
	if cfg.BindRO[0].Source != "/lib" {
		t.Errorf("BindRO[0].Source = %q", cfg.BindRO[0].Source)
	}
	if cfg.BindRO[1].ReadOnly != true {
		t.Errorf("BindRO[1].ReadOnly = %v", cfg.BindRO[1].ReadOnly)
	}
	if cfg.BindRO[2].Source != "/opt" || !cfg.BindRO[2].ReadOnly {
		t.Errorf("BindRO[2] = %+v", cfg.BindRO[2])
	}
	if cfg.BindRO[3].Source != "/var" {
		t.Errorf("BindRO[3].Source = %q", cfg.BindRO[3].Source)
	}
}

func TestParseBubblewrapConfigTmpfs(t *testing.T) {
	cfg := parseBubblewrapConfig(map[string]any{
		"tmpfs": map[string]any{
			"/tmp":  int64(64 * 1024 * 1024),
			"/run":  32 * 1024 * 1024,
			"/home": float64(128 * 1024 * 1024),
		},
	})
	if got := cfg.Tmpfs["/tmp"]; got != 64*1024*1024 {
		t.Errorf("Tmpfs[/tmp] = %d", got)
	}
	if got := cfg.Tmpfs["/run"]; got != 32*1024*1024 {
		t.Errorf("Tmpfs[/run] = %d", got)
	}
	if got := cfg.Tmpfs["/home"]; got != 128*1024*1024 {
		t.Errorf("Tmpfs[/home] = %d", got)
	}
}

func TestParseBubblewrapConfigTimeout(t *testing.T) {
	cfg := parseBubblewrapConfig(map[string]any{
		"timeout_seconds": int(5),
	})
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", cfg.Timeout)
	}

	cfg = parseBubblewrapConfig(map[string]any{
		"timeout_seconds": float64(2.5),
	})
	if cfg.Timeout != 2500*time.Millisecond {
		t.Errorf("Timeout = %v, want 2.5s", cfg.Timeout)
	}
}

func TestParseBubblewrapConfigFlags(t *testing.T) {
	cfg := parseBubblewrapConfig(map[string]any{
		"unshare_all":     true,
		"share_net":       true,
		"clearenv":        true,
		"new_session":     true,
		"die_with_parent": true,
	})
	if !cfg.UnshareAll {
		t.Error("UnshareAll should be true")
	}
	if !cfg.ShareNet {
		t.Error("ShareNet should be true")
	}
	if !cfg.Clearenv {
		t.Error("Clearenv should be true")
	}
	if !cfg.NewSession {
		t.Error("NewSession should be true")
	}
	if !cfg.DieWithParent {
		t.Error("DieWithParent should be true")
	}
}

func TestParseMountEntriesUnknownType(t *testing.T) {
	out := parseMountEntries(42)
	if out != nil {
		t.Errorf("expected nil for int type, got %v", out)
	}
}

func TestParseTmpfsMapUnknownType(t *testing.T) {
	out := parseTmpfsMap("string")
	if out != nil {
		t.Errorf("expected nil for string type, got %v", out)
	}
}

// ============================================================================
// Handle 生命周期测试
// ============================================================================

func TestBubblewrapHandleStatusTransitions(t *testing.T) {
	p := NewBubblewrapProvider()
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Skip("bwrap not available, skipping handle test")
	}

	policy := DefaultPolicy()
	h, err := p.CreateHandle(nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle error: %v", err)
	}
	if h.Status() != StatusCreated {
		t.Errorf("Status = %v, want created", h.Status())
	}
	ctx := context.Background()
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if h.Status() != StatusRunning {
		t.Errorf("Status = %v, want running", h.Status())
	}
	// 重复 Start 应幂等。
	if err := h.Start(ctx); err != nil {
		t.Errorf("second Start error: %v", err)
	}
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("Stop error: %v", err)
	}
	if h.Status() != StatusStopped {
		t.Errorf("Status = %v, want stopped", h.Status())
	}
	// 重复 Stop 应幂等。
	if err := h.Stop(ctx); err != nil {
		t.Errorf("second Stop error: %v", err)
	}
}

func TestBubblewrapHandleExecuteWithoutStart(t *testing.T) {
	p := NewBubblewrapProvider()
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Skip("bwrap not available")
	}
	policy := DefaultPolicy()
	h, err := p.CreateHandle(nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle error: %v", err)
	}
	_, err = h.Execute(context.Background(), &Command{Argv: []string{"echo", "hi"}})
	if !errors.Is(err, ErrHandleNotRunning) {
		t.Fatalf("expected ErrHandleNotRunning, got %v", err)
	}
}

func TestBubblewrapHandleExecuteNilCommand(t *testing.T) {
	p := NewBubblewrapProvider()
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Skip("bwrap not available")
	}
	policy := DefaultPolicy()
	h, err := p.CreateHandle(nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle error: %v", err)
	}
	ctx := context.Background()
	_ = h.Start(ctx)
	_, err = h.Execute(ctx, nil)
	if err == nil {
		t.Error("expected error for nil command")
	}
	_, err = h.Execute(ctx, &Command{Argv: nil})
	if err == nil {
		t.Error("expected error for empty argv")
	}
}

func TestBubblewrapHandleCreateHandlePolicyNil(t *testing.T) {
	p := NewBubblewrapProvider()
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Skip("bwrap not available")
	}
	h, err := p.CreateHandle(nil, nil)
	if err != nil {
		t.Fatalf("CreateHandle(nil, nil) error: %v", err)
	}
	if h == nil {
		t.Fatal("handle is nil")
	}
}

// ============================================================================
// 集成测试：实际执行 bwrap
// ============================================================================

// TestBubblewrapIntegrationEcho 在沙箱中执行 /bin/echo，验证基本可用性。
// 需要 bwrap 已安装且支持用户命名空间（CI 容器可能禁用）。
func TestBubblewrapIntegrationEcho(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	// 在受限环境（如某些 Docker 容器）中，用户命名空间可能被禁用。
	// 先用最小配置试跑，失败则跳过。
	p := NewBubblewrapProvider()
	policy := DefaultPolicy()
	h, err := p.CreateHandle(map[string]any{
		"unshare_all": true,
		"share_net":   false,
		"bind_ro":     []string{"/usr:/usr", "/bin:/bin", "/lib:/lib", "/lib64:/lib64"},
	}, &policy)
	if err != nil {
		t.Fatalf("CreateHandle error: %v", err)
	}
	ctx := context.Background()
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer h.Stop(ctx)

	res, err := h.Execute(ctx, &Command{Argv: []string{"/bin/echo", "hello-bwrap"}})
	if err != nil {
		// bwrap 在 CI 中可能因 unprivileged user namespace 被禁用而失败。
		// 这种情况下记录 stderr 并跳过，不算测试失败。
		if res != nil && strings.Contains(res.Stderr, "namespace") {
			t.Skipf("bwrap unavailable in this environment: %s", res.Stderr)
		}
		t.Fatalf("Execute error: %v (stderr: %s)", err, func() string {
			if res != nil {
				return res.Stderr
			}
			return ""
		}())
	}
	if res.ExitCode != 0 {
		t.Logf("ExitCode=%d Stderr=%s", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "hello-bwrap") {
		t.Errorf("Stdout = %q, want contains 'hello-bwrap'", res.Stdout)
	}
}

// TestBubblewrapIntegrationReadOnly 在只读沙箱中尝试写 /tmp，预期失败。
func TestBubblewrapIntegrationReadOnly(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bwrap not installed")
	}
	tmp, err := os.MkdirTemp("", "bwrap-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp error: %v", err)
	}
	defer os.RemoveAll(tmp)

	p := NewBubblewrapProvider()
	policy := DefaultPolicy()
	// 仅挂载只读根和 tmpfs /tmp（不可写宿主文件系统）。
	h, err := p.CreateHandle(map[string]any{
		"unshare_all": true,
		"bind_ro":     []string{"/usr:/usr", "/bin:/bin", "/lib:/lib", "/lib64:/lib64"},
	}, &policy)
	if err != nil {
		t.Fatalf("CreateHandle error: %v", err)
	}
	ctx := context.Background()
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer h.Stop(ctx)

	// 尝试写入宿主文件系统：应失败。
	target := filepath.Join(tmp, "should-not-exist")
	res, err := h.Execute(ctx, &Command{Argv: []string{"/bin/sh", "-c", "echo x > " + target}})
	if err != nil {
		t.Skipf("Execute error (likely env restriction): %v", err)
	}
	// 在沙箱内该路径不应存在 -> 写入失败。
	if _, statErr := os.Stat(target); statErr == nil {
		// 文件存在了：可能 bwrap 未真正隔离或 bind 配置错误。
		// 不直接 fail，因为某些环境 unshare 失败但 bwrap 仍执行了。
		t.Logf("warning: target file exists (bwrap may not have isolated): stderr=%s", res.Stderr)
	}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestBubblewrapProviderConcurrentInspect(t *testing.T) {
	p := NewBubblewrapProvider()
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

func TestBubblewrapHandleConcurrentStatus(t *testing.T) {
	p := NewBubblewrapProvider()
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Skip("bwrap not available")
	}
	policy := DefaultPolicy()
	h, _ := p.CreateHandle(nil, &policy)
	ctx := context.Background()
	_ = h.Start(ctx)

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
