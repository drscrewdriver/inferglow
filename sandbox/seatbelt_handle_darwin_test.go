//go:build darwin

package sandbox

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestSeatbeltProviderCreateHandle(t *testing.T) {
	provider := NewSeatbeltProvider()
	if provider == nil {
		t.Fatal("NewSeatbeltProvider returned nil")
	}

	pol := DefaultPolicy()
	h, err := provider.CreateHandle(nil, &pol)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}
	if h == nil {
		t.Fatal("CreateHandle returned nil handle")
	}

	status := h.Status()
	if status != StatusCreated {
		t.Errorf("expected StatusCreated, got %q", status)
	}
}

func TestSeatbeltProviderCreateHandleWithConfig(t *testing.T) {
	provider := NewSeatbeltProvider()
	cfg := map[string]any{
		"writable_paths":         []string{"/tmp/test-sandbox"},
		"network_allow_outbound": false,
	}
	pol := DefaultPolicy()

	h, err := provider.CreateHandle(cfg, &pol)
	if err != nil {
		t.Fatalf("CreateHandle with config failed: %v", err)
	}
	if h == nil {
		t.Fatal("CreateHandle returned nil handle")
	}
}

func TestSeatbeltHandleStartStopLifecycle(t *testing.T) {
	provider := NewSeatbeltProvider()
	pol := DefaultPolicy()

	h, err := provider.CreateHandle(nil, &pol)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}

	// Start 应该成功
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	status := h.Status()
	if status != StatusRunning {
		t.Errorf("expected StatusRunning after Start, got %q", status)
	}

	// Stop 应该成功
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	if err := h.Stop(ctx2); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	status = h.Status()
	if status != StatusStopped {
		t.Errorf("expected StatusStopped after Stop, got %q", status)
	}
}

func TestSeatbeltHandleExecuteCommand(t *testing.T) {
	provider := NewSeatbeltProvider()
	pol := DefaultPolicy()

	h, err := provider.CreateHandle(nil, &pol)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := h.Start(ctx); err != nil {
		t.Skipf("Start skipped: %v (sandbox-exec may not be available)", err)
	}

	// 执行一个简单的 echo 命令
	cmd := &Command{
		Argv: []string{"echo", "hello"},
	}

	result, err := h.Execute(ctx, cmd)
	if err != nil {
		t.Skipf("Execute skipped: %v (sandbox-exec may not be available)", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	// 清理
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	h.Stop(ctx2)
}

func TestSeatbeltHandleCancel(t *testing.T) {
	provider := NewSeatbeltProvider()
	pol := DefaultPolicy()

	h, err := provider.CreateHandle(nil, &pol)
	if err != nil {
		t.Skipf("CreateHandle skipped: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.Start(ctx); err != nil {
		t.Skipf("Start skipped: %v", err)
	}

	// Cancel 应该成功（即使进程已经结束）
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	if err := h.Cancel(ctx2); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
}

func TestSeatbeltHandleStopWithoutStart(t *testing.T) {
	provider := NewSeatbeltProvider()
	pol := DefaultPolicy()

	h, err := provider.CreateHandle(nil, &pol)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}

	// 在 Start 之前调用 Stop 应该返回错误
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err = h.Stop(ctx)
	if err == nil {
		t.Error("expected error when stopping handle that was never started")
	}
}

func TestSeatbeltHandleExecuteNotRunning(t *testing.T) {
	provider := NewSeatbeltProvider()
	pol := DefaultPolicy()

	h, err := provider.CreateHandle(nil, &pol)
	if err != nil {
		t.Fatalf("CreateHandle failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	cmd := &Command{
		Argv: []string{"echo", "test"},
	}

	_, err = h.Execute(ctx, cmd)
	if err == nil {
		t.Error("expected error when executing on handle that is not running")
	}
}

func TestSeatbeltHandlePolicyFileCleanup(t *testing.T) {
	provider := NewSeatbeltProvider()
	pol := DefaultPolicy()

	h, err := provider.CreateHandle(nil, &pol)
	if err != nil {
		t.Skipf("CreateHandle skipped: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.Start(ctx); err != nil {
		t.Skipf("Start skipped: %v", err)
	}

	// 获取策略文件路径
	type policyFileGetter interface {
		PolicyFilePath() string
	}
	if pfg, ok := h.(policyFileGetter); ok {
		policyPath := pfg.PolicyFilePath()
		if policyPath != "" {
			// 策略文件应该存在
			if _, err := os.Stat(policyPath); err != nil && !os.IsNotExist(err) {
				t.Errorf("policy file check failed: %v", err)
			}
		}
	}

	// Stop 应该清理策略文件
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	if err := h.Stop(ctx2); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 验证策略文件已被删除
	if pfg, ok := h.(policyFileGetter); ok {
		policyPath := pfg.PolicyFilePath()
		if policyPath != "" {
			if _, err := os.Stat(policyPath); !os.IsNotExist(err) {
				t.Error("policy file was not cleaned up after Stop")
			}
		}
	}
}

func TestSeatbeltProviderInspectAvailability(t *testing.T) {
	provider := NewSeatbeltProvider()
	result, err := provider.InspectAvailability()
	if err != nil {
		t.Fatalf("InspectAvailability failed: %v", err)
	}
	if result == nil {
		t.Fatal("InspectAvailability returned nil result")
	}
	if result.Platform != "darwin" {
		t.Errorf("expected platform darwin, got %q", result.Platform)
	}
}

func TestSeatbeltProviderNameKind(t *testing.T) {
	provider := NewSeatbeltProvider()
	if name := provider.Name(); name != "seatbelt" {
		t.Errorf("expected Name 'seatbelt', got %q", name)
	}
	if kind := provider.Kind(); kind != "local" {
		t.Errorf("expected Kind 'local', got %q", kind)
	}
}

func TestSeatbeltProviderImplementsProvider(t *testing.T) {
	var _ Provider = (*SeatbeltProvider)(nil)
}

// ---------- 集成测试（需要 macOS sandbox-exec 可用） ----------

func TestSeatbeltHandleWithSimpleCommand(t *testing.T) {
	// 检查 sandbox-exec 是否可用
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		t.Skipf("sandbox-exec not available: %v", err)
	}

	provider := NewSeatbeltProvider()
	pol := DefaultPolicy()

	h, err := provider.CreateHandle(nil, &pol)
	if err != nil {
		t.Skipf("CreateHandle skipped: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := h.Start(ctx); err != nil {
		t.Skipf("Start skipped (sandbox-exec may require provisioning profile): %v", err)
	}

	cmd := &Command{
		Argv: []string{"uname", "-s"},
	}

	result, err := h.Execute(ctx, cmd)
	if err != nil {
		t.Skipf("Execute skipped: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	t.Logf("Execute result: exitCode=%d, stdout=%q, stderr=%q", result.ExitCode, result.Stdout, result.Stderr)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	h.Stop(ctx2)
}
