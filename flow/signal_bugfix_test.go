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

package flow

import (
	"errors"
	"testing"
	"time"
)

// ============================================================================
// F-MEDIUM-3: AcceptSignal 合并 AttemptTracker
//
// 现状（修复前）：AcceptSignal 在重新接受同一 signalID 时静默覆盖旧条目，
// 替换为全新的 SignalAttemptTracker{Attempts: 0}。这丢失了之前的尝试历史
// （Attempts 计数 + LastError），不利于重试策略和诊断。
//
// 修复要求：
//   - 若信号已存在，保留原 tracker（Attempts + LastError）
//   - 替换 signal 引用为新的 Signal（更新 Value 等）
//   - 更新 acceptedAt 时间戳（重新接受视为最新）
// ============================================================================

// TestAcceptSignal_MergeTracker 验证重新接受同一 signalID 时保留原 tracker。
func TestAcceptSignal_MergeTracker(t *testing.T) {
	sn := NewSignalNet()

	// 第一次接受 sig-1，IncrementAttempt 2 次，MarkAttemptError 1 次
	sn.AcceptSignal(&Signal{ID: "sig-1", TriggerEvent: "evt", Value: "v1"})
	sn.IncrementAttempt("sig-1")
	sn.IncrementAttempt("sig-1")
	sn.MarkAttemptError("sig-1", errors.New("transient err"))

	tracker := sn.GetAttemptTracker("sig-1")
	if tracker == nil {
		t.Fatal("tracker is nil after first AcceptSignal")
	}
	if tracker.Attempts != 2 {
		t.Fatalf("Attempts = %d, want 2 (after 2 increments)", tracker.Attempts)
	}
	if tracker.LastError == nil || tracker.LastError.Error() != "transient err" {
		t.Fatalf("LastError = %v, want 'transient err'", tracker.LastError)
	}

	// 第二次接受同一 signalID（模拟重新触发）
	sn.AcceptSignal(&Signal{ID: "sig-1", TriggerEvent: "evt", Value: "v2"})

	// F-MEDIUM-3: tracker 应保留原 Attempts
	tracker = sn.GetAttemptTracker("sig-1")
	if tracker == nil {
		t.Fatal("tracker is nil after re-accept")
	}
	if tracker.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2 (preserved across re-accept)", tracker.Attempts)
	}
	// F-MEDIUM-3: tracker 应保留原 LastError
	if tracker.LastError == nil || tracker.LastError.Error() != "transient err" {
		t.Errorf("LastError = %v, want 'transient err' (preserved across re-accept)", tracker.LastError)
	}

	// 但 Signal 引用应被替换为新的（Value=v2）
	sig := sn.GetAcceptedSignal("sig-1")
	if sig == nil {
		t.Fatal("accepted signal is nil")
	}
	if sig.Value != "v2" {
		t.Errorf("Signal.Value = %v, want v2 (replaced on re-accept)", sig.Value)
	}

	// IncrementAttempt 应在保留计数的基础上递增
	sn.IncrementAttempt("sig-1")
	tracker = sn.GetAttemptTracker("sig-1")
	if tracker.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3 (incremented after merge)", tracker.Attempts)
	}
}

// TestAcceptSignal_MergeTrackerPreservesAfterMultipleReaccepts 验证多次重新接受后
// tracker 仍保留累计的 Attempts。
func TestAcceptSignal_MergeTrackerPreservesAfterMultipleReaccepts(t *testing.T) {
	sn := NewSignalNet()

	// 初始接受 + 1 次尝试
	sn.AcceptSignal(&Signal{ID: "sig-x", TriggerEvent: "evt"})
	sn.IncrementAttempt("sig-x")

	// 重新接受 5 次，每次后递增 1
	for i := 0; i < 5; i++ {
		sn.AcceptSignal(&Signal{ID: "sig-x", TriggerEvent: "evt"})
		sn.IncrementAttempt("sig-x")
	}

	tracker := sn.GetAttemptTracker("sig-x")
	if tracker == nil {
		t.Fatal("tracker is nil")
	}
	// F-MEDIUM-3: 应保留全部 6 次 Attempts（初始 1 + 后续 5）
	if tracker.Attempts != 6 {
		t.Errorf("Attempts = %d, want 6 (preserved across 5 re-accepts)", tracker.Attempts)
	}
}

// ============================================================================
// BUG-14 / F-MEDIUM-4: signalAccepted TTL 清理
//
// 现状（修复前）：SignalNet.signalAccepted map 只增不减，每个 AcceptSignal
// 调用都会永久存储 Signal 和 SignalAttemptTracker。在长时间运行的进程中
// （例如持续处理 trigger 事件的 TriggerFlow），该 map 会无限增长，导致
// 内存泄漏。
//
// 修复要求：
//   - 默认最大 10000 条（FIFO 驱逐最旧条目）
//   - 默认 TTL 1 小时（超过后自动清理）
//   - 提供 SetAcceptedSignalsLimit / SetAcceptedSignalsTTL 配置方法
//   - 提供 CleanupAcceptedSignals 显式清理方法
//   - 提供 AcceptedSignalsCount 用于测试与可观测性
// ============================================================================

// TestSignalNet_AcceptedSignalsSizeLimit 验证 signalAccepted 有大小上限。
// 设置 limit=5 后接受 10 个信号，map 大小不应超过 5。
func TestSignalNet_AcceptedSignalsSizeLimit(t *testing.T) {
	sn := NewSignalNet()
	sn.SetAcceptedSignalsLimit(5)

	for i := 0; i < 10; i++ {
		sn.AcceptSignal(&Signal{
			ID:           string(rune('a' + i)),
			TriggerEvent: "evt",
		})
	}

	if count := sn.AcceptedSignalsCount(); count > 5 {
		t.Fatalf("expected ≤5 accepted signals after size limit, got %d", count)
	}

	// 旧的信号应被驱逐（FIFO），只保留最新的 5 个（f, g, h, i, j）
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		if sn.IsAccepted(id) {
			t.Errorf("signal %q should have been evicted (FIFO)", id)
		}
	}
	for _, id := range []string{"f", "g", "h", "i", "j"} {
		if !sn.IsAccepted(id) {
			t.Errorf("signal %q should still be accepted (recent)", id)
		}
	}
}

// TestSignalNet_AcceptedSignalsTTL 验证 signalAccepted 有 TTL 清理。
// 设置 TTL=50ms 后接受信号，等待 100ms 后信号应被清理。
func TestSignalNet_AcceptedSignalsTTL(t *testing.T) {
	sn := NewSignalNet()
	sn.SetAcceptedSignalsTTL(50 * time.Millisecond)

	sn.AcceptSignal(&Signal{ID: "sig-ttl-1", TriggerEvent: "evt"})
	if !sn.IsAccepted("sig-ttl-1") {
		t.Fatal("signal should be accepted immediately after AcceptSignal")
	}

	time.Sleep(100 * time.Millisecond)

	// 触发清理（通过再次调用 AcceptSignal 或显式 CleanupAcceptedSignals）
	sn.CleanupAcceptedSignals()

	if sn.IsAccepted("sig-ttl-1") {
		t.Error("signal should be cleaned up after TTL expiry")
	}
}

// TestSignalNet_AcceptedSignalsTTLAutoCleanupOnAccept 验证 AcceptSignal
// 会顺带清理过期的条目。
func TestSignalNet_AcceptedSignalsTTLAutoCleanupOnAccept(t *testing.T) {
	sn := NewSignalNet()
	sn.SetAcceptedSignalsTTL(50 * time.Millisecond)

	sn.AcceptSignal(&Signal{ID: "sig-old", TriggerEvent: "evt"})
	time.Sleep(100 * time.Millisecond)

	// 接受新信号应顺带清理过期的 sig-old
	sn.AcceptSignal(&Signal{ID: "sig-new", TriggerEvent: "evt"})

	if sn.IsAccepted("sig-old") {
		t.Error("expired signal should be cleaned up when new signal is accepted")
	}
	if !sn.IsAccepted("sig-new") {
		t.Error("new signal should be accepted")
	}
}

// TestSignalNet_AcceptedSignalsDefaultLimits 验证默认配置：未调用 Set* 方法时
// 应有合理的默认值（limit > 0，TTL > 0）。
func TestSignalNet_AcceptedSignalsDefaultLimits(t *testing.T) {
	sn := NewSignalNet()
	if sn.GetAcceptedSignalsLimit() <= 0 {
		t.Error("default accepted signals limit should be > 0")
	}
	if sn.GetAcceptedSignalsTTL() <= 0 {
		t.Error("default accepted signals TTL should be > 0")
	}
}

// TestSignalNet_AcceptedSignalsNoEvictionWithoutLimit 验证 limit=0 表示无限制。
func TestSignalNet_AcceptedSignalsNoEvictionWithoutLimit(t *testing.T) {
	sn := NewSignalNet()
	sn.SetAcceptedSignalsLimit(0) // 无限制

	for i := 0; i < 100; i++ {
		sn.AcceptSignal(&Signal{
			ID:           string(rune('a'+i%26)) + string(rune('a'+i/26)),
			TriggerEvent: "evt",
		})
	}

	if count := sn.AcceptedSignalsCount(); count != 100 {
		t.Fatalf("expected 100 accepted signals (no limit), got %d", count)
	}
}

// TestSignalNet_CleanupAcceptedSignalsPreservesRecent 验证 CleanupAcceptedSignals
// 只清理过期条目，不影响未过期的条目。
func TestSignalNet_CleanupAcceptedSignalsPreservesRecent(t *testing.T) {
	sn := NewSignalNet()
	sn.SetAcceptedSignalsTTL(100 * time.Millisecond)

	sn.AcceptSignal(&Signal{ID: "sig-old", TriggerEvent: "evt"})
	time.Sleep(60 * time.Millisecond)
	sn.AcceptSignal(&Signal{ID: "sig-new", TriggerEvent: "evt"})
	time.Sleep(50 * time.Millisecond) // sig-old now ~110ms old, sig-new ~50ms old

	sn.CleanupAcceptedSignals()

	if sn.IsAccepted("sig-old") {
		t.Error("sig-old should be cleaned up (expired)")
	}
	if !sn.IsAccepted("sig-new") {
		t.Error("sig-new should still be accepted (not expired)")
	}
}
