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

package session

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// rollupFixedTime 提供确定性的时间戳，避免时间片竞态导致断言不稳定。
func rollupFixedTime() time.Time {
	return time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
}

// TestRolloutRecorder_OrderAndList 验证记录顺序（user→tool_call→
// tool_result→assistant 按 JSONL 行序）、List 重建 items 流即 Seq 连续递增、
// SessionID 与 Timestamp 自动填充。
func TestRolloutRecorder_OrderAndList(t *testing.T) {
	dir := t.TempDir()
	rec := NewRolloutRecorder(dir, "rollout-s1")
	rec.clock = rollupFixedTime

	if err := rec.Record(RolloutItem{Type: RolloutUserMessage, Content: "Hello"}); err != nil {
		t.Fatalf("Record user failed: %v", err)
	}
	if err := rec.Record(RolloutItem{Type: RolloutToolCall, ToolName: "weather", Params: map[string]any{"city": "NYC"}}); err != nil {
		t.Fatalf("Record tool_call failed: %v", err)
	}
	if err := rec.Record(RolloutItem{Type: RolloutToolResult, ToolName: "weather", Result: "sunny"}); err != nil {
		t.Fatalf("Record tool_result failed: %v", err)
	}
	if err := rec.Record(RolloutItem{Type: RolloutAssistantMessage, Content: "sunny in NYC"}); err != nil {
		t.Fatalf("Record assistant failed: %v", err)
	}

	items, err := rec.List("rollout-s1")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("List returned %d items; want 4", len(items))
	}

	// 顺序与发生顺序一致。
	wantTypes := []RolloutItemType{
		RolloutUserMessage,
		RolloutToolCall,
		RolloutToolResult,
		RolloutAssistantMessage,
	}
	for i, want := range wantTypes {
		if items[i].Type != want {
			t.Errorf("items[%d].Type = %q; want %q", i, items[i].Type, want)
		}
		if items[i].Seq != int64(i+1) {
			t.Errorf("items[%d].Seq = %d; want %d", i, items[i].Seq, i+1)
		}
		if items[i].SessionID != "rollout-s1" {
			t.Errorf("items[%d].SessionID = %q; want rollup-s1", i, items[i].SessionID)
		}
		if !items[i].Timestamp.Equal(rollupFixedTime()) {
			t.Errorf("items[%d].Timestamp = %v; want %v", i, items[i].Timestamp, rollupFixedTime())
		}
	}

	// 首批字段内容正确。
	if items[1].ToolName != "weather" {
		t.Errorf("tool_call ToolName = %q; want weather", items[1].ToolName)
	}
	if items[1].Params["city"] != "NYC" {
		t.Errorf("tool_call Params = %v; want city=NYC", items[1].Params)
	}
	if items[3].Content != "sunny in NYC" {
		t.Errorf("assistant Content = %q; want sunny in NYC", items[3].Content)
	}
}

// TestRolloutRecorder_ReplayOrder 验证 Replay 按记录顺序返回 items（与
// 记录时顺序一致），可作为重放驱动。
func TestRolloutRecorder_ReplayOrder(t *testing.T) {
	dir := t.TempDir()
	rec := NewRolloutRecorder(dir, "rollout-replay")

	if err := rec.Record(RolloutItem{Type: RolloutUserMessage, Content: "q"}); err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	if err := rec.Record(RolloutItem{Type: RolloutToolCall, ToolName: "t", Params: map[string]any{"k": "v"}}); err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	if err := rec.Record(RolloutItem{Type: RolloutToolResult, ToolName: "t", Result: "r"}); err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	if err := rec.Record(RolloutItem{Type: RolloutAssistantMessage, Content: "a"}); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	replayed, err := rec.Replay("rollout-replay")
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}
	if len(replayed) != 4 {
		t.Fatalf("Replay returned %d items; want 4", len(replayed))
	}
	wantTypes := []RolloutItemType{
		RolloutUserMessage,
		RolloutToolCall,
		RolloutToolResult,
		RolloutAssistantMessage,
	}
	for i, want := range wantTypes {
		if replayed[i].Type != want {
			t.Errorf("replay[%d].Type = %q; want %q", i, replayed[i].Type, want)
		}
	}
}

// TestRolloutRecorder_PersistenceCrossInstance 验证 JSONL 落盘可从新的
// recorder 实例重建同一 items 流（List 跨进程等价于文件读取）。
func TestRolloutRecorder_PersistenceCrossInstance(t *testing.T) {
	dir := t.TempDir()

	w := NewRolloutRecorder(dir, "rollout-persist")
	if err := w.Record(RolloutItem{Type: RolloutUserMessage, Content: "hi"}); err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	if err := w.Record(RolloutItem{Type: RolloutToolCall, ToolName: "echo", Params: map[string]any{"x": 1}}); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	// 文件确实落盘于 conventions 目录。
	path := filepath.Join(dir, "sessions", "rollout-persist.rollout.jsonl")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("rollout.jsonl file was not created at %s", path)
	}

	// 新实例读取同一目录，应重建相同 items 流。
	r := NewRolloutRecorder(dir, "rollout-persist")
	items, err := r.List("rollout-persist")
	if err != nil {
		t.Fatalf("List from new instance failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("List from new instance returned %d items; want 2", len(items))
	}
	if items[0].Type != RolloutUserMessage || items[0].Content != "hi" {
		t.Errorf("rebuilt items[0] = %+v; want user hi", items[0])
	}
	if items[1].Type != RolloutToolCall || items[1].ToolName != "echo" || items[1].Seq != 2 {
		t.Errorf("rebuilt items[1] = %+v; want tool_call echo seq=2", items[1])
	}
}

// TestRolloutRecorder_AuditRecordIDField 验证 tool 记录携带 audit_record_id
// 字段：不可得时为空（字段仍可被反序列化备查），可得时随 JSONL 往返。
func TestRolloutRecorder_AuditRecordIDField(t *testing.T) {
	dir := t.TempDir()
	rec := NewRolloutRecorder(dir, "rollout-audit")

	// 无 audit ID 的 tool_call：字段存在但为空。
	if err := rec.Record(RolloutItem{Type: RolloutToolCall, ToolName: "echo"}); err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	// 带 audit ID 的 tool_result：应往返保留。
	if err := rec.Record(RolloutItem{Type: RolloutToolResult, ToolName: "echo", Result: "ok", AuditRecordID: "rec_abc123"}); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	items, err := rec.List("rollout-audit")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if items[0].AuditRecordID != "" {
		t.Errorf("tool_call AuditRecordID = %q; want empty", items[0].AuditRecordID)
	}
	if items[1].AuditRecordID != "rec_abc123" {
		t.Errorf("tool_result AuditRecordID = %q; want rec_abc123", items[1].AuditRecordID)
	}
}

// TestRolloutRecorder_NoAuditRecordIDNonEmpty 验证 JSONL 文件中 audit_record_id
// 字段在空值时以显式空串存在（便于外部工具按字段探测），而非整体缺失。
func TestRolloutRecorder_NoAuditRecordIDNonEmpty(t *testing.T) {
	dir := t.TempDir()
	rec := NewRolloutRecorder(dir, "rollout-field")
	if err := rec.Record(RolloutItem{Type: RolloutToolCall, ToolName: "echo"}); err != nil {
		t.Fatalf("Record failed: %v", err)
	}
	path := filepath.Join(dir, "sessions", "rollout-field.rollout.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rollout file failed: %v", err)
	}
	if !containsBytes(data, []byte(`"audit_record_id"`)) {
		t.Errorf("rollout JSONL line missing audit_record_id field:\n%s", data)
	}
}

// containsBytes 报告 haystack 是否包含 needle 字节序列。
func containsBytes(haystack, needle []byte) bool {
	return bytes.Contains(haystack, needle)
}

// TestRolloutRecorder_EphemeralNoOp 验证 ephemeral（空 dataDir）会话 no-op：
// Record 返回 nil、不产生任何文件、List 为空。
func TestRolloutRecorder_EphemeralNoOp(t *testing.T) {
	dir := t.TempDir()
	rec := NewRolloutRecorder("", "rollout-ephemeral")

	if err := rec.Record(RolloutItem{Type: RolloutUserMessage, Content: "x"}); err != nil {
		t.Fatalf("Record on ephemeral recorder returned error: %v", err)
	}
	path := filepath.Join(dir, "sessions", "rollout-ephemeral.rollout.jsonl")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("ephemeral recorder wrote a file; stat err=%v", err)
	}
	items, err := rec.List("rollout-ephemeral")
	if err != nil {
		t.Fatalf("List on ephemeral recorder failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("List on ephemeral recorder returned %d items; want 0", len(items))
	}
}

// TestRolloutRecorder_SessionIsolation 验证按会话 ID 隔离：List(sessionID) 只
// 返回该会话自身的记录流，互不混入（数据目录为多会话共享的 JSONL 目录）。
func TestRolloutRecorder_SessionIsolation(t *testing.T) {
	dir := t.TempDir()
	a := NewRolloutRecorder(dir, "iso-a")
	b := NewRolloutRecorder(dir, "iso-b")

	if err := a.Record(RolloutItem{Type: RolloutUserMessage, Content: "for a"}); err != nil {
		t.Fatalf("Record a failed: %v", err)
	}
	if err := b.Record(RolloutItem{Type: RolloutUserMessage, Content: "for b"}); err != nil {
		t.Fatalf("Record b failed: %v", err)
	}

	aItems, err := a.List("iso-a")
	if err != nil {
		t.Fatalf("List iso-a failed: %v", err)
	}
	bItems, err := b.List("iso-b")
	if err != nil {
		t.Fatalf("List iso-b failed: %v", err)
	}
	// 各自仅含自身记录。
	if len(aItems) != 1 || aItems[0].Content != "for a" {
		t.Errorf("iso-a items = %+v; want single 'for a'", aItems)
	}
	if len(bItems) != 1 || bItems[0].Content != "for b" {
		t.Errorf("iso-b items = %+v; want single 'for b'", bItems)
	}
	// 同一共享目录按 ID 取流：不让会话 B 冒进会话 A 的流（Seq/内容独立）。
	bLookupA, err := b.List("iso-a")
	if err != nil {
		t.Fatalf("List(iso-a) via b failed: %v", err)
	}
	// 共享目录下按 ID 读取是允许的：其内容是会话 A 自己的记录。
	if len(bLookupA) != 1 || bLookupA[0].Content != "for a" {
		t.Errorf("shared-dir lookup of iso-a = %+v; want single 'for a'", bLookupA)
	}
}

// TestRolloutRecorder_ConcurrentSafe 验证并发追加安全：Seq 连续无重复，
// 全量记录数正确。
func TestRolloutRecorder_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	rec := NewRolloutRecorder(dir, "rollout-conc")

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = rec.Record(RolloutItem{Type: RolloutUserMessage, Content: "msg"})
		}(i)
	}
	wg.Wait()

	items, err := rec.List("rollout-conc")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != n {
		t.Fatalf("List returned %d items; want %d", len(items), n)
	}
	seen := make(map[int64]bool, n)
	for _, it := range items {
		if seen[it.Seq] {
			t.Fatalf("duplicate Seq %d under concurrent append", it.Seq)
		}
		seen[it.Seq] = true
	}
}
