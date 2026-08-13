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

//go:build windows

package sandbox

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestRestrictedTokenHandleExecuteCapturesStdout runs a real command under
// the restricted token and verifies that stdout/stderr are captured through
// the anonymous pipes wired by launchProcessWithIO.
//
// The test is skipped when the restricted token cannot be created or the
// process cannot be launched (e.g. missing SE_INCREASE_QUOTA_NAME).
func TestRestrictedTokenHandleExecuteCapturesStdout(t *testing.T) {
	handle := &RestrictedTokenHandle{
		config: map[string]any{},
		status: StatusCreated,
	}
	ctx := context.Background()

	if err := handle.Start(ctx); err != nil {
		t.Skipf("restricted token unavailable: %v", err)
	}
	defer handle.Stop(ctx)

	result, err := handle.Execute(ctx, &Command{
		Argv: []string{"cmd", "/c", "echo", "hello-sandbox"},
	})
	if err != nil {
		t.Skipf("execute under restricted token unavailable: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0 (stderr=%q)", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "hello-sandbox") {
		t.Errorf("stdout = %q, want it to contain %q", result.Stdout, "hello-sandbox")
	}
	if result.Duration <= 0 {
		t.Errorf("duration = %v, want > 0", result.Duration)
	}
}

// TestRestrictedTokenHandleExecuteCapturesStderr verifies the stderr pipe is
// wired independently of stdout.
func TestRestrictedTokenHandleExecuteCapturesStderr(t *testing.T) {
	handle := &RestrictedTokenHandle{
		config: map[string]any{},
		status: StatusCreated,
	}
	ctx := context.Background()

	if err := handle.Start(ctx); err != nil {
		t.Skipf("restricted token unavailable: %v", err)
	}
	defer handle.Stop(ctx)

	result, err := handle.Execute(ctx, &Command{
		Argv: []string{"cmd", "/c", "echo", "err-line", "1>&2"},
	})
	if err != nil {
		t.Skipf("execute under restricted token unavailable: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}
	if !strings.Contains(result.Stderr, "err-line") {
		t.Errorf("stderr = %q, want it to contain %q", result.Stderr, "err-line")
	}
}

// TestRestrictedTokenHandleExecuteTimeout verifies that a hung command is
// terminated on ctx cancellation and no goroutine leaks are left behind.
func TestRestrictedTokenHandleExecuteTimeout(t *testing.T) {
	handle := &RestrictedTokenHandle{
		config: map[string]any{},
		status: StatusCreated,
	}
	ctx := context.Background()

	if err := handle.Start(ctx); err != nil {
		t.Skipf("restricted token unavailable: %v", err)
	}
	defer handle.Stop(ctx)

	before := countGoroutines()
	execCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()

	_, err := handle.Execute(execCtx, &Command{
		Argv: []string{"cmd", "/c", "ping", "-n", "60", "127.0.0.1"},
	})
	if err == nil {
		t.Error("Execute should have returned a timeout error")
	}

	// Give any stray goroutines a chance to wind down.
	time.Sleep(200 * time.Millisecond)
	after := countGoroutines()
	if after > before+2 {
		t.Errorf("goroutine count grew from %d to %d after timeout", before, after)
	}
}

// TestAppContainerTokenHasContainerSid verifies that the token derived by
// createAppContainerToken really carries the AppContainer SID, i.e. child
// processes started under it run with the container identity.
func TestAppContainerTokenHasContainerSid(t *testing.T) {
	profileName := fmt.Sprintf("inferglow-test-%d", time.Now().UnixNano())
	sid, err := createAppContainerProfile(profileName)
	if err != nil {
		t.Skipf("CreateAppContainerProfile unavailable: %v", err)
	}
	defer deleteAppContainerProfile(profileName)
	defer freeSID(sid)

	token, err := createAppContainerToken(sid)
	if err != nil {
		t.Skipf("CreateAppContainerToken unavailable: %v", err)
	}
	defer token.Close()

	got, err := tokenAppContainerSID(token)
	if err != nil {
		t.Fatalf("GetTokenInformation(TokenAppContainerSid): %v", err)
	}
	if got == nil {
		t.Fatal("token has no AppContainer SID: not an AppContainer token")
	}

	// The returned SID must be the profile SID we created.
	gotStr, _ := got.String()
	wantStr, _ := sid.String()
	if gotStr != wantStr {
		t.Errorf("token AppContainer SID = %s, want profile SID %s", gotStr, wantStr)
	}
}

// TestLaunchProcessWithIONilToken verifies the launcher fails closed when no
// token is provided.
func TestLaunchProcessWithIONilToken(t *testing.T) {
	_, err := launchProcessWithIO(context.Background(), 0, []string{"cmd", "/c", "echo", "x"}, nil, "")
	if err == nil {
		t.Error("launchProcessWithIO with zero token should fail")
	}
}

// countGoroutines returns the current goroutine count.
func countGoroutines() int {
	return runtime.NumGoroutine()
}
