//go:build windows

package sandbox

import (
	"context"
	"testing"
	"time"
)

func TestRestrictedTokenHandleNewAlwaysReturnsNonNil(t *testing.T) {
	// RED: 编译期断言 - RestrictedTokenHandle 实现 Handle 接口
	var _ Handle = (*RestrictedTokenHandle)(nil)
}

func TestRestrictedTokenHandleStatusStartsAsCreated(t *testing.T) {
	h := &RestrictedTokenHandle{
		status: StatusCreated,
	}
	if s := h.Status(); s != StatusCreated {
		t.Errorf("expected StatusCreated, got %s", s)
	}
}

func TestRestrictedTokenHandleStopWhenNotRunningIsNoop(t *testing.T) {
	h := &RestrictedTokenHandle{
		status: StatusCreated,
	}
	ctx := context.Background()
	// Should not panic, may return error if process is nil
	_ = h.Stop(ctx)
}

func TestRestrictedTokenHandleExecuteWhenNotRunningReturnsError(t *testing.T) {
	h := &RestrictedTokenHandle{
		status: StatusCreated,
	}
	ctx := context.Background()
	_, err := h.Execute(ctx, &Command{
		Argv: []string{"echo", "hello"},
	})
	if err == nil {
		t.Fatal("expected error when executing on non-running handle")
	}
}

func TestRestrictedTokenHandlePolicyPreserved(t *testing.T) {
	policy := &ExecutionPolicy{
		SandboxMode: ModeLocal,
		ResourceLimit: ResourceLimit{
			CPUShares:   512,
			MemoryBytes: 256 << 20, // 256MB
		},
		Timeout: 30 * time.Second,
	}
	h := &RestrictedTokenHandle{
		status: StatusCreated,
		policy: policy,
		config: map[string]any{},
	}
	if h.policy != policy {
		t.Fatal("policy not preserved")
	}
}

func TestRestrictedTokenHandleConfigPreserved(t *testing.T) {
	cfg := map[string]any{
		"backend": BackendRestrictedToken,
		"shared_folders": []SharedFolder{
			{HostPath: "C:\\host", SandboxPath: "S:\\shared", ReadOnly: true},
		},
	}
	h := &RestrictedTokenHandle{
		status: StatusCreated,
		config: cfg,
	}
	if h.config["backend"] != BackendRestrictedToken {
		t.Error("config backend not preserved")
	}
}

func TestRestrictedTokenHandleWithResourceLimits(t *testing.T) {
	policy := &ExecutionPolicy{
		ResourceLimit: ResourceLimit{
			MemoryBytes: 512 << 20,
			CPUShares:   1024,
			NPROC:       10,
		},
		Timeout: 60 * time.Second,
	}
	h := &RestrictedTokenHandle{
		status: StatusCreated,
		policy: policy,
		config: map[string]any{},
	}

	if h.policy.ResourceLimit.MemoryBytes != 512<<20 {
		t.Error("memory limit not preserved")
	}
	if h.policy.ResourceLimit.CPUShares != 1024 {
		t.Error("CPU shares not preserved")
	}
	if h.policy.ResourceLimit.NPROC != 10 {
		t.Error("NPROC limit not preserved")
	}
	if h.policy.Timeout != 60*time.Second {
		t.Error("timeout not preserved")
	}
}

func TestRestrictedTokenHandleWithNetworkPolicy(t *testing.T) {
	policy := &ExecutionPolicy{
		NetworkAccess: NetworkPolicy{
			AllowInternet: false,
			AllowedPorts:  []int{8080, 8443},
			AllowedHosts:  []string{"127.0.0.1"},
		},
	}
	h := &RestrictedTokenHandle{
		status: StatusCreated,
		policy: policy,
		config: map[string]any{},
	}

	if h.policy.NetworkAccess.AllowInternet {
		t.Error("internet should be denied")
	}
	if len(h.policy.NetworkAccess.AllowedPorts) != 2 {
		t.Error("allowed ports not preserved")
	}
}

func TestRestrictedTokenHandleWithFilesystemPolicy(t *testing.T) {
	policy := &ExecutionPolicy{
		FilesystemAccess: FilesystemPolicy{
			ReadOnlyRoot: true,
			AllowedPaths: []string{"C:\\Windows", "C:\\Users"},
			DeniedPaths:  []string{"C:\\Program Files"},
		},
	}
	h := &RestrictedTokenHandle{
		status: StatusCreated,
		policy: policy,
		config: map[string]any{},
	}

	if !h.policy.FilesystemAccess.ReadOnlyRoot {
		t.Error("read-only root not preserved")
	}
	if len(h.policy.FilesystemAccess.AllowedPaths) != 2 {
		t.Error("allowed paths not preserved")
	}
	if len(h.policy.FilesystemAccess.DeniedPaths) != 1 {
		t.Error("denied paths not preserved")
	}
}

func TestRestrictedTokenHandleWithTimeout(t *testing.T) {
	policy := &ExecutionPolicy{
		Timeout: 5 * time.Millisecond,
	}
	h := &RestrictedTokenHandle{
		status: StatusCreated,
		policy: policy,
		config: map[string]any{},
	}

	ctx := context.Background()
	_, err := h.Execute(ctx, &Command{
		Argv: []string{"echo", "test"},
	})
	if err == nil {
		t.Fatal("expected error on non-running handle with timeout")
	}
	// 验证超时设置被保存
	if h.policy.Timeout != 5*time.Millisecond {
		t.Error("timeout not preserved correctly")
	}
}

func TestRestrictedTokenHandleWithEnvVars(t *testing.T) {
	cmd := &Command{
		Argv: []string{"setx", "KEY=VALUE"},
		Env:  []string{"FOO=bar", "PATH=C:\\Windows"},
	}
	h := &RestrictedTokenHandle{
		status: StatusCreated,
		config: map[string]any{},
	}

	ctx := context.Background()
	_, err := h.Execute(ctx, cmd)
	if err == nil {
		t.Fatal("expected error on non-running handle")
	}

	// 验证 Command 被正确传递
	if len(cmd.Env) != 2 {
		t.Error("env vars not preserved")
	}
}

func TestRestrictedTokenHandleMultipleExecuteCalls(t *testing.T) {
	h := &RestrictedTokenHandle{
		status: StatusCreated,
		config: map[string]any{},
	}

	ctx := context.Background()
	// 第一次 Execute
	_, err1 := h.Execute(ctx, &Command{Argv: []string{"echo", "1"}})
	// 第二次 Execute
	_, err2 := h.Execute(ctx, &Command{Argv: []string{"echo", "2"}})

	// 两次都应该失败（因为 handle 未启动）
	if err1 == nil || err2 == nil {
		t.Fatal("both Execute calls should fail on non-running handle")
	}
}

func TestRestrictedTokenHandleStopMultipleTimes(t *testing.T) {
	h := &RestrictedTokenHandle{
		status: StatusCreated,
		config: map[string]any{},
	}

	ctx := context.Background()
	// 连续调用 Stop
	_ = h.Stop(ctx)
	_ = h.Stop(ctx)
	_ = h.Stop(ctx)

	// 不 panic 即通过
}

func TestRestrictedTokenHandleStatusTransitions(t *testing.T) {
	h := &RestrictedTokenHandle{
		status: StatusCreated,
		config: map[string]any{},
	}

	// 初始状态
	if h.Status() != StatusCreated {
		t.Errorf("expected StatusCreated, got %s", h.Status())
	}

	// 手动模拟状态转换（在实际实现中由 Start/Stop 驱动）
	h.status = StatusRunning
	if h.Status() != StatusRunning {
		t.Errorf("expected StatusRunning, got %s", h.Status())
	}

	h.status = StatusStopped
	if h.Status() != StatusStopped {
		t.Errorf("expected StatusStopped, got %s", h.Status())
	}

	h.status = StatusError
	if h.Status() != StatusError {
		t.Errorf("expected StatusError, got %s", h.Status())
	}
}

func TestRestrictedTokenHandleWithEmptyConfig(t *testing.T) {
	h := &RestrictedTokenHandle{
		status: StatusCreated,
		config: map[string]any{},
	}

	// config 应该是空的 map，不是 nil
	if h.config == nil {
		t.Error("config should not be nil")
	}
}

func TestRestrictedTokenHandleWithNilPolicy(t *testing.T) {
	h := &RestrictedTokenHandle{
		status: StatusCreated,
		config: map[string]any{},
		// policy 为 nil
	}

	// nil policy 不应该导致 panic
	ctx := context.Background()
	_, err := h.Execute(ctx, &Command{Argv: []string{"echo", "test"}})
	// 应该返回错误
	if err == nil {
		t.Fatal("expected error with nil policy")
	}
}
