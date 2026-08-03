package turnloop

import (
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 1. TurnPhase.String()
// ---------------------------------------------------------------------------

func TestTurnPhaseString(t *testing.T) {
	tests := []struct {
		phase TurnPhase
		want  string
	}{
		{TurnPhaseIdle, "idle"},
		{TurnPhasePlanning, "planning"},
		{TurnPhaseActive, "active"},
		{TurnPhase(42), "unknown(42)"},
	}
	for _, tt := range tests {
		got := tt.phase.String()
		if got != tt.want {
			t.Errorf("TurnPhase(%d).String() = %q; want %q", tt.phase, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// 2. NewTurnLoop 初始状态为 idle
// ---------------------------------------------------------------------------

func TestNewTurnLoopIdle(t *testing.T) {
	loop := NewTurnLoop()
	if loop.Phase() != TurnPhaseIdle {
		t.Errorf("NewTurnLoop().Phase() = %v; want %v", loop.Phase(), TurnPhaseIdle)
	}
}

// ---------------------------------------------------------------------------
// 3. Phase() 状态转换：idle→EnterPlanning→planning→EnterActive→active→EnterIdle→idle
// ---------------------------------------------------------------------------

func TestPhaseTransitions(t *testing.T) {
	loop := NewTurnLoop()

	// idle → planning
	ch1 := loop.EnterPlanning()
	if loop.Phase() != TurnPhasePlanning {
		t.Errorf("after EnterPlanning, Phase() = %v; want planning", loop.Phase())
	}
	if ch1 == nil {
		t.Error("EnterPlanning returned nil channel")
	}

	// planning → active
	ch2 := loop.EnterActive()
	if loop.Phase() != TurnPhaseActive {
		t.Errorf("after EnterActive, Phase() = %v; want active", loop.Phase())
	}
	if ch2 == nil {
		t.Error("EnterActive returned nil channel")
	}
	// preempt channels should be different
	if ch1 == ch2 {
		t.Error("EnterPlanning and EnterActive should return different channels")
	}

	// active → idle
	ch3 := loop.EnterIdle()
	if loop.Phase() != TurnPhaseIdle {
		t.Errorf("after EnterIdle, Phase() = %v; want idle", loop.Phase())
	}
	if ch3 != nil {
		t.Error("EnterIdle should return nil channel")
	}
}

// ---------------------------------------------------------------------------
// 4. Preempt：从 planning 或 active 调用后回到 idle、IsPreempted=true、PreemptReason 返回原因
// ---------------------------------------------------------------------------

func TestPreemptFromPlanning(t *testing.T) {
	loop := NewTurnLoop()
	loop.EnterPlanning()

	err := loop.Preempt("test reason: planning")
	if err != nil {
		t.Errorf("Preempt from planning: unexpected error %v", err)
	}
	if loop.Phase() != TurnPhaseIdle {
		t.Errorf("after Preempt, Phase() = %v; want idle", loop.Phase())
	}
	if !loop.IsPreempted() {
		t.Error("IsPreempted should be true after Preempt")
	}
	if loop.PreemptReason() != "test reason: planning" {
		t.Errorf("PreemptReason() = %q; want %q", loop.PreemptReason(), "test reason: planning")
	}
}

func TestPreemptFromActive(t *testing.T) {
	loop := NewTurnLoop()
	loop.EnterPlanning()
	loop.EnterActive()

	err := loop.Preempt("test reason: active")
	if err != nil {
		t.Errorf("Preempt from active: unexpected error %v", err)
	}
	if loop.Phase() != TurnPhaseIdle {
		t.Errorf("after Preempt, Phase() = %v; want idle", loop.Phase())
	}
	if !loop.IsPreempted() {
		t.Error("IsPreempted should be true after Preempt")
	}
	if loop.PreemptReason() != "test reason: active" {
		t.Errorf("PreemptReason() = %q; want %q", loop.PreemptReason(), "test reason: active")
	}
}

// ---------------------------------------------------------------------------
// 5. Preempt：从 idle 调用返回 ErrCannotPreemptIdle
// ---------------------------------------------------------------------------

func TestPreemptFromIdle(t *testing.T) {
	loop := NewTurnLoop()
	err := loop.Preempt("any reason")
	if err != ErrCannotPreemptIdle {
		t.Errorf("Preempt from idle: got error %v; want %v", err, ErrCannotPreemptIdle)
	}
	if loop.Phase() != TurnPhaseIdle {
		t.Errorf("Phase() after failed Preempt = %v; want idle", loop.Phase())
	}
}

// ---------------------------------------------------------------------------
// 6. Reset：清除 preempted 状态和 reason，回到 idle
// ---------------------------------------------------------------------------

func TestReset(t *testing.T) {
	loop := NewTurnLoop()

	// Enter a non-idle state and preempt
	loop.EnterPlanning()
	_ = loop.Preempt("reset me")
	if !loop.IsPreempted() {
		t.Fatal("IsPreempted should be true before Reset")
	}

	loop.Reset()
	if loop.Phase() != TurnPhaseIdle {
		t.Errorf("after Reset, Phase() = %v; want idle", loop.Phase())
	}
	if loop.IsPreempted() {
		t.Error("IsPreempted should be false after Reset")
	}
	if loop.PreemptReason() != "" {
		t.Errorf("PreemptReason() after Reset = %q; want empty", loop.PreemptReason())
	}

	// Reset on an already-idle loop should also be fine
	loop.Reset()
	if loop.Phase() != TurnPhaseIdle {
		t.Errorf("second Reset, Phase() = %v; want idle", loop.Phase())
	}
}

// ---------------------------------------------------------------------------
// 7. EnterIdle：关闭 preemptCh 并返回 nil
// ---------------------------------------------------------------------------

func TestEnterIdleClosesPreemptCh(t *testing.T) {
	loop := NewTurnLoop()
	ch := loop.EnterPlanning()

	// Verify ch is open before EnterIdle
	select {
	case <-ch:
		t.Fatal("preemptCh should not be closed before EnterIdle")
	default:
	}

	// EnterIdle should close the channel
	result := loop.EnterIdle()
	if result != nil {
		t.Error("EnterIdle should return nil")
	}

	// Channel should now be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("preemptCh should be closed after EnterIdle")
		}
	default:
		t.Error("preemptCh should be readable (closed) after EnterIdle")
	}

	// When phase is already idle, EnterIdle still returns nil and doesn't panic
	ch2 := loop.EnterIdle()
	if ch2 != nil {
		t.Error("second EnterIdle should return nil")
	}
}

// ---------------------------------------------------------------------------
// 8. Snapshot：正确捕获当前状态和传入的计数器值
// ---------------------------------------------------------------------------

func TestSnapshot(t *testing.T) {
	loop := NewTurnLoop()
	loop.EnterPlanning()
	_ = loop.Preempt("snapshot test")

	before := time.Now()
	snap := loop.Snapshot(3, 5, 7)
	after := time.Now()

	if snap.Phase != TurnPhaseIdle {
		t.Errorf("snapshot Phase = %v; want idle", snap.Phase)
	}
	if snap.Round != 3 {
		t.Errorf("snapshot Round = %d; want 3", snap.Round)
	}
	if snap.ToolCallRounds != 5 {
		t.Errorf("snapshot ToolCallRounds = %d; want 5", snap.ToolCallRounds)
	}
	if snap.MessageCount != 7 {
		t.Errorf("snapshot MessageCount = %d; want 7", snap.MessageCount)
	}
	if snap.PreemptReason != "snapshot test" {
		t.Errorf("snapshot PreemptReason = %q; want %q", snap.PreemptReason, "snapshot test")
	}
	if snap.Timestamp.Before(before) || snap.Timestamp.After(after) {
		t.Errorf("snapshot Timestamp %v not in [%v, %v]", snap.Timestamp, before, after)
	}

	// Snapshot on a fresh loop
	loop2 := NewTurnLoop()
	snap2 := loop2.Snapshot(0, 0, 0)
	if snap2.Phase != TurnPhaseIdle {
		t.Errorf("fresh loop snapshot Phase = %v; want idle", snap2.Phase)
	}
	if snap2.PreemptReason != "" {
		t.Errorf("fresh loop snapshot PreemptReason = %q; want empty", snap2.PreemptReason)
	}
}

// ---------------------------------------------------------------------------
// 9. 并发安全：goroutine 并发调用 Phase/Preempt 不 panic
// ---------------------------------------------------------------------------

func TestConcurrentSafety(t *testing.T) {
	loop := NewTurnLoop()
	var wg sync.WaitGroup

	// Spawn multiple goroutines that call Phase() continuously
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				loop.Phase()
			}
		}()
	}

	// Spawn goroutines that preempt from various states
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				loop.EnterPlanning()
				loop.EnterActive()
				_ = loop.Preempt("concurrent test")
				loop.Reset()
			}
		}(i)
	}

	// Spawn goroutines that read preempted/reason
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				loop.IsPreempted()
				loop.PreemptReason()
			}
		}()
	}

	wg.Wait()
	// Final state should be idle after all the Reset calls
	if loop.Phase() != TurnPhaseIdle {
		t.Errorf("after concurrent test, Phase() = %v; want idle", loop.Phase())
	}
}

// ---------------------------------------------------------------------------
// Preempt 关闭 channel 使 select 感知中断
// ---------------------------------------------------------------------------

func TestPreemptChannelUnblocks(t *testing.T) {
	loop := NewTurnLoop()
	ch := loop.EnterPlanning()

	done := make(chan struct{})
	go func() {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Error("timed out waiting for preempt channel to close")
		}
		close(done)
	}()

	// Give the goroutine time to enter the select
	time.Sleep(10 * time.Millisecond)
	_ = loop.Preempt("unblock")

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("goroutine did not unblock within 5s")
	}
}