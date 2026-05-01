package mcp

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// echoHelperSource is a tiny Go program that reads stdin line by
// line and writes each line back to stdout, flushing after every
// write. It is used as a cross-platform echo subprocess for the
// StdioTransport integration tests, avoiding the buffering
// quirks of Windows `findstr` and Unix `cat` (which both behave
// differently across platforms when stdin is a pipe).
const echoHelperSource = `package main

import (
	"bufio"
	"os"
)

func main() {
	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	for {
		line, err := in.ReadString('\n')
		if line != "" {
			out.WriteString(line)
			out.Flush()
		}
		if err != nil {
			return
		}
	}
}
`

// buildEchoHelper writes echoHelperSource to a temp file and returns
// the path. The test uses `go run <path>` to launch it.
func buildEchoHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "echo_helper.go")
	if err := os.WriteFile(path, []byte(echoHelperSource), 0o644); err != nil {
		t.Fatalf("write echo helper: %v", err)
	}
	return path
}

// skipIfNoGo skips the test if the `go` binary is not on PATH.
// `go run` is used to launch the echo helper, so the toolchain
// must be available.
func skipIfNoGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go toolchain not on PATH: %v", err)
	}
}

func TestStdioTransport_StartStop(t *testing.T) {
	skipIfNoGo(t)
	helper := buildEchoHelper(t)

	tr := &StdioTransport{Command: "go", Args: []string{"run", helper}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := tr.Stop(ctx); err != nil && !isNormalExit(err) {
		t.Logf("Stop returned %v (acceptable)", err)
	}
}

// isNormalExit reports whether err is nil or an *exec.ExitError
// indicating the process exited (any status). StdioTransport.Stop
// surfaces the child's exit error, which for `go run` may be
// non-zero when the helper is killed mid-stream.
func isNormalExit(err error) bool {
	if err == nil {
		return true
	}
	_, ok := err.(*exec.ExitError)
	return ok
}

func TestStdioTransport_SendRecvRoundTrip(t *testing.T) {
	skipIfNoGo(t)
	helper := buildEchoHelper(t)

	tr := &StdioTransport{Command: "go", Args: []string{"run", helper}}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = tr.Stop(context.Background()) }()

	// Send the payload. The helper will echo it once `go run`
	// finishes compiling (typically 1-3s on a warm cache). The
	// OS buffers stdin until the helper starts reading.
	payload := []byte(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`)
	if err := tr.Send(ctx, payload); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	got, err := tr.Recv(ctx)
	if err != nil {
		t.Fatalf("Recv failed: %v", err)
	}
	if !bytes.Equal(payload, got) {
		t.Errorf("Recv mismatch:\n want: %s\n got:  %s", payload, got)
	}
}

func TestStdioTransport_SendBeforeStartFails(t *testing.T) {
	tr := &StdioTransport{Command: "cat"}
	if err := tr.Send(context.Background(), []byte("x")); err == nil {
		t.Errorf("Send before Start should fail")
	}
	if _, err := tr.Recv(context.Background()); err == nil {
		t.Errorf("Recv before Start should fail")
	}
}

func TestStdioTransport_StartTwiceFails(t *testing.T) {
	skipIfNoGo(t)
	helper := buildEchoHelper(t)

	tr := &StdioTransport{Command: "go", Args: []string{"run", helper}}
	ctx := context.Background()
	if err := tr.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer func() { _ = tr.Stop(ctx) }()
	if err := tr.Start(ctx); err == nil {
		t.Errorf("second Start should fail")
	}
}

func TestStdioTransport_StopWithoutStartNoop(t *testing.T) {
	tr := &StdioTransport{Command: "cat"}
	if err := tr.Stop(context.Background()); err != nil {
		t.Errorf("Stop on unstarted transport should be no-op, got %v", err)
	}
}

func TestStdioTransport_RecvCancelRespectsContext(t *testing.T) {
	skipIfNoGo(t)
	helper := buildEchoHelper(t)

	tr := &StdioTransport{Command: "go", Args: []string{"run", helper}}
	ctx := context.Background()
	if err := tr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = tr.Stop(ctx) }()

	// Don't send anything — Recv should block until ctx is canceled.
	recvCtx, recvCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer recvCancel()
	if _, err := tr.Recv(recvCtx); err == nil {
		t.Errorf("Recv with canceled ctx should return error")
	}
}
