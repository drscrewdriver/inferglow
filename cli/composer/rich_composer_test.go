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

package composer

import (
	"testing"
	"time"
)

// baseTime 返回合成基准时间 t0，所有事件的时间戳基于它加精确间隔构造。
func baseTime() time.Time {
	return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
}

func sameActions(a, b []Action) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// 场景 1 + 8：突发粘贴多行；mode 依次覆盖 ModePasteHold→ModePasteBurst→ModeEnterSuppress。
func TestBurstPasteMultiLine(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	if got := c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base}); got != nil {
		t.Fatalf("'a': want no action, got %v", got)
	}
	if c.Mode() != ModePasteHold {
		t.Fatalf("mode after 'a': want ModePasteHold, got %v", c.Mode())
	}

	if got := c.Feed(Event{Kind: EventPlainChar, Text: "b", Now: base.Add(1 * time.Millisecond)}); got != nil {
		t.Fatalf("'b': want no action, got %v", got)
	}
	if c.Mode() != ModePasteBurst {
		t.Fatalf("mode after 'b': want ModePasteBurst, got %v", c.Mode())
	}

	// 突发期间绝无 ActionSubmit / ActionInsertNewline，全部返回 nil。
	for _, e := range []Event{
		{Kind: EventPlainChar, Text: "?", Now: base.Add(2 * time.Millisecond)},
		{Kind: EventEnter, Now: base.Add(3 * time.Millisecond)},
		{Kind: EventPlainChar, Text: "c", Now: base.Add(4 * time.Millisecond)},
	} {
		if got := c.Feed(e); got != nil {
			t.Fatalf("burst feed %+v: want no action, got %v", e, got)
		}
	}

	// tick 在 t0+14ms（距 lastCharAt 10ms）→ 恰一次 Paste("ab?\nc")。
	got := c.Feed(Event{Kind: EventTick, Now: base.Add(14 * time.Millisecond)})
	if len(got) != 1 || got[0].Kind != ActionPaste || got[0].Text != "ab?\nc" {
		t.Fatalf("tick flush: want single Paste(%q), got %v", "ab?\nc", got)
	}
	if c.Mode() != ModeEnterSuppress {
		t.Fatalf("mode after tick: want ModeEnterSuppress, got %v", c.Mode())
	}
	if c.Pending() != "" {
		t.Fatalf("buffer should be empty after flush, got %q", c.Pending())
	}
}

// 场景 2：慢速键入。
func TestSlowTyping(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	if got := c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base}); got != nil {
		t.Fatalf("'a': want no action, got %v", got)
	}
	acts := c.Feed(Event{Kind: EventPlainChar, Text: "b", Now: base.Add(20 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionTyped, "a"}}) {
		t.Fatalf("'b': want Typed(a), got %v", acts)
	}
	if c.Mode() != ModeTyping {
		t.Fatalf("mode after 'b': want ModeTyping, got %v", c.Mode())
	}

	acts = c.Feed(Event{Kind: EventPlainChar, Text: "c", Now: base.Add(40 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionTyped, "b"}}) {
		t.Fatalf("'c': want Typed(b), got %v", acts)
	}

	acts = c.Feed(Event{Kind: EventTick, Now: base.Add(50 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionTyped, "c"}}) {
		t.Fatalf("tick: want Typed(c), got %v", acts)
	}
	if c.Mode() != ModeIdle {
		t.Fatalf("mode after tick: want ModeIdle, got %v", c.Mode())
	}
}

// 场景 3：IME no_hold 立即 Typed，不进入突发，后续字符不被误并。
func TestIMENoHold(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	acts := c.Feed(Event{Kind: EventPlainCharNoHold, Text: "中", Now: base})
	if !sameActions(acts, []Action{{ActionTyped, "中"}}) {
		t.Fatalf("no-hold '中': want Typed(中), got %v", acts)
	}
	if c.Mode() != ModeIdle {
		t.Fatalf("mode after '中': want ModeIdle, got %v", c.Mode())
	}
	if c.Pending() != "" {
		t.Fatalf("pending after '中': want empty, got %q", c.Pending())
	}

	// 后续字符正常键入：'a' 进入 hold（防闪烁），快速 'b' 触发 Typed('a') 而非 Paste("中a")。
	if got := c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base.Add(1 * time.Millisecond)}); got != nil {
		t.Fatalf("'a': want no action, got %v", got)
	}
	if c.Mode() != ModePasteHold {
		t.Fatalf("mode after 'a': want ModePasteHold, got %v", c.Mode())
	}
	acts = c.Feed(Event{Kind: EventPlainChar, Text: "b", Now: base.Add(21 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionTyped, "a"}}) {
		t.Fatalf("'b': want Typed(a), got %v", acts)
	}
}

// NoHold 在已有暂存字符时，先冲刷 Typed(pending) 再 Typed(IME)。
func TestIMENoHoldFlushesPending(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	if got := c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base}); got != nil {
		t.Fatalf("'a': want no action, got %v", got)
	}
	acts := c.Feed(Event{Kind: EventPlainCharNoHold, Text: "中", Now: base.Add(1 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionTyped, "a"}, {ActionTyped, "中"}}) {
		t.Fatalf("no-hold flush: want [Typed(a) Typed(中)], got %v", acts)
	}
	if c.Mode() != ModeIdle {
		t.Fatalf("mode: want ModeIdle, got %v", c.Mode())
	}
}

// NoHold 在突发进行中到达时：立即 Typed，且不打断突发。
func TestIMENoHoldDuringBurst(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base})
	c.Feed(Event{Kind: EventPlainChar, Text: "b", Now: base.Add(1 * time.Millisecond)})
	if c.Mode() != ModePasteBurst {
		t.Fatalf("want ModePasteBurst, got %v", c.Mode())
	}

	acts := c.Feed(Event{Kind: EventPlainCharNoHold, Text: "中", Now: base.Add(2 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionTyped, "中"}}) {
		t.Fatalf("want Typed(中), got %v", acts)
	}
	if c.Mode() != ModePasteBurst {
		t.Fatalf("burst must survive IME, got %v", c.Mode())
	}

	acts = c.Feed(Event{Kind: EventTick, Now: base.Add(12 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionPaste, "ab"}}) {
		t.Fatalf("tick: want Paste(ab), got %v", acts)
	}
}

// NoHold 携带多 rune（IME 组合/成块文本）时一次 Typed 输出全部 rune。
func TestIMENoHoldMultiRune(t *testing.T) {
	c := New(Config{})
	acts := c.Feed(Event{Kind: EventPlainCharNoHold, Text: "中文", Now: baseTime()})
	if !sameActions(acts, []Action{{ActionTyped, "中文"}}) {
		t.Fatalf("no-hold multi-rune: want Typed(中文), got %v", acts)
	}
	if c.Mode() != ModeIdle {
		t.Fatalf("mode: want ModeIdle, got %v", c.Mode())
	}
}

// Enter 带暂存字符时先冲刷为 Typed 再 Submit（防暂存字符丢失）。
func TestEnterFlushesPendingChar(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	if got := c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base}); got != nil {
		t.Fatalf("'a': want no action, got %v", got)
	}
	if c.Mode() != ModePasteHold {
		t.Fatalf("mode: want ModePasteHold, got %v", c.Mode())
	}
	acts := c.Feed(Event{Kind: EventEnter, Now: base.Add(100 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionTyped, "a"}, {Kind: ActionSubmit}}) {
		t.Fatalf("enter with pending: want [Typed(a) Submit], got %v", acts)
	}
	if c.Mode() != ModeIdle {
		t.Fatalf("mode: want ModeIdle, got %v", c.Mode())
	}
	if c.Pending() != "" {
		t.Fatalf("pending: want empty, got %q", c.Pending())
	}
}

// 场景 4：突发后 Enter 抑制窗口。
func TestEnterSuppressWindow(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base})
	c.Feed(Event{Kind: EventPlainChar, Text: "b", Now: base.Add(1 * time.Millisecond)})
	flush := c.Feed(Event{Kind: EventTick, Now: base.Add(11 * time.Millisecond)})
	if !sameActions(flush, []Action{{ActionPaste, "ab"}}) {
		t.Fatalf("flush: want Paste(ab), got %v", flush)
	}
	if c.Mode() != ModeEnterSuppress {
		t.Fatalf("mode after flush: want ModeEnterSuppress, got %v", c.Mode())
	}

	// 5ms 内 Enter → InsertNewline（不 Submit）。
	acts := c.Feed(Event{Kind: EventEnter, Now: base.Add(12 * time.Millisecond)})
	if !sameActions(acts, []Action{{Kind: ActionInsertNewline}}) {
		t.Fatalf("enter in window: want InsertNewline, got %v", acts)
	}

	// 越过窗口：tick 结束抑制窗口。
	if acts := c.Feed(Event{Kind: EventTick, Now: base.Add(22 * time.Millisecond)}); acts != nil {
		t.Fatalf("tick past window: want no action, got %v", acts)
	}
	if c.Mode() != ModeIdle {
		t.Fatalf("mode after window: want ModeIdle, got %v", c.Mode())
	}

	// 窗口外 Enter → Submit。
	acts = c.Feed(Event{Kind: EventEnter, Now: base.Add(22 * time.Millisecond)})
	if !sameActions(acts, []Action{{Kind: ActionSubmit}}) {
		t.Fatalf("enter past window: want Submit, got %v", acts)
	}
}

// 场景 5：modified_input 冲刷突发缓冲为 Paste，并回到 idle。
func TestModifiedInputFlush(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base})
	c.Feed(Event{Kind: EventPlainChar, Text: "b", Now: base.Add(1 * time.Millisecond)})
	if got := c.Pending(); got != "ab" {
		t.Fatalf("pending: want %q, got %q", "ab", got)
	}

	acts := c.Feed(Event{Kind: EventModifiedInput, Now: base.Add(2 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionPaste, "ab"}}) {
		t.Fatalf("modified: want Paste(ab), got %v", acts)
	}
	if c.Mode() != ModeIdle {
		t.Fatalf("mode: want ModeIdle, got %v", c.Mode())
	}
	if c.Pending() != "" {
		t.Fatalf("pending: want empty, got %q", c.Pending())
	}
}

// ModifiedInput 冲刷暂存字符（非突发态）。
func TestModifiedInputFlushPending(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base}) // hold, pending='a'
	acts := c.Feed(Event{Kind: EventModifiedInput, Now: base.Add(5 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionTyped, "a"}}) {
		t.Fatalf("modified: want Typed(a), got %v", acts)
	}
	if c.Mode() != ModeIdle {
		t.Fatalf("mode: want ModeIdle, got %v", c.Mode())
	}
	if got := c.Pending(); got != "" {
		t.Fatalf("pending: want empty, got %q", got)
	}
}

// ModifiedInput 无任何状态时返回空动作。
func TestModifiedInputNoState(t *testing.T) {
	c := New(Config{})
	acts := c.Feed(Event{Kind: EventModifiedInput, Now: baseTime()})
	if len(acts) != 0 {
		t.Fatalf("modified with no state: want empty, got %v", acts)
	}
}

// 场景 6：bracketed_paste 整体作为 Paste。
func TestBracketedPaste(t *testing.T) {
	c := New(Config{})
	acts := c.Feed(Event{Kind: EventBracketedPaste, Text: "x\n", Now: baseTime()})
	if !sameActions(acts, []Action{{ActionPaste, "x\n"}}) {
		t.Fatalf("bracketed: want Paste(x\\n), got %v", acts)
	}
	if c.Mode() != ModeIdle {
		t.Fatalf("mode: want ModeIdle, got %v", c.Mode())
	}
}

// BracketedPaste 会整体清空进行中的突发/暂存态。
func TestBracketedPasteClearsState(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base})
	c.Feed(Event{Kind: EventPlainChar, Text: "b", Now: base.Add(1 * time.Millisecond)}) // burst
	acts := c.Feed(Event{Kind: EventBracketedPaste, Text: "z", Now: base.Add(2 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionPaste, "z"}}) {
		t.Fatalf("bracketed: want Paste(z), got %v", acts)
	}
	if c.Mode() != ModeIdle {
		t.Fatalf("mode: want ModeIdle, got %v", c.Mode())
	}
	if c.Pending() != "" {
		t.Fatalf("pending: want empty, got %q", c.Pending())
	}
}

// 场景 7：确定性快照——同一事件序列喂两次，动作序列完全一致。
func TestDeterministicSnapshot(t *testing.T) {
	seq := []Event{
		{Kind: EventPlainChar, Text: "a", Now: baseTime()},
		{Kind: EventPlainChar, Text: "b", Now: baseTime().Add(1 * time.Millisecond)},
		{Kind: EventPlainChar, Text: "?", Now: baseTime().Add(2 * time.Millisecond)},
		{Kind: EventEnter, Now: baseTime().Add(3 * time.Millisecond)},
		{Kind: EventPlainChar, Text: "c", Now: baseTime().Add(4 * time.Millisecond)},
		{Kind: EventTick, Now: baseTime().Add(14 * time.Millisecond)},
		{Kind: EventEnter, Now: baseTime().Add(15 * time.Millisecond)},
		{Kind: EventEnter, Now: baseTime().Add(30 * time.Millisecond)},
	}
	run := func() []Action {
		c := New(Config{})
		var out []Action
		for _, e := range seq {
			out = append(out, c.Feed(e)...)
		}
		return out
	}

	first, second := run(), run()
	if len(first) != len(second) {
		t.Fatalf("deterministic: length mismatch %v vs %v", first, second)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("deterministic: mismatch at %d: %v vs %v", i, first[i], second[i])
		}
	}
	want := []Action{
		{ActionPaste, "ab?\nc"},
		{Kind: ActionInsertNewline},
		{Kind: ActionSubmit},
	}
	if !sameActions(first, want) {
		t.Fatalf("snapshot: want %v, got %v", want, first)
	}
}

// 边界：tick 在 ModePasteHold 下冲刷为 Typed。
func TestTickFlushPasteHold(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base}) // hold
	if c.Mode() != ModePasteHold {
		t.Fatalf("mode: want ModePasteHold, got %v", c.Mode())
	}
	acts := c.Feed(Event{Kind: EventTick, Now: base.Add(10 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionTyped, "a"}}) {
		t.Fatalf("tick: want Typed(a), got %v", acts)
	}
	if c.Mode() != ModeIdle {
		t.Fatalf("mode: want ModeIdle, got %v", c.Mode())
	}
}

// 边界：typing→burst 回退，以及突发中慢速字符冲刷 Paste。
func TestTypingToBurstFallback(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base})
	acts := c.Feed(Event{Kind: EventPlainChar, Text: "b", Now: base.Add(20 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionTyped, "a"}}) {
		t.Fatalf("'b': want Typed(a), got %v", acts)
	}
	if c.Mode() != ModeTyping {
		t.Fatalf("mode: want ModeTyping, got %v", c.Mode())
	}

	// 快速 'c'（间隔 1ms）→ 回退为突发 [b,c]，无动作。
	if got := c.Feed(Event{Kind: EventPlainChar, Text: "c", Now: base.Add(21 * time.Millisecond)}); got != nil {
		t.Fatalf("'c': want no action, got %v", got)
	}
	if c.Mode() != ModePasteBurst {
		t.Fatalf("mode: want ModePasteBurst, got %v", c.Mode())
	}

	// 快速 'd' → append。
	if got := c.Feed(Event{Kind: EventPlainChar, Text: "d", Now: base.Add(22 * time.Millisecond)}); got != nil {
		t.Fatalf("'d': want no action, got %v", got)
	}

	// 慢速 'e'（间隔 28ms > 2ms）→ 冲刷 Paste("bcd")，模式回 Typing。
	acts = c.Feed(Event{Kind: EventPlainChar, Text: "e", Now: base.Add(50 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionPaste, "bcd"}}) {
		t.Fatalf("'e': want Paste(bcd), got %v", acts)
	}
	if c.Mode() != ModeTyping {
		t.Fatalf("mode: want ModeTyping, got %v", c.Mode())
	}
}

// 边界：ModeEnterSuppress 下普通字符进入 PasteHold。
func TestPlainCharInEnterSuppress(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base})
	c.Feed(Event{Kind: EventPlainChar, Text: "b", Now: base.Add(1 * time.Millisecond)})
	c.Feed(Event{Kind: EventTick, Now: base.Add(11 * time.Millisecond)})
	if c.Mode() != ModeEnterSuppress {
		t.Fatalf("mode: want ModeEnterSuppress, got %v", c.Mode())
	}

	if got := c.Feed(Event{Kind: EventPlainChar, Text: "x", Now: base.Add(12 * time.Millisecond)}); got != nil {
		t.Fatalf("'x' in suppress: want no action, got %v", got)
	}
	if c.Mode() != ModePasteHold {
		t.Fatalf("mode: want ModePasteHold, got %v", c.Mode())
	}

	acts := c.Feed(Event{Kind: EventTick, Now: base.Add(22 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionTyped, "x"}}) {
		t.Fatalf("tick: want Typed(x), got %v", acts)
	}
	if c.Mode() != ModeIdle {
		t.Fatalf("mode: want ModeIdle, got %v", c.Mode())
	}
}

// 边界：各状态下 tick 未满足超时条件时返回 nil。
func TestTickNoOp(t *testing.T) {
	base := baseTime()

	// idle 时 tick → nil。
	c := New(Config{})
	if got := c.Feed(Event{Kind: EventTick, Now: base}); got != nil {
		t.Fatalf("tick idle: want nil, got %v", got)
	}

	// 突发但间隔未超时 → nil。
	c = New(Config{})
	c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base})
	c.Feed(Event{Kind: EventPlainChar, Text: "b", Now: base.Add(1 * time.Millisecond)})
	if got := c.Feed(Event{Kind: EventTick, Now: base.Add(5 * time.Millisecond)}); got != nil {
		t.Fatalf("tick within burst: want nil, got %v", got)
	}
	if c.Mode() != ModePasteBurst {
		t.Fatalf("mode: want ModePasteBurst, got %v", c.Mode())
	}

	// hold 但未超时 → nil。
	c = New(Config{})
	c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base})
	if got := c.Feed(Event{Kind: EventTick, Now: base.Add(1 * time.Millisecond)}); got != nil {
		t.Fatalf("tick within hold: want nil, got %v", got)
	}
	if c.Mode() != ModePasteHold {
		t.Fatalf("mode: want ModePasteHold, got %v", c.Mode())
	}

	// EnterSuppress 窗口内 tick → nil。
	c = New(Config{})
	c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base})
	c.Feed(Event{Kind: EventPlainChar, Text: "b", Now: base.Add(1 * time.Millisecond)})
	c.Feed(Event{Kind: EventTick, Now: base.Add(11 * time.Millisecond)}) // flush → EnterSuppress
	if got := c.Feed(Event{Kind: EventTick, Now: base.Add(15 * time.Millisecond)}); got != nil {
		t.Fatalf("tick within suppress window: want nil, got %v", got)
	}
	if c.Mode() != ModeEnterSuppress {
		t.Fatalf("mode: want ModeEnterSuppress, got %v", c.Mode())
	}
}

// Reset 清空到 idle。
func TestReset(t *testing.T) {
	c := New(Config{})
	base := baseTime()

	c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base})
	c.Feed(Event{Kind: EventPlainChar, Text: "b", Now: base.Add(1 * time.Millisecond)})
	if c.Pending() != "ab" {
		t.Fatalf("pending: want ab, got %q", c.Pending())
	}

	c.Reset()
	if c.Mode() != ModeIdle {
		t.Fatalf("mode after reset: want ModeIdle, got %v", c.Mode())
	}
	if c.Pending() != "" {
		t.Fatalf("pending after reset: want empty, got %q", c.Pending())
	}
	acts := c.Feed(Event{Kind: EventEnter, Now: base.Add(100 * time.Millisecond)})
	if !sameActions(acts, []Action{{Kind: ActionSubmit}}) {
		t.Fatalf("enter after reset: want Submit, got %v", acts)
	}
}

// 默认配置数值。
func TestDefaultConfigValues(t *testing.T) {
	def := DefaultConfig()
	if def.BurstCharInterval != 2*time.Millisecond {
		t.Fatalf("BurstCharInterval: want 2ms, got %v", def.BurstCharInterval)
	}
	if def.BurstActiveIdleTimeout != 10*time.Millisecond {
		t.Fatalf("BurstActiveIdleTimeout: want 10ms, got %v", def.BurstActiveIdleTimeout)
	}
	if def.EnterSuppressWindow != 10*time.Millisecond {
		t.Fatalf("EnterSuppressWindow: want 10ms, got %v", def.EnterSuppressWindow)
	}
}

// New 对 <=0 字段回填默认值。
func TestNewFillsDefaults(t *testing.T) {
	c := New(Config{})
	if c.cfg.BurstCharInterval != 2*time.Millisecond {
		t.Fatalf("burst interval default not applied: %v", c.cfg.BurstCharInterval)
	}
	if c.cfg.BurstActiveIdleTimeout != 10*time.Millisecond {
		t.Fatalf("idle timeout default not applied: %v", c.cfg.BurstActiveIdleTimeout)
	}
	if c.cfg.EnterSuppressWindow != 10*time.Millisecond {
		t.Fatalf("enter window default not applied: %v", c.cfg.EnterSuppressWindow)
	}
}

// 自定义配置被实际使用（而非回退默认）。
func TestCustomConfigHonored(t *testing.T) {
	c := New(Config{
		BurstCharInterval:    10 * time.Millisecond,
		BurstActiveIdleTimeout: 20 * time.Millisecond,
		EnterSuppressWindow:  5 * time.Millisecond,
	})
	base := baseTime()

	c.Feed(Event{Kind: EventPlainChar, Text: "a", Now: base})
	c.Feed(Event{Kind: EventPlainChar, Text: "b", Now: base.Add(5 * time.Millisecond)}) // 5ms < 10ms → 突发
	if c.Mode() != ModePasteBurst {
		t.Fatalf("mode: want ModePasteBurst, got %v", c.Mode())
	}
	// 19ms < 20ms 超时 → 仍不冲刷。
	if got := c.Feed(Event{Kind: EventTick, Now: base.Add(24 * time.Millisecond)}); got != nil {
		t.Fatalf("tick under timeout: want nil, got %v", got)
	}
	// 20ms >= 20ms → 冲刷。
	acts := c.Feed(Event{Kind: EventTick, Now: base.Add(25 * time.Millisecond)})
	if !sameActions(acts, []Action{{ActionPaste, "ab"}}) {
		t.Fatalf("flush: want Paste(ab), got %v", acts)
	}
	// EnterSuppressWindow=5ms：flush 在 base+25ms → 窗口到 base+30ms。
	acts = c.Feed(Event{Kind: EventEnter, Now: base.Add(28 * time.Millisecond)})
	if !sameActions(acts, []Action{{Kind: ActionInsertNewline}}) {
		t.Fatalf("enter: want InsertNewline, got %v", acts)
	}
}

// TickInterval 返回配置的突发活动空闲超时。
func TestTickInterval(t *testing.T) {
	c := New(Config{})
	if got := c.TickInterval(); got != 10*time.Millisecond {
		t.Fatalf("TickInterval: want 10ms, got %v", got)
	}
	custom := New(Config{BurstActiveIdleTimeout: 25 * time.Millisecond})
	if got := custom.TickInterval(); got != 25*time.Millisecond {
		t.Fatalf("TickInterval custom: want 25ms, got %v", got)
	}
}

// firstRune 辅助函数。
func TestFirstRune(t *testing.T) {
	if got := firstRune("abc"); got != 'a' {
		t.Fatalf("firstRune(abc): want 'a', got %q", got)
	}
	if got := firstRune("中"); got != '中' {
		t.Fatalf("firstRune(中): want '中', got %q", got)
	}
	if got := firstRune(""); got != 0 {
		t.Fatalf("firstRune(empty): want 0, got %q", got)
	}
}
