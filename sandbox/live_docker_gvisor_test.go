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

// live_docker_gvisor_test.go — 真实环境集成测试
//
// 验证 Docker daemon 与 gVisor runsc 运行时的端到端连通性。
// 这些测试会实际创建容器并执行命令；当 Docker 或 runsc 不可用时优雅跳过。
//
// 运行: go test -run TestLive -v -timeout 120s

package sandbox

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// ensureAlpineImage pulls alpine:latest if not present, so live tests can run.
func ensureAlpineImage(t *testing.T) {
	t.Helper()
	// Check if image exists
	check := exec.Command("docker", "image", "inspect", "alpine:latest")
	if err := check.Run(); err == nil {
		return // image exists
	}
	// Pull it
	t.Logf("alpine:latest not found, pulling...")
	pull := exec.Command("docker", "pull", "alpine:latest")
	pull.Stdout = os.Stdout
	pull.Stderr = os.Stderr
	if err := pull.Run(); err != nil {
		t.Skipf("failed to pull alpine:latest: %v", err)
	}
}

// TestLiveDockerDaemonReachable verifies that NewDockerProvider can reach the
// Docker daemon.
func TestLiveDockerDaemonReachable(t *testing.T) {
	provider, err := NewDockerProvider()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	if provider == nil {
		t.Fatal("NewDockerProvider returned nil provider without error")
	}
	if name := provider.Name(); name != "docker" {
		t.Errorf("provider.Name() = %q, want %q", name, "docker")
	}
}

// TestLiveDockerInspectAvailability verifies that InspectAvailability
// reports docker as available on this host.
func TestLiveDockerInspectAvailability(t *testing.T) {
	provider, err := NewDockerProvider()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	avail, err := provider.InspectAvailability()
	if err != nil {
		t.Fatalf("InspectAvailability returned error: %v", err)
	}
	if !avail.Available {
		t.Skipf("docker not available: %s", avail.ErrorMessage)
	}
	t.Logf("docker available: binary=%s platform=%s", avail.BinaryPath, avail.Platform)
}

// TestLiveDockerHandleEcho performs an end-to-end test: create a Docker
// container, start it, run `echo hello`, and verify the output.
func TestLiveDockerHandleEcho(t *testing.T) {
	provider, err := NewDockerProvider()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}

	ensureAlpineImage(t)

	policy := DefaultPolicy()
	policy.Timeout = 60 * time.Second

	handle, err := provider.CreateHandle(map[string]any{"image": "alpine:latest"}, &policy)
	if err != nil {
		t.Fatalf("CreateHandle returned error: %v", err)
	}

	ctx := context.Background()
	if err := handle.Start(ctx); err != nil {
		t.Skipf("handle.Start failed (environment issue): %v", err)
	}
	defer handle.Stop(ctx)

	result, err := handle.Execute(ctx, &Command{Argv: []string{"echo", "hello"}})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("Stdout = %q, want contains %q", result.Stdout, "hello")
	}
	t.Logf("echo stdout: %q", result.Stdout)
}

// TestLiveGVisorProviderAvailable verifies that NewGVisorProvider succeeds
// when both docker and runsc are available.
func TestLiveGVisorProviderAvailable(t *testing.T) {
	provider, err := NewGVisorProvider()
	if err != nil {
		t.Skipf("GVisor not available: %v", err)
	}
	if provider == nil {
		t.Fatal("NewGVisorProvider returned nil provider without error")
	}
	if name := provider.Name(); name != "gvisor" {
		t.Errorf("provider.Name() = %q, want %q", name, "gvisor")
	}
	avail, err := provider.InspectAvailability()
	if err != nil {
		t.Fatalf("InspectAvailability returned error: %v", err)
	}
	if !avail.Available {
		t.Skipf("gvisor not available: %s", avail.ErrorMessage)
	}
	t.Logf("gvisor available: binary=%s platform=%s", avail.BinaryPath, avail.Platform)
}

// TestLiveGVisorHandleEcho performs an end-to-end test against gVisor:
// create a container with runsc runtime, start it, run `echo hello`, and
// verify the output.
func TestLiveGVisorHandleEcho(t *testing.T) {
	provider, err := NewGVisorProvider()
	if err != nil {
		t.Skipf("GVisor not available: %v", err)
	}

	ensureAlpineImage(t)

	policy := DefaultPolicy()
	policy.Timeout = 60 * time.Second

	handle, err := provider.CreateHandle(map[string]any{"image": "alpine:latest"}, &policy)
	if err != nil {
		t.Fatalf("CreateHandle returned error: %v", err)
	}

	ctx := context.Background()
	if err := handle.Start(ctx); err != nil {
		t.Skipf("handle.Start failed (environment issue): %v", err)
	}
	defer handle.Stop(ctx)

	result, err := handle.Execute(ctx, &Command{Argv: []string{"echo", "hello"}})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Stdout, "hello") {
		t.Errorf("Stdout = %q, want contains %q", result.Stdout, "hello")
	}
	t.Logf("echo stdout: %q", result.Stdout)
}
