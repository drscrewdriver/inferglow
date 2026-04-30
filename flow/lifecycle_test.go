package flow

import (
	"strings"
	"testing"
)

// ============================================================================
// LifecycleState 常量测试
// ============================================================================

func TestLifecycleStateConstants(t *testing.T) {
	cases := []struct {
		state LifecycleState
		want  string
	}{
		{LifecycleOpen, "open"},
		{LifecycleRunning, "running"},
		{LifecycleWaiting, "waiting"},
		{LifecycleFailed, "failed"},
		{LifecycleSealed, "sealed"},
		{LifecycleClosed, "closed"},
	}
	if len(cases) != 6 {
		t.Fatalf("expected 6 lifecycle states, got %d", len(cases))
	}
	for _, c := range cases {
		if string(c.state) != c.want {
			t.Errorf("state = %q, want %q", c.state, c.want)
		}
	}
}

// ============================================================================
// NewLifecycleMachine 测试
// ============================================================================

func TestNewLifecycleMachine(t *testing.T) {
	m := NewLifecycleMachine()
	if m.Current() != LifecycleOpen {
		t.Errorf("initial state = %q, want open", m.Current())
	}
	h := m.History()
	if len(h) != 1 || h[0] != LifecycleOpen {
		t.Errorf("initial history = %v, want [open]", h)
	}
}

// ============================================================================
// 正常执行流程：open -> running -> closed
// ============================================================================

func TestLifecycleNormalFlow(t *testing.T) {
	m := NewLifecycleMachine()
	if err := m.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if m.Current() != LifecycleRunning {
		t.Errorf("after Start, state = %q, want running", m.Current())
	}
	if err := m.Close("done"); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if m.Current() != LifecycleClosed {
		t.Errorf("after Close, state = %q, want closed", m.Current())
	}
	h := m.History()
	if len(h) != 3 {
		t.Fatalf("history len = %d, want 3", len(h))
	}
	wantSeq := []LifecycleState{LifecycleOpen, LifecycleRunning, LifecycleClosed}
	for i, want := range wantSeq {
		if h[i] != want {
			t.Errorf("history[%d] = %q, want %q", i, h[i], want)
		}
	}
}

// ============================================================================
// 异常终止：running -> failed -> closed
// ============================================================================

func TestLifecycleFailFromRunning(t *testing.T) {
	m := NewLifecycleMachine()
	_ = m.Start()
	if err := m.Fail("execution error"); err != nil {
		t.Fatalf("Fail failed: %v", err)
	}
	if m.Current() != LifecycleFailed {
		t.Errorf("after Fail, state = %q, want failed", m.Current())
	}
	if m.GetError() != "execution error" {
		t.Errorf("GetError = %q, want 'execution error'", m.GetError())
	}
	if err := m.Close("closing after fail"); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if m.Current() != LifecycleClosed {
		t.Errorf("after Close, state = %q, want closed", m.Current())
	}
}

// ============================================================================
// 干预暂停与恢复：running -> waiting -> running -> closed
// ============================================================================

func TestLifecycleInterventionResume(t *testing.T) {
	m := NewLifecycleMachine()
	_ = m.Start()
	if err := m.Wait(); err != nil {
		t.Fatalf("Wait failed: %v", err)
	}
	if m.Current() != LifecycleWaiting {
		t.Errorf("after Wait, state = %q, want waiting", m.Current())
	}
	if err := m.Resume(); err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if m.Current() != LifecycleRunning {
		t.Errorf("after Resume, state = %q, want running", m.Current())
	}
	_ = m.Close("completed")
	if m.Current() != LifecycleClosed {
		t.Errorf("final state = %q, want closed", m.Current())
	}
}

// ============================================================================
// 手动封闭：running -> sealed -> closed
// ============================================================================

func TestLifecycleSealClose(t *testing.T) {
	m := NewLifecycleMachine()
	_ = m.Start()
	if err := m.Seal(); err != nil {
		t.Fatalf("Seal failed: %v", err)
	}
	if m.Current() != LifecycleSealed {
		t.Errorf("after Seal, state = %q, want sealed", m.Current())
	}
	if err := m.Close("sealed complete"); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if m.Current() != LifecycleClosed {
		t.Errorf("final state = %q, want closed", m.Current())
	}
	// sealed -> closed 时不应覆盖 errorInfo（除非 reason 非空）
	// 这里 reason="sealed complete" 但 from=sealed，所以不写入 errorInfo
	if m.GetError() != "" {
		t.Errorf("GetError = %q, want empty (sealed path doesn't set error)", m.GetError())
	}
}

// ============================================================================
// 非法转换测试
// ============================================================================

func TestLifecycleInvalidTransitionOpenToWaiting(t *testing.T) {
	m := NewLifecycleMachine()
	// open -> waiting 不合法
	err := m.Transition(LifecycleOpen, LifecycleWaiting)
	if err == nil {
		t.Fatal("expected error for open -> waiting")
	}
	if !strings.Contains(err.Error(), "invalid transition") {
		t.Errorf("error should mention 'invalid transition', got: %v", err)
	}
}

func TestLifecycleInvalidTransitionClosedToRunning(t *testing.T) {
	m := NewLifecycleMachine()
	_ = m.Start()
	_ = m.Close("done")
	// closed -> running 不合法
	err := m.Transition(LifecycleClosed, LifecycleRunning)
	if err == nil {
		t.Fatal("expected error for closed -> running")
	}
}

func TestLifecycleInvalidTransitionWrongCurrent(t *testing.T) {
	m := NewLifecycleMachine()
	// 当前是 open，但声明 from=running
	err := m.Transition(LifecycleRunning, LifecycleClosed)
	if err == nil {
		t.Fatal("expected error for wrong current state")
	}
	if !strings.Contains(err.Error(), "current state is open") {
		t.Errorf("error should mention 'current state is open', got: %v", err)
	}
}

// ============================================================================
// Start/Wait/Resume/Fail 边界条件
// ============================================================================

func TestLifecycleStartFromNonOpen(t *testing.T) {
	m := NewLifecycleMachine()
	_ = m.Start()
	// 已经 running，再次 Start 应失败
	err := m.Start()
	if err == nil {
		t.Fatal("expected error for Start from non-open state")
	}
}

func TestLifecycleWaitFromNonRunning(t *testing.T) {
	m := NewLifecycleMachine()
	// open 状态不能 Wait
	err := m.Wait()
	if err == nil {
		t.Fatal("expected error for Wait from open state")
	}
}

func TestLifecycleResumeFromNonWaiting(t *testing.T) {
	m := NewLifecycleMachine()
	_ = m.Start()
	// running 状态不能 Resume
	err := m.Resume()
	if err == nil {
		t.Fatal("expected error for Resume from running state")
	}
}

func TestLifecycleFailFromOpen(t *testing.T) {
	m := NewLifecycleMachine()
	err := m.Fail("cannot fail from open")
	if err == nil {
		t.Fatal("expected error for Fail from open state")
	}
}

func TestLifecycleSealFromNonRunning(t *testing.T) {
	m := NewLifecycleMachine()
	err := m.Seal()
	if err == nil {
		t.Fatal("expected error for Seal from open state")
	}
}

func TestLifecycleCloseFromOpen(t *testing.T) {
	m := NewLifecycleMachine()
	// open -> closed 是合法的，所以应该成功
	if err := m.Close("aborted"); err != nil {
		t.Fatalf("Close from open should succeed: %v", err)
	}
	if m.Current() != LifecycleClosed {
		t.Errorf("state = %q, want closed", m.Current())
	}
}

func TestLifecycleCloseFromWaiting(t *testing.T) {
	m := NewLifecycleMachine()
	_ = m.Start()
	_ = m.Wait()
	if err := m.Close("cancelled while waiting"); err != nil {
		t.Fatalf("Close from waiting failed: %v", err)
	}
	if m.Current() != LifecycleClosed {
		t.Errorf("state = %q, want closed", m.Current())
	}
}

func TestLifecycleCloseFromClosed(t *testing.T) {
	m := NewLifecycleMachine()
	_ = m.Start()
	_ = m.Close("done")
	err := m.Close("again")
	if err == nil {
		t.Fatal("expected error for Close from closed state")
	}
}

// ============================================================================
// IsTerminal / CanTransition
// ============================================================================

func TestLifecycleIsTerminal(t *testing.T) {
	m := NewLifecycleMachine()
	if m.IsTerminal() {
		t.Error("open is not terminal")
	}
	_ = m.Start()
	if m.IsTerminal() {
		t.Error("running is not terminal")
	}
	_ = m.Close("done")
	if !m.IsTerminal() {
		t.Error("closed should be terminal")
	}
}

func TestLifecycleCanTransition(t *testing.T) {
	m := NewLifecycleMachine()
	if !m.CanTransition(LifecycleRunning) {
		t.Error("open should be able to transition to running")
	}
	if m.CanTransition(LifecycleWaiting) {
		t.Error("open should NOT be able to transition to waiting")
	}
	_ = m.Start()
	if !m.CanTransition(LifecycleWaiting) {
		t.Error("running should be able to transition to waiting")
	}
	if m.CanTransition(LifecycleOpen) {
		t.Error("running should NOT be able to transition back to open")
	}
}

// ============================================================================
// SetError/GetError 不影响状态
// ============================================================================

func TestLifecycleSetGetError(t *testing.T) {
	m := NewLifecycleMachine()
	if m.GetError() != "" {
		t.Errorf("initial error should be empty, got %q", m.GetError())
	}
	m.SetError("warning")
	if m.GetError() != "warning" {
		t.Errorf("GetError = %q, want 'warning'", m.GetError())
	}
	if m.Current() != LifecycleOpen {
		t.Errorf("SetError should not change state, got %q", m.Current())
	}
}

// ============================================================================
// History 返回副本（修改不影响内部状态）
// ============================================================================

func TestLifecycleHistoryCopy(t *testing.T) {
	m := NewLifecycleMachine()
	_ = m.Start()
	h1 := m.History()
	h1[0] = LifecycleClosed
	h2 := m.History()
	if h2[0] != LifecycleOpen {
		t.Errorf("modifying returned history should not affect internal state, h2[0] = %q", h2[0])
	}
}

// ============================================================================
// nil 安全
// ============================================================================

func TestLifecycleMachineNilSafe(t *testing.T) {
	var m *LifecycleMachine
	if m.Current() != "" {
		t.Errorf("nil Current should return empty string")
	}
	if m.IsTerminal() {
		t.Error("nil IsTerminal should return false")
	}
	if m.CanTransition(LifecycleRunning) {
		t.Error("nil CanTransition should return false")
	}
	if m.GetError() != "" {
		t.Error("nil GetError should return empty string")
	}
	if err := m.Start(); err == nil {
		t.Error("nil Start should return error")
	}
	if err := m.Close("x"); err == nil {
		t.Error("nil Close should return error")
	}
	m.SetError("no-op") // 不应 panic
}

// ============================================================================
// 完整流程综合测试
// ============================================================================

func TestLifecycleFullFlowWithIntervention(t *testing.T) {
	m := NewLifecycleMachine()
	// 启动
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// 第一次干预
	if err := m.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	// 恢复
	if err := m.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	// 第二次干预
	if err := m.Wait(); err != nil {
		t.Fatalf("Wait 2: %v", err)
	}
	// 恢复
	if err := m.Resume(); err != nil {
		t.Fatalf("Resume 2: %v", err)
	}
	// 失败
	if err := m.Fail("transient error"); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	// 关闭
	if err := m.Close("final"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !m.IsTerminal() {
		t.Error("should be terminal")
	}
	h := m.History()
	wantSeq := []LifecycleState{
		LifecycleOpen,
		LifecycleRunning,
		LifecycleWaiting,
		LifecycleRunning,
		LifecycleWaiting,
		LifecycleRunning,
		LifecycleFailed,
		LifecycleClosed,
	}
	if len(h) != len(wantSeq) {
		t.Fatalf("history len = %d, want %d", len(h), len(wantSeq))
	}
	for i, want := range wantSeq {
		if h[i] != want {
			t.Errorf("history[%d] = %q, want %q", i, h[i], want)
		}
	}
}

// ============================================================================
// isValidTransition 单元测试
// ============================================================================

func TestIsValidTransitionMatrix(t *testing.T) {
	valid := []struct {
		from, to LifecycleState
	}{
		{LifecycleOpen, LifecycleRunning},
		{LifecycleOpen, LifecycleClosed},
		{LifecycleRunning, LifecycleWaiting},
		{LifecycleRunning, LifecycleFailed},
		{LifecycleRunning, LifecycleSealed},
		{LifecycleRunning, LifecycleClosed},
		{LifecycleWaiting, LifecycleRunning},
		{LifecycleWaiting, LifecycleFailed},
		{LifecycleWaiting, LifecycleClosed},
		{LifecycleSealed, LifecycleClosed},
		{LifecycleFailed, LifecycleClosed},
	}
	for _, c := range valid {
		if !isValidTransition(c.from, c.to) {
			t.Errorf("isValidTransition(%s, %s) = false, want true", c.from, c.to)
		}
	}

	invalid := []struct {
		from, to LifecycleState
	}{
		{LifecycleOpen, LifecycleWaiting},
		{LifecycleOpen, LifecycleFailed},
		{LifecycleOpen, LifecycleSealed},
		{LifecycleRunning, LifecycleOpen},
		{LifecycleWaiting, LifecycleOpen},
		{LifecycleWaiting, LifecycleSealed},
		{LifecycleSealed, LifecycleRunning},
		{LifecycleSealed, LifecycleFailed},
		{LifecycleFailed, LifecycleRunning},
		{LifecycleFailed, LifecycleSealed},
		{LifecycleClosed, LifecycleOpen},
		{LifecycleClosed, LifecycleRunning},
		{LifecycleClosed, LifecycleClosed},
	}
	for _, c := range invalid {
		if isValidTransition(c.from, c.to) {
			t.Errorf("isValidTransition(%s, %s) = true, want false", c.from, c.to)
		}
	}
}
