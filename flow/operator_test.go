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
	"testing"
)

// Check: 13 种 OperatorKind 常量字符串值正确
func TestOperatorKindConstants(t *testing.T) {
	kinds := []struct {
		name OperatorKind
		want string
	}{
		{OpChunk, "chunk"},
		{OpSignalGate, "signal_gate"},
		{OpBatchFanout, "batch_fanout"},
		{OpBatchCollect, "batch_collect"},
		{OpForEachSplit, "for_each_split"},
		{OpForEachCollect, "for_each_collect"},
		{OpMatchRoute, "match_route"},
		{OpMatchCase, "match_case"},
		{OpMatchCollect, "match_collect"},
		{OpCollectBranch, "collect_branch"},
		{OpIntervention, "intervention_point"},
		{OpSubFlow, "sub_flow"},
		{OpResultSink, "result_sink"},
	}
	if len(kinds) != 13 {
		t.Fatalf("expected 13 operator kinds, got %d", len(kinds))
	}
	for _, k := range kinds {
		if string(k.name) != k.want {
			t.Errorf("OperatorKind constant = %q, want %q", k.name, k.want)
		}
	}
}

// Check: CallableRef 构造与字段
func TestCallableRef(t *testing.T) {
	ref := &CallableRef{
		Kind:     CallableRegistered,
		Name:     "myHandler",
		Module:   "main",
		Qualname: "main.myHandler",
		Line:     42,
	}
	if ref.Kind != CallableRegistered {
		t.Errorf("Kind = %q, want %q", ref.Kind, CallableRegistered)
	}
	if ref.Name != "myHandler" {
		t.Errorf("Name = %q, want myHandler", ref.Name)
	}
	if ref.Module != "main" {
		t.Errorf("Module = %q, want main", ref.Module)
	}
	if ref.Qualname != "main.myHandler" {
		t.Errorf("Qualname = %q, want main.myHandler", ref.Qualname)
	}
	if ref.Line != 42 {
		t.Errorf("Line = %d, want 42", ref.Line)
	}
}

// Check: CallableRef Kind 常量
func TestCallableRefKinds(t *testing.T) {
	if CallableRegistered != "registered" {
		t.Errorf("CallableRegistered = %q, want registered", CallableRegistered)
	}
	if CallableInspected != "inspected" {
		t.Errorf("CallableInspected = %q, want inspected", CallableInspected)
	}
	if CallableAnonymous != "anonymous" {
		t.Errorf("CallableAnonymous = %q, want anonymous", CallableAnonymous)
	}
}

// Check: Operator 构造包含所有字段
func TestOperatorConstruction(t *testing.T) {
	ref := &CallableRef{Kind: CallableAnonymous, Name: "handler1"}
	op := Operator{
		ID:            "op-1",
		Kind:          OpChunk,
		Name:          "process_chunk",
		ListenSignals: []string{"START"},
		EmitSignals:   []string{"chunk_done"},
		Options:       map[string]any{"batch_size": 10},
		HandlerRef:    ref,
	}
	if op.ID != "op-1" {
		t.Errorf("ID = %q, want op-1", op.ID)
	}
	if op.Kind != OpChunk {
		t.Errorf("Kind = %q, want %q", op.Kind, OpChunk)
	}
	if op.Name != "process_chunk" {
		t.Errorf("Name = %q, want process_chunk", op.Name)
	}
	if len(op.ListenSignals) != 1 || op.ListenSignals[0] != "START" {
		t.Errorf("ListenSignals = %v, want [START]", op.ListenSignals)
	}
	if len(op.EmitSignals) != 1 || op.EmitSignals[0] != "chunk_done" {
		t.Errorf("EmitSignals = %v, want [chunk_done]", op.EmitSignals)
	}
	if op.HandlerRef == nil || op.HandlerRef.Name != "handler1" {
		t.Error("HandlerRef not set correctly")
	}
	if op.Options["batch_size"] != 10 {
		t.Errorf("Options[batch_size] = %v, want 10", op.Options["batch_size"])
	}
}

// Check: Operator 可以用不同 Kind 构造
func TestOperatorDifferentKinds(t *testing.T) {
	ops := []Operator{
		{ID: "1", Kind: OpSignalGate, Name: "gate"},
		{ID: "2", Kind: OpBatchFanout, Name: "fanout"},
		{ID: "3", Kind: OpMatchRoute, Name: "route"},
		{ID: "4", Kind: OpIntervention, Name: "intervene"},
		{ID: "5", Kind: OpResultSink, Name: "sink"},
	}
	for _, op := range ops {
		if op.Kind == "" {
			t.Errorf("Operator %s has empty Kind", op.ID)
		}
	}
}

// Check: OperatorRegistry 注册和查找
func TestOperatorRegistryRegister(t *testing.T) {
	reg := NewOperatorRegistry()
	op := &Operator{
		ID:   "op-1",
		Kind: OpChunk,
		Name: "process",
	}
	if err := reg.Register(op); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if len(reg.List()) != 1 {
		t.Errorf("List() len = %d, want 1", len(reg.List()))
	}
}

// Check: OperatorRegistry Get 按 ID 查找
func TestOperatorRegistryGet(t *testing.T) {
	reg := NewOperatorRegistry()
	op := &Operator{ID: "op-1", Kind: OpChunk, Name: "process"}
	_ = reg.Register(op)

	found, err := reg.Get("op-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if found.ID != "op-1" {
		t.Errorf("Get returned ID %q, want op-1", found.ID)
	}
}

// Check: OperatorRegistry Get 不存在返回 error
func TestOperatorRegistryGetNotFound(t *testing.T) {
	reg := NewOperatorRegistry()
	_, err := reg.Get("non-existent")
	if err == nil {
		t.Fatal("expected error for non-existent operator")
	}
}

// Check: OperatorRegistry 重复注册返回 error
func TestOperatorRegistryDuplicate(t *testing.T) {
	reg := NewOperatorRegistry()
	op1 := &Operator{ID: "op-1", Kind: OpChunk, Name: "a"}
	op2 := &Operator{ID: "op-1", Kind: OpChunk, Name: "b"}
	_ = reg.Register(op1)
	if err := reg.Register(op2); err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

// Check: OperatorRegistry nil 注册返回 error
func TestOperatorRegistryNil(t *testing.T) {
	reg := NewOperatorRegistry()
	if err := reg.Register(nil); err == nil {
		t.Fatal("expected error for nil operator")
	}
}

// Check: OperatorRegistry 按 ListenSignals 查找
func TestOperatorRegistryFindByListenSignal(t *testing.T) {
	reg := NewOperatorRegistry()
	_ = reg.Register(&Operator{ID: "op-1", Kind: OpChunk, ListenSignals: []string{"START", "resume"}})
	_ = reg.Register(&Operator{ID: "op-2", Kind: OpSignalGate, ListenSignals: []string{"chunk_done"}})
	_ = reg.Register(&Operator{ID: "op-3", Kind: OpResultSink, ListenSignals: []string{"START", "final"}})

	matching := reg.FindByListenSignal("START")
	if len(matching) != 2 {
		t.Fatalf("FindByListenSignal(START) = %d ops, want 2", len(matching))
	}

	matching = reg.FindByListenSignal("chunk_done")
	if len(matching) != 1 {
		t.Fatalf("FindByListenSignal(chunk_done) = %d ops, want 1", len(matching))
	}

	matching = reg.FindByListenSignal("nonexistent")
	if len(matching) != 0 {
		t.Errorf("FindByListenSignal(nonexistent) = %d ops, want 0", len(matching))
	}
}
