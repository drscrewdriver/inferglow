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

package agent

import (
	"errors"
	"testing"
	"time"
)

// TestCancelMode_BitwiseOR verifies that CancelAfterChatModel |
// CancelAfterToolCalls combines into a mode that matches either safe-point via
// bitwise AND.
func TestCancelMode_BitwiseOR(t *testing.T) {
	combined := CancelAfterChatModel | CancelAfterToolCalls

	// The combined mode must include both individual bits.
	if combined&CancelAfterChatModel == 0 {
		t.Errorf("combined mode %d does not include CancelAfterChatModel (%d)",
			combined, CancelAfterChatModel)
	}
	if combined&CancelAfterToolCalls == 0 {
		t.Errorf("combined mode %d does not include CancelAfterToolCalls (%d)",
			combined, CancelAfterToolCalls)
	}

	// The combined value must differ from each individual value.
	if combined == CancelAfterChatModel {
		t.Errorf("combined == CancelAfterChatModel, expected a distinct value")
	}
	if combined == CancelAfterToolCalls {
		t.Errorf("combined == CancelAfterToolCalls, expected a distinct value")
	}

	// CancelImmediate is 0; OR-ing it must not change a safe-point mode.
	if (combined | CancelImmediate) != combined {
		t.Errorf("OR with CancelImmediate changed the mode")
	}
}

// TestCancelMode_String verifies the String method for each mode and a
// combined mode.
func TestCancelMode_String(t *testing.T) {
	cases := []struct {
		mode CancelMode
		want string
	}{
		{CancelImmediate, "immediate"},
		{CancelAfterChatModel, "after_chat_model"},
		{CancelAfterToolCalls, "after_tool_calls"},
		{CancelAfterChatModel | CancelAfterToolCalls, "after_chat_model|after_tool_calls"},
	}
	for _, c := range cases {
		if got := c.mode.String(); got != c.want {
			t.Errorf("CancelMode(%d).String() = %q, want %q", c.mode, got, c.want)
		}
	}
}

// TestCancelHandle_Wait verifies that a standalone CancelHandle's Wait
// unblocks when its done channel is closed and returns the handle's err.
func TestCancelHandle_Wait(t *testing.T) {
	h := &CancelHandle{done: make(chan struct{})}

	// Close done in a goroutine so Wait can observe the unblock.
	go func() {
		close(h.done)
	}()

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- h.Wait()
	}()

	select {
	case err := <-doneCh:
		if err != nil {
			t.Errorf("Wait returned non-nil error for standalone handle: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for CancelHandle.Wait")
	}
}

// TestCancelManager_ImmediateCancel verifies that Cancel(CancelImmediate)
// registers a pending request that fires at the immediate safe-point.
func TestCancelManager_ImmediateCancel(t *testing.T) {
	tl := NewTurnLoop()
	tl.EnterPlanning() // allow Preempt to succeed
	m := NewCancelManager(tl)

	if m.HasPendingCancel() {
		t.Fatalf("expected no pending cancel before Cancel")
	}

	h := m.Cancel(CancelImmediate)
	if h == nil {
		t.Fatalf("Cancel returned nil handle")
	}
	if !m.HasPendingCancel() {
		t.Fatalf("expected HasPendingCancel true after Cancel")
	}
	if !m.CheckCancel(CancelImmediate) {
		t.Errorf("expected CheckCancel(CancelImmediate) true")
	}
	// Immediate cancel must also match every safe-point (it fires anywhere).
	if !m.CheckCancel(CancelAfterChatModel) {
		t.Errorf("expected CheckCancel(CancelAfterChatModel) true for immediate request")
	}
	if !m.CheckCancel(CancelAfterToolCalls) {
		t.Errorf("expected CheckCancel(CancelAfterToolCalls) true for immediate request")
	}
}

// TestCancelManager_AfterChatModel verifies that a CancelAfterChatModel
// request fires at the after-chat-model safe-point but not at the
// after-tool-calls safe-point.
func TestCancelManager_AfterChatModel(t *testing.T) {
	tl := NewTurnLoop()
	m := NewCancelManager(tl)

	m.Cancel(CancelAfterChatModel)

	if !m.CheckCancel(CancelAfterChatModel) {
		t.Errorf("expected CheckCancel(CancelAfterChatModel) true")
	}
	if m.CheckCancel(CancelAfterToolCalls) {
		t.Errorf("expected CheckCancel(CancelAfterToolCalls) false")
	}
	// A safe-point request must not fire at the immediate checkpoint.
	if m.CheckCancel(CancelImmediate) {
		t.Errorf("expected CheckCancel(CancelImmediate) false for safe-point request")
	}
}

// TestCancelManager_AfterToolCalls verifies that a CancelAfterToolCalls
// request fires at the after-tool-calls safe-point but not at the
// after-chat-model safe-point.
func TestCancelManager_AfterToolCalls(t *testing.T) {
	tl := NewTurnLoop()
	m := NewCancelManager(tl)

	m.Cancel(CancelAfterToolCalls)

	if !m.CheckCancel(CancelAfterToolCalls) {
		t.Errorf("expected CheckCancel(CancelAfterToolCalls) true")
	}
	if m.CheckCancel(CancelAfterChatModel) {
		t.Errorf("expected CheckCancel(CancelAfterChatModel) false")
	}
	if m.CheckCancel(CancelImmediate) {
		t.Errorf("expected CheckCancel(CancelImmediate) false for safe-point request")
	}
}

// TestCancelManager_CompleteCancel verifies that CompleteCancel clears the
// active request and unblocks the handle's Wait.
func TestCancelManager_CompleteCancel(t *testing.T) {
	tl := NewTurnLoop()
	m := NewCancelManager(tl)

	h := m.Cancel(CancelAfterChatModel)
	if !m.HasPendingCancel() {
		t.Fatalf("expected pending cancel before CompleteCancel")
	}

	m.CompleteCancel(nil)

	if m.HasPendingCancel() {
		t.Fatalf("expected no pending cancel after CompleteCancel")
	}

	// Wait must unblock immediately and return nil (success).
	select {
	case err := <-func() chan error {
		c := make(chan error, 1)
		c <- h.Wait()
		return c
	}():
		if err != nil {
			t.Errorf("Wait returned %v after CompleteCancel(nil), want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Wait did not unblock after CompleteCancel")
	}
}

// TestCancelManager_TimeoutEscalation verifies that a safe-point cancel with a
// very short timeout escalates to CancelImmediate once the deadline elapses,
// and that CheckTimeoutEscalation returns true on escalation.
func TestCancelManager_TimeoutEscalation(t *testing.T) {
	tl := NewTurnLoop()
	tl.EnterPlanning() // so Preempt succeeds during escalation
	m := NewCancelManager(tl)

	m.Cancel(CancelAfterChatModel, WithCancelTimeout(1*time.Millisecond))

	// Before the deadline: no escalation.
	if m.CheckTimeoutEscalation() {
		t.Fatalf("CheckTimeoutEscalation returned true before deadline")
	}

	// Wait for the deadline to elapse.
	time.Sleep(20 * time.Millisecond)

	// After the deadline: escalation should occur.
	if !m.CheckTimeoutEscalation() {
		t.Fatalf("expected CheckTimeoutEscalation to return true after deadline")
	}

	// After escalation the mode is CancelImmediate, so every safe-point fires.
	if !m.CheckCancel(CancelImmediate) {
		t.Errorf("expected CheckCancel(CancelImmediate) true after escalation")
	}
	if !m.CheckCancel(CancelAfterChatModel) {
		t.Errorf("expected CheckCancel(CancelAfterChatModel) true after escalation")
	}

	// A second call must not escalate again (already immediate).
	if m.CheckTimeoutEscalation() {
		t.Errorf("CheckTimeoutEscalation should return false after already escalated")
	}

	// CompleteCancel(nil) must preserve the escalation error on the handle.
	h := func() *CancelHandle {
		// Recover the handle's request via the manager's active request.
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.activeReq == nil {
			return nil
		}
		return &CancelHandle{req: m.activeReq}
	}()
	m.CompleteCancel(nil)
	if h == nil {
		t.Fatalf("could not recover handle before CompleteCancel")
	}
	if err := h.Wait(); !errors.Is(err, ErrCancelTimeout) {
		t.Errorf("after escalation, Wait returned %v, want ErrCancelTimeout", err)
	}
}

// TestCancelManager_Recursive verifies that WithRecursive sets the recursive
// flag on the active cancel request.
func TestCancelManager_Recursive(t *testing.T) {
	tl := NewTurnLoop()
	m := NewCancelManager(tl)

	m.Cancel(CancelImmediate, WithRecursive())

	m.mu.Lock()
	req := m.activeReq
	m.mu.Unlock()

	if req == nil {
		t.Fatalf("expected active request after Cancel")
	}
	if !req.recursive {
		t.Errorf("expected recursive flag true, got false")
	}
}

// TestCancelManager_RecursiveDefault verifies that the recursive flag defaults
// to false when WithRecursive is not supplied.
func TestCancelManager_RecursiveDefault(t *testing.T) {
	tl := NewTurnLoop()
	m := NewCancelManager(tl)

	m.Cancel(CancelImmediate)

	m.mu.Lock()
	req := m.activeReq
	m.mu.Unlock()

	if req == nil {
		t.Fatalf("expected active request after Cancel")
	}
	if req.recursive {
		t.Errorf("expected recursive flag false by default, got true")
	}
}

// TestCancelOptions verifies that WithRecursive and WithCancelTimeout set the
// corresponding fields on a cancelConfig.
func TestCancelOptions(t *testing.T) {
	cfg := &cancelConfig{}

	WithRecursive()(cfg)
	if !cfg.recursive {
		t.Errorf("WithRecursive did not set recursive=true")
	}

	d := 5 * time.Second
	WithCancelTimeout(d)(cfg)
	if cfg.timeout != d {
		t.Errorf("WithCancelTimeout did not set timeout, got %v want %v", cfg.timeout, d)
	}

	// A fresh config with no options applied should have zero values.
	cfg2 := &cancelConfig{}
	if cfg2.recursive {
		t.Errorf("default cancelConfig.recursive should be false")
	}
	if cfg2.timeout != 0 {
		t.Errorf("default cancelConfig.timeout should be 0, got %v", cfg2.timeout)
	}
}

// TestCancelManager_NoTimeoutNoEscalation verifies that a safe-point cancel
// without a timeout never escalates.
func TestCancelManager_NoTimeoutNoEscalation(t *testing.T) {
	tl := NewTurnLoop()
	m := NewCancelManager(tl)

	m.Cancel(CancelAfterChatModel)
	time.Sleep(5 * time.Millisecond)

	if m.CheckTimeoutEscalation() {
		t.Errorf("CheckTimeoutEscalation returned true without a configured timeout")
	}
	if m.CheckCancel(CancelImmediate) {
		t.Errorf("CheckCancel(CancelImmediate) should be false without escalation")
	}
	if !m.CheckCancel(CancelAfterChatModel) {
		t.Errorf("CheckCancel(CancelAfterChatModel) should still be true")
	}
}

// TestCancelManager_Supersede verifies that a second Cancel call supersedes
// the first, unblocking the first handle's Wait with a nil outcome.
func TestCancelManager_Supersede(t *testing.T) {
	tl := NewTurnLoop()
	m := NewCancelManager(tl)

	h1 := m.Cancel(CancelAfterChatModel)
	h2 := m.Cancel(CancelAfterToolCalls)

	if !m.HasPendingCancel() {
		t.Fatalf("expected pending cancel after second Cancel")
	}
	// The active request must be the second one.
	if !m.CheckCancel(CancelAfterToolCalls) {
		t.Errorf("expected active request to be CancelAfterToolCalls after supersede")
	}
	if m.CheckCancel(CancelAfterChatModel) {
		t.Errorf("expected CheckCancel(CancelAfterChatModel) false after supersede")
	}

	// The first handle must unblock with a nil outcome (superseded).
	select {
	case err := <-func() chan error {
		c := make(chan error, 1)
		c <- h1.Wait()
		return c
	}():
		if err != nil {
			t.Errorf("superseded handle Wait returned %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("superseded handle Wait did not unblock")
	}

	// The second handle is still pending.
	if !m.HasPendingCancel() {
		t.Fatalf("second cancel should still be pending")
	}
	m.CompleteCancel(nil)
	_ = h2 // h2 unblocks via CompleteCancel
}

// TestCancelManager_NilTurnLoop verifies that a CancelManager with a nil
// TurnLoop still functions (Preempt is skipped).
func TestCancelManager_NilTurnLoop(t *testing.T) {
	m := NewCancelManager(nil)

	// Immediate cancel with nil TurnLoop must not panic.
	h := m.Cancel(CancelImmediate)
	if h == nil {
		t.Fatalf("Cancel returned nil handle")
	}
	if !m.HasPendingCancel() {
		t.Fatalf("expected pending cancel")
	}
	if !m.CheckCancel(CancelImmediate) {
		t.Errorf("expected CheckCancel(CancelImmediate) true")
	}
	m.CompleteCancel(nil)
	if m.HasPendingCancel() {
		t.Fatalf("expected no pending cancel after CompleteCancel")
	}
}
