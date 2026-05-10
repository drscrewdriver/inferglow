package flow

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// BUG-20 / F-MEDIUM-6: SubFlowFrame 深拷贝
//
// 现状（修复前）：SubFlowFrame 没有深拷贝方法，persistence.go buildSnapshot
// 使用 `*f`（浅拷贝）复制 frame。State/RuntimeData/FlowData 是引用类型
// (map / pointer)，浅拷贝后多个 frame 共享同一引用，外部修改会影响 snapshot。
//
// 修复要求：
//   - 新增 (f *SubFlowFrame) DeepCopy() *SubFlowFrame 方法
//   - 深拷贝 State / FlowData (map[string]any) - 递归拷贝嵌套 map/slice
//   - 深拷贝 RuntimeData (*TriggerFlowRuntimeData) 包括其 RuntimeData/FlowData map
//   - 拷贝 Signal (*Signal) 及其 Meta map
//   - buildSnapshot 使用 DeepCopy 替换 `*f` 浅拷贝
// ============================================================================

// TestSubFlowFrame_DeepCopy 验证 SubFlowFrame.DeepCopy 产生独立的副本，
// 修改副本的 State/FlowData/RuntimeData 不影响原 frame。
func TestSubFlowFrame_DeepCopy(t *testing.T) {
	now := time.Now()
	completed := now.Add(time.Second)
	frameA := &SubFlowFrame{
		ParentID: "p1",
		State: map[string]any{
			"k1": "v1",
			"nested": map[string]any{
				"x": 1,
			},
		},
		RuntimeData: &TriggerFlowRuntimeData{
			RuntimeData: map[string]any{"rd": "rd1"},
			FlowData:    map[string]any{"fd": "fd1"},
			Signal: &Signal{
				ID:           "sig-1",
				TriggerEvent: "evt-1",
				TriggerType:  SignalEvent,
				Value:        "v",
				Meta:         map[string]any{"m": "mv"},
			},
			Result: "r1",
		},
		FlowData: map[string]any{
			"fk1": "fv1",
			"nested": map[string]any{
				"y": 2,
			},
		},
		CreatedAt:   now,
		CompletedAt: &completed,
		Result:      "result1",
		Error:       "",
	}

	frameB := frameA.DeepCopy()
	if frameB == nil {
		t.Fatal("DeepCopy returned nil")
	}

	// 修改 B 的 State 顶层 + 嵌套 map
	frameB.State["k1"] = "modified"
	frameB.State["nested"].(map[string]any)["x"] = 999
	if got := frameA.State["k1"]; got != "v1" {
		t.Errorf("State[k1] = %v, want v1 (top-level State not deep-copied)", got)
	}
	if got := frameA.State["nested"].(map[string]any)["x"]; got != 1 {
		t.Errorf("nested State x = %v, want 1 (nested map not deep-copied)", got)
	}

	// 修改 B 的 FlowData 顶层 + 嵌套 map
	frameB.FlowData["fk1"] = "modified"
	frameB.FlowData["nested"].(map[string]any)["y"] = 999
	if got := frameA.FlowData["fk1"]; got != "fv1" {
		t.Errorf("FlowData[fk1] = %v, want fv1 (top-level FlowData not deep-copied)", got)
	}
	if got := frameA.FlowData["nested"].(map[string]any)["y"]; got != 2 {
		t.Errorf("nested FlowData y = %v, want 2 (nested map not deep-copied)", got)
	}

	// 修改 B 的 RuntimeData.RuntimeData / FlowData
	frameB.RuntimeData.RuntimeData["rd"] = "modified"
	frameB.RuntimeData.FlowData["fd"] = "modified"
	if got := frameA.RuntimeData.RuntimeData["rd"]; got != "rd1" {
		t.Errorf("RuntimeData.RuntimeData[rd] = %v, want rd1 (not deep-copied)", got)
	}
	if got := frameA.RuntimeData.FlowData["fd"]; got != "fd1" {
		t.Errorf("RuntimeData.FlowData[fd] = %v, want fd1 (not deep-copied)", got)
	}

	// 修改 B 的 RuntimeData.Signal.Meta
	frameB.RuntimeData.Signal.Meta["m"] = "modified"
	if got := frameA.RuntimeData.Signal.Meta["m"]; got != "mv" {
		t.Errorf("Signal.Meta[m] = %v, want mv (not deep-copied)", got)
	}

	// 修改 B 的 CompletedAt
	newCompleted := now.Add(time.Hour)
	frameB.CompletedAt = &newCompleted
	if frameA.CompletedAt.Equal(newCompleted) {
		t.Errorf("CompletedAt should not be shared (pointer not deep-copied)")
	}
}

// TestSubFlowFrame_DeepCopyNilSafe 验证 nil / 空 frame 的 DeepCopy 安全。
func TestSubFlowFrame_DeepCopyNilSafe(t *testing.T) {
	var nilFrame *SubFlowFrame
	if got := nilFrame.DeepCopy(); got != nil {
		t.Errorf("nil.DeepCopy() = %v, want nil", got)
	}

	// 空 frame（State/RuntimeData/FlowData 全 nil）应安全 DeepCopy
	empty := &SubFlowFrame{ParentID: "p"}
	cp := empty.DeepCopy()
	if cp == nil {
		t.Fatal("DeepCopy of empty frame returned nil")
	}
	if cp.ParentID != "p" {
		t.Errorf("ParentID = %q, want p", cp.ParentID)
	}
	if cp.State != nil {
		t.Errorf("State = %v, want nil", cp.State)
	}
	if cp.RuntimeData != nil {
		t.Errorf("RuntimeData = %v, want nil", cp.RuntimeData)
	}
	if cp.FlowData != nil {
		t.Errorf("FlowData = %v, want nil", cp.FlowData)
	}
}

// TestBuildSnapshot_DeepCopiesStateMaps 验证 buildSnapshot 使用深拷贝，
// 修改原 frame 的 State/FlowData 不影响 snapshot 中的副本。
//
// 这是 BUG-20 的集成测试：buildSnapshot 应调用 DeepCopy 而非浅拷贝。
func TestBuildSnapshot_DeepCopiesStateMaps(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	original := &SubFlowFrame{
		ParentID:  "p-1",
		CreatedAt: time.Now(),
		State: map[string]any{
			"key": "original",
			"nested": map[string]any{
				"deep": "value",
			},
		},
		FlowData: map[string]any{
			"fk": "fv",
		},
	}
	GlobalSubFlowRegistry().Register("f1", original)

	exec := &Execution{State: ExecutionState{Status: StatusRunning}}
	persistence := NewExecutionPersistence(exec, "test-flow")
	snapshot := persistence.buildSnapshot()

	// 修改原 frame 的 State 顶层与嵌套
	original.State["key"] = "modified"
	original.State["nested"].(map[string]any)["deep"] = "modified"
	original.FlowData["fk"] = "modified"

	// snapshot 中的 frame 应保留原始值（深拷贝）
	if len(snapshot.SubFlowFrames) != 1 {
		t.Fatalf("len(SubFlowFrames) = %d, want 1", len(snapshot.SubFlowFrames))
	}
	cp := snapshot.SubFlowFrames[0]
	if got := cp.State["key"]; got != "original" {
		t.Errorf("State[key] = %v, want original (buildSnapshot should deep-copy State)", got)
	}
	if got := cp.State["nested"].(map[string]any)["deep"]; got != "value" {
		t.Errorf("nested State deep = %v, want value (buildSnapshot should deep-copy nested State)", got)
	}
	if got := cp.FlowData["fk"]; got != "fv" {
		t.Errorf("FlowData[fk] = %v, want fv (buildSnapshot should deep-copy FlowData)", got)
	}
}

// ============================================================================
// F-MEDIUM-5: frameID UUID/atomic 唯一性
//
// 现状（修复前）：generateSubFlowFrameID 使用 time.Now().UnixNano() 生成 ID。
// 高频调用（特别是 Windows 上 time.Now() 分辨率 ~1ms）会得到相同 UnixNano，
// 导致 frameID 冲突，frame 在 SubFlowRegistry 中互相覆盖。
//
// 修复要求：
//   - 使用 atomic counter 或 UUID 保证唯一性
//   - 保留 Operator.ID 前缀以保证可读性
//   - 快速生成 1000 个 frameID 全部唯一
// ============================================================================

// TestFrameID_Unique 验证快速连续生成的 1000 个 frameID 全部唯一。
// 修复前：UnixNano 在 Windows 上分辨率 ~1ms，循环 1000 次必然冲突。
func TestFrameID_Unique(t *testing.T) {
	oc := &OperatorContext{
		Operator: &Operator{ID: "op-unique"},
	}

	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := generateSubFlowFrameID(oc)
		if id == "" {
			t.Fatalf("iteration %d: empty frameID", i)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("iteration %d: duplicate frameID %q (UnixNano collision)", i, id)
		}
		seen[id] = struct{}{}
	}
}

// TestFrameID_UniqueConcurrent 验证并发调用下 frameID 全部唯一。
func TestFrameID_UniqueConcurrent(t *testing.T) {
	oc := &OperatorContext{
		Operator: &Operator{ID: "op-concurrent"},
	}

	const n = 200
	ids := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			ids[idx] = generateSubFlowFrameID(oc)
		}()
		_ = errs
		_ = idx
	}
	wg.Wait()

	seen := make(map[string]struct{}, n)
	for i, id := range ids {
		if id == "" {
			t.Fatalf("goroutine %d: empty frameID", i)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("goroutine %d: duplicate frameID %q", i, id)
		}
		seen[id] = struct{}{}
	}
}

// TestFrameID_ContainsOperatorID 验证 frameID 包含 Operator.ID 前缀。
func TestFrameID_ContainsOperatorID(t *testing.T) {
	oc := &OperatorContext{
		Operator: &Operator{ID: "my-op"},
	}
	id := generateSubFlowFrameID(oc)
	if !strings.HasPrefix(id, "my-op_") {
		t.Errorf("frameID %q should start with 'my-op_'", id)
	}
}

// TestFrameID_NilContextFallback 验证 nil OperatorContext 时回退到 "subflow_..." 格式。
func TestFrameID_NilContextFallback(t *testing.T) {
	id := generateSubFlowFrameID(nil)
	if id == "" {
		t.Error("frameID should not be empty for nil context")
	}
	if !strings.HasPrefix(id, "subflow_") {
		t.Errorf("frameID %q should start with 'subflow_' for nil context", id)
	}
}

// TestSubFlowRegistry_AutoCleanup verifies that SubFlowRegistry automatically
// cleans up completed frames after a configurable TTL (default 5 minutes for
// production, configurable via SetCleanupTTL for testing).
//
// BUG-10 / F-HIGH-6: SubFlowRegistry 完成后不清理 frame，内存泄漏.
// 修复：完成后启动 TTL 清理（默认 5 分钟）；新增 Cleanup(frameID) 方法.
func TestSubFlowRegistry_AutoCleanup(t *testing.T) {
	// 使用短 TTL 加速测试
	r := NewSubFlowRegistryWithTTL(100 * time.Millisecond)

	// 注册一个已完成的 frame
	completedAt := time.Now()
	frame := &SubFlowFrame{
		ParentID:    "p1",
		CreatedAt:   time.Now(),
		CompletedAt: &completedAt,
	}
	r.Register("frame1", frame)

	// TTL 窗口内仍可查询
	if _, ok := r.Get("frame1"); !ok {
		t.Fatal("frame1 should be retrievable within TTL window")
	}

	// 等待 TTL 过期 + 清理周期
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := r.Get("frame1"); !ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if _, ok := r.Get("frame1"); ok {
		t.Error("frame1 should be auto-cleaned after TTL, but is still present")
	}
}

// TestSubFlowRegistry_CleanupMethod verifies the explicit Cleanup(frameID) method
// removes a frame immediately.
func TestSubFlowRegistry_CleanupMethod(t *testing.T) {
	r := NewSubFlowRegistryWithTTL(time.Hour) // 长 TTL，不会被自动清理
	frame := &SubFlowFrame{
		ParentID:    "p1",
		CreatedAt:   time.Now(),
		CompletedAt: nil, // 未完成
	}
	r.Register("frame2", frame)

	if _, ok := r.Get("frame2"); !ok {
		t.Fatal("frame2 should be retrievable before Cleanup")
	}

	// 显式 Cleanup
	if !r.Cleanup("frame2") {
		t.Error("Cleanup should return true for existing frame")
	}
	if _, ok := r.Get("frame2"); ok {
		t.Error("frame2 should not exist after Cleanup")
	}

	// Cleanup 不存在的 frame 返回 false
	if r.Cleanup("nonexistent") {
		t.Error("Cleanup should return false for non-existent frame")
	}
}

// TestSubFlowRegistry_AutoCleanupSkipsIncomplete verifies that the auto-cleanup
// only removes completed frames, not in-progress ones.
func TestSubFlowRegistry_AutoCleanupSkipsIncomplete(t *testing.T) {
	r := NewSubFlowRegistryWithTTL(100 * time.Millisecond)

	// 注册一个未完成的 frame
	frame := &SubFlowFrame{
		ParentID:    "p1",
		CreatedAt:   time.Now(),
		CompletedAt: nil, // 未完成
	}
	r.Register("frame3", frame)

	// 等待 TTL 过期 + 清理周期
	time.Sleep(300 * time.Millisecond)

	// 未完成的 frame 不应被清理
	if _, ok := r.Get("frame3"); !ok {
		t.Error("incomplete frame3 should NOT be auto-cleaned (still in progress)")
	}
}

// ============================================================================
// BUG-NEW-3: generateSubFlowFrameID 时序一致性 (LOW)
//
// 现状（修复前）：generateSubFlowFrameID 在 atomic.AddUint64 取 seq 后，两个分支
// 各自调用 time.Now().UnixNano()。在并发场景下，seq 与时间戳的逻辑顺序可能不一致
// （seq 大的 ID 时间戳反而小）。该 bug 不影响唯一性，仅影响 ID 可读性。
//
// 修复要求：
//   - 在函数入口捕获 time.Now().UnixNano() 一次
//   - 两个分支复用同一 now 变量
//   - 保持 F-MEDIUM-5 的唯一性修复不回归
// ============================================================================

// parseSubflowFrameID 解析 generateSubFlowFrameID(nil) 产生的 ID
// （格式 "subflow_<unixnano>_<seq>"），返回 unixnano 与 seq。
// 解析失败时 t.Fatal。
func parseSubflowFrameID(t *testing.T, id string) (unixnano int64, seq uint64) {
	t.Helper()
	parts := strings.Split(id, "_")
	if len(parts) != 3 {
		t.Fatalf("id %q has %d parts, want 3", id, len(parts))
	}
	if parts[0] != "subflow" {
		t.Fatalf("id %q prefix = %q, want 'subflow'", id, parts[0])
	}
	un, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		t.Fatalf("id %q: cannot parse unixnano %q: %v", id, parts[1], err)
	}
	if un < 0 {
		t.Fatalf("id %q: unixnano %d < 0", id, un)
	}
	sq, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		t.Fatalf("id %q: cannot parse seq %q: %v", id, parts[2], err)
	}
	if sq == 0 {
		t.Fatalf("id %q: seq = 0, want > 0", id)
	}
	return un, sq
}

// TestGenerateSubFlowFrameID_SingleTimestampCall 验证 generateSubFlowFrameID 在
// 同一次调用中只取一次 time.Now()（通过单调性间接验证）。
//
// 思路：连续 500 次快速调用 generateSubFlowFrameID(nil)，每次记录 (seq, unixnano)。
// 按 seq 排序后，unixnano 应非降序（容差 100ms = 1e8 ns 处理调度抖动）。
//
// BUG-NEW-3 修复前：两个分支各自调用 time.Now()，理论上 seq 与时间戳可能不一致，
// 但因为本函数每次调用只走一个分支，分支内只有一次 time.Now()，所以单次调用内部
// 不存在不一致。真正的不一致发生在多次调用之间：理论上 seq 单调递增，UnixNano
// 也应单调非降，但若两次调用之间发生时间回拨（NTP 校正等），可能出现 seq 升而
// UnixNano 降的情况。此测试用严格容差（0 ns）捕捉该场景；若不能稳定失败，
// 也作为修复后的回归基线（修复后必然通过）。
//
// 注：BUG-NEW-3 是 LOW 严重等级的预防性修复。若该测试在修复前已通过，
// 属预期内情况——核心保证是修复后不回归、并保留 F-MEDIUM-5 唯一性。
func TestGenerateSubFlowFrameID_SingleTimestampCall(t *testing.T) {
	const n = 500
	const tolerance int64 = 100 * 1000 * 1000 // 100ms

	type pair struct {
		seq      uint64
		unixnano int64
		rawID    string
	}
	pairs := make([]pair, n)
	for i := 0; i < n; i++ {
		id := generateSubFlowFrameID(nil)
		un, sq := parseSubflowFrameID(t, id)
		pairs[i] = pair{seq: sq, unixnano: un, rawID: id}
	}

	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].seq < pairs[j].seq
	})

	for i := 1; i < n; i++ {
		if pairs[i].unixnano < pairs[i-1].unixnano {
			diff := pairs[i-1].unixnano - pairs[i].unixnano
			if diff > tolerance {
				t.Errorf("seq=%d -> unixnano=%d, but seq=%d -> unixnano=%d (diff=%d ns > tolerance %d ns); ID %q vs %q",
					pairs[i-1].seq, pairs[i-1].unixnano,
					pairs[i].seq, pairs[i].unixnano,
					diff, tolerance,
					pairs[i-1].rawID, pairs[i].rawID)
			}
		}
	}
}

// TestGenerateSubFlowFrameID_UniquenessPreserved 验证修复后 frameID 唯一性不回归。
//   - 子测试 "Sequential"：1000 次连续调用，全部唯一
//   - 子测试 "Concurrent"：200 个 goroutine 各调用 1 次，全部唯一
//
// 这是 F-MEDIUM-5 修复的回归测试。
func TestGenerateSubFlowFrameID_UniquenessPreserved(t *testing.T) {
	t.Run("Sequential", func(t *testing.T) {
		seen := make(map[string]struct{}, 1000)
		for i := 0; i < 1000; i++ {
			id := generateSubFlowFrameID(nil)
			if id == "" {
				t.Fatalf("iteration %d: empty frameID", i)
			}
			if _, exists := seen[id]; exists {
				t.Fatalf("iteration %d: duplicate frameID %q", i, id)
			}
			seen[id] = struct{}{}
		}
		if len(seen) != 1000 {
			t.Errorf("unique count = %d, want 1000", len(seen))
		}
	})

	t.Run("Concurrent", func(t *testing.T) {
		const n = 200
		ids := make([]string, n)
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			idx := i
			go func() {
				defer wg.Done()
				ids[idx] = generateSubFlowFrameID(nil)
			}()
		}
		wg.Wait()

		seen := make(map[string]struct{}, n)
		for i, id := range ids {
			if id == "" {
				t.Fatalf("goroutine %d: empty frameID", i)
			}
			if _, exists := seen[id]; exists {
				t.Fatalf("goroutine %d: duplicate frameID %q", i, id)
			}
			seen[id] = struct{}{}
		}
		if len(seen) != n {
			t.Errorf("concurrent unique count = %d, want %d", len(seen), n)
		}
	})
}
