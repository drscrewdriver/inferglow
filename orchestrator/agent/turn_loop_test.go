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
	"sync"
	"testing"
	"time"
)

// Test 1: A new TurnLoop starts in the Idle phase.
func TestTurnLoop_InitialPhase(t *testing.T) {
	l := NewTurnLoop()
	if got := l.Phase(); got != TurnPhaseIdle {
		t.Fatalf("expected initial phase idle, got %s", got)
	}
	if got := l.Phase().String(); got != "idle" {
		t.Fatalf("expected initial phase string \"idle\", got %q", got)
	}
}

// Test 2: Idle → Planning → Active → Idle, verifying each transition.
func TestTurnLoop_PhaseTransitions(t *testing.T) {
	l := NewTurnLoop()

	l.EnterPlanning()
	if got := l.Phase(); got != TurnPhasePlanning {
		t.Fatalf("after EnterPlanning: expected planning, got %s", got)
	}
	if got := l.Phase().String(); got != "planning" {
		t.Fatalf("after EnterPlanning: expected string \"planning\", got %q", got)
	}

	l.EnterActive()
	if got := l.Phase(); got != TurnPhaseActive {
		t.Fatalf("after EnterActive: expected active, got %s", got)
	}
	if got := l.Phase().String(); got != "active" {
		t.Fatalf("after EnterActive: expected string \"active\", got %q", got)
	}

	l.EnterIdle()
	if got := l.Phase(); got != TurnPhaseIdle {
		t.Fatalf("after EnterIdle: expected idle, got %s", got)
	}
	if got := l.Phase().String(); got != "idle" {
		t.Fatalf("after EnterIdle: expected string \"idle\", got %q", got)
	}
}

// Test 3: Preempt from planning transitions to idle and sets the preempted flag.
func TestTurnLoop_PreemptFromPlanning(t *testing.T) {
	l := NewTurnLoop()
	l.EnterPlanning()

	if err := l.Preempt("user interrupt"); err != nil {
		t.Fatalf("Preempt from planning returned unexpected error: %v", err)
	}
	if got := l.Phase(); got != TurnPhaseIdle {
		t.Fatalf("after Preempt: expected idle, got %s", got)
	}
	if !l.IsPreempted() {
		t.Fatalf("expected IsPreempted true, got false")
	}
}

// Test 4: Preempt from active transitions to idle.
func TestTurnLoop_PreemptFromActive(t *testing.T) {
	l := NewTurnLoop()
	l.EnterPlanning()
	l.EnterActive()

	if err := l.Preempt("user interrupt"); err != nil {
		t.Fatalf("Preempt from active returned unexpected error: %v", err)
	}
	if got := l.Phase(); got != TurnPhaseIdle {
		t.Fatalf("after Preempt: expected idle, got %s", got)
	}
	if !l.IsPreempted() {
		t.Fatalf("expected IsPreempted true, got false")
	}
}

// Test 5: Preempt when idle returns an error and does not set the preempted flag.
func TestTurnLoop_PreemptFromIdle(t *testing.T) {
	l := NewTurnLoop()

	err := l.Preempt("user interrupt")
	if err == nil {
		t.Fatalf("expected error when preempting idle, got nil")
	}
	if !errors.Is(err, ErrCannotPreemptIdle) {
		t.Fatalf("expected ErrCannotPreemptIdle, got %v", err)
	}
	if l.IsPreempted() {
		t.Fatalf("expected IsPreempted false, got true")
	}
	if got := l.Phase(); got != TurnPhaseIdle {
		t.Fatalf("expected phase to remain idle, got %s", got)
	}
}

// Test 6: Preempt records the reason, retrievable via PreemptReason.
func TestTurnLoop_PreemptReason(t *testing.T) {
	l := NewTurnLoop()
	l.EnterPlanning()

	reason := "user pressed stop"
	if got := l.PreemptReason(); got != "" {
		t.Fatalf("before preempt: expected empty reason, got %q", got)
	}
	if err := l.Preempt(reason); err != nil {
		t.Fatalf("Preempt returned unexpected error: %v", err)
	}
	if got := l.PreemptReason(); got != reason {
		t.Fatalf("after preempt: expected reason %q, got %q", reason, got)
	}
}

// Test 7: Reset clears the preempted state and reason and returns to idle.
func TestTurnLoop_Reset(t *testing.T) {
	l := NewTurnLoop()
	l.EnterPlanning()
	if err := l.Preempt("interrupt"); err != nil {
		t.Fatalf("Preempt returned unexpected error: %v", err)
	}
	if !l.IsPreempted() {
		t.Fatalf("expected preempted before Reset")
	}

	l.Reset()

	if l.IsPreempted() {
		t.Fatalf("after Reset: expected IsPreempted false, got true")
	}
	if got := l.PreemptReason(); got != "" {
		t.Fatalf("after Reset: expected empty reason, got %q", got)
	}
	if got := l.Phase(); got != TurnPhaseIdle {
		t.Fatalf("after Reset: expected idle, got %s", got)
	}
}

// Test 8: The preempt channel returned by EnterPlanning is closed when Preempt
// is called from another goroutine, unblocking a select on it.
func TestTurnLoop_PreemptChannelSignal(t *testing.T) {
	l := NewTurnLoop()
	ch := l.EnterPlanning()
	if ch == nil {
		t.Fatalf("EnterPlanning returned nil channel")
	}

	go func() {
		l.Preempt("async interrupt")
	}()

	select {
	case <-ch:
		// Channel was closed by Preempt — expected.
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for preempt channel to close")
	}

	if !l.IsPreempted() {
		t.Fatalf("expected IsPreempted true after channel closed")
	}
}

// Test 9: Concurrent EnterPlanning/EnterActive/Preempt calls must not race or
// panic. Run with -race to verify.
func TestTurnLoop_ConcurrentAccess(t *testing.T) {
	l := NewTurnLoop()

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			l.EnterPlanning()
		}()
		go func() {
			defer wg.Done()
			l.EnterActive()
		}()
		go func() {
			defer wg.Done()
			// Preempt may return ErrCannotPreemptIdle if the loop is idle at
			// the moment of the call; that is expected and safe to ignore.
			_ = l.Preempt("concurrent")
		}()
	}
	wg.Wait()

	// After all goroutines complete, the loop must be in a valid phase with no
	// corruption or panic.
	phase := l.Phase()
	switch phase {
	case TurnPhaseIdle, TurnPhasePlanning, TurnPhaseActive:
		// valid
	default:
		t.Fatalf("invalid phase after concurrent access: %s", phase)
	}
}
