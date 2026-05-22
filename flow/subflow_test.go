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
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// SubFlowFrame 测试
// ============================================================================

// Check: SubFlowFrame.IsCompleted 在新帧上返回 false
func TestSubFlowFrameNewNotCompleted(t *testing.T) {
	f := &SubFlowFrame{
		ParentID:  "parent-1",
		CreatedAt: time.Now(),
	}
	if f.IsCompleted() {
		t.Error("new frame should not be completed")
	}
}

// Check: SubFlowFrame.IsCompleted 在 CompletedAt 设置后返回 true
func TestSubFlowFrameCompleted(t *testing.T) {
	now := time.Now()
	f := &SubFlowFrame{
		ParentID:    "parent-1",
		CreatedAt:   now,
		CompletedAt: &now,
	}
	if !f.IsCompleted() {
		t.Error("frame with CompletedAt should be completed")
	}
}

// Check: nil SubFlowFrame 的 IsCompleted 安全返回 false
func TestSubFlowFrameNilSafe(t *testing.T) {
	var f *SubFlowFrame
	if f.IsCompleted() {
		t.Error("nil frame IsCompleted should return false")
	}
}

// ============================================================================
// SubFlowRegistry 测试
// ============================================================================

// Check: NewSubFlowRegistry 创建空注册表
func TestNewSubFlowRegistry(t *testing.T) {
	r := NewSubFlowRegistry()
	if r == nil {
		t.Fatal("NewSubFlowRegistry returned nil")
	}
	if r.Count() != 0 {
		t.Errorf("expected 0 frames, got %d", r.Count())
	}
}

// Check: Register + Get 正常工作
func TestSubFlowRegistryRegisterGet(t *testing.T) {
	r := NewSubFlowRegistry()
	f := &SubFlowFrame{ParentID: "p1", CreatedAt: time.Now()}
	r.Register("f1", f)

	got, ok := r.Get("f1")
	if !ok {
		t.Fatal("Get(f1) returned ok=false")
	}
	if got != f {
		t.Error("Get returned different frame")
	}
}

// Check: Get 未找到返回 false
func TestSubFlowRegistryGetMissing(t *testing.T) {
	r := NewSubFlowRegistry()
	if _, ok := r.Get("nonexistent"); ok {
		t.Error("Get(nonexistent) should return false")
	}
}

// Check: Unregister 移除 frame
func TestSubFlowRegistryUnregister(t *testing.T) {
	r := NewSubFlowRegistry()
	r.Register("f1", &SubFlowFrame{ParentID: "p1", CreatedAt: time.Now()})

	if !r.Unregister("f1") {
		t.Error("Unregister(f1) should return true")
	}
	if _, ok := r.Get("f1"); ok {
		t.Error("Get(f1) should return false after Unregister")
	}
	if r.Unregister("f1") {
		t.Error("Unregister(f1) second time should return false")
	}
}

// Check: Register 覆盖同名
func TestSubFlowRegistryRegisterOverride(t *testing.T) {
	r := NewSubFlowRegistry()
	f1 := &SubFlowFrame{ParentID: "p1", CreatedAt: time.Now()}
	f2 := &SubFlowFrame{ParentID: "p2", CreatedAt: time.Now()}

	r.Register("f", f1)
	r.Register("f", f2)

	got, _ := r.Get("f")
	if got != f2 {
		t.Error("override did not replace frame")
	}
	if r.Count() != 1 {
		t.Errorf("Count = %d, want 1", r.Count())
	}
}

// Check: List 返回所有 frame
func TestSubFlowRegistryList(t *testing.T) {
	r := NewSubFlowRegistry()
	r.Register("a", &SubFlowFrame{ParentID: "p1", CreatedAt: time.Now()})
	r.Register("b", &SubFlowFrame{ParentID: "p2", CreatedAt: time.Now()})
	r.Register("c", &SubFlowFrame{ParentID: "p3", CreatedAt: time.Now()})

	list := r.List()
	if len(list) != 3 {
		t.Errorf("len(List) = %d, want 3", len(list))
	}
}

// Check: Clear 清空所有 frame
func TestSubFlowRegistryClear(t *testing.T) {
	r := NewSubFlowRegistry()
	r.Register("a", &SubFlowFrame{ParentID: "p1", CreatedAt: time.Now()})
	r.Register("b", &SubFlowFrame{ParentID: "p2", CreatedAt: time.Now()})

	r.Clear()
	if r.Count() != 0 {
		t.Errorf("Count after Clear = %d, want 0", r.Count())
	}
}

// Check: nil registry 的方法安全返回
func TestSubFlowRegistryNilSafe(t *testing.T) {
	var r *SubFlowRegistry
	r.Register("x", &SubFlowFrame{}) // 不应 panic
	if _, ok := r.Get("x"); ok {
		t.Error("nil.Get should return false")
	}
	if r.Unregister("x") {
		t.Error("nil.Unregister should return false")
	}
	if r.Count() != 0 {
		t.Error("nil.Count should return 0")
	}
	if r.List() != nil {
		t.Error("nil.List should return nil")
	}
	r.Clear() // 不应 panic
}

// ============================================================================
// GlobalSubFlowRegistry 测试
// ============================================================================

// Check: GlobalSubFlowRegistry 返回非 nil
func TestGlobalSubFlowRegistry(t *testing.T) {
	if GlobalSubFlowRegistry() == nil {
		t.Fatal("GlobalSubFlowRegistry returned nil")
	}
}

// Check: SetGlobalSubFlowRegistry 替换全局注册表
func TestSetGlobalSubFlowRegistry(t *testing.T) {
	original := GlobalSubFlowRegistry()
	defer SetGlobalSubFlowRegistry(original)

	newReg := NewSubFlowRegistry()
	newReg.Register("test_frame", &SubFlowFrame{ParentID: "p", CreatedAt: time.Now()})
	SetGlobalSubFlowRegistry(newReg)

	if GlobalSubFlowRegistry() != newReg {
		t.Error("global registry not replaced")
	}
	if GlobalSubFlowRegistry().Count() != 1 {
		t.Errorf("Count = %d, want 1", GlobalSubFlowRegistry().Count())
	}
}

// Check: ResetGlobalSubFlowRegistry 清空全局注册表
func TestResetGlobalSubFlowRegistry(t *testing.T) {
	defer ResetGlobalSubFlowRegistry()

	GlobalSubFlowRegistry().Register("temp", &SubFlowFrame{ParentID: "p", CreatedAt: time.Now()})
	ResetGlobalSubFlowRegistry()
	if GlobalSubFlowRegistry().Count() != 0 {
		t.Errorf("Count after reset = %d, want 0", GlobalSubFlowRegistry().Count())
	}
}

// ============================================================================
// ChildFlow 接口 + TriggerFlow.RunChild 测试
// ============================================================================

// Check: TriggerFlow 实现 ChildFlow 接口
func TestTriggerFlowImplementsChildFlow(t *testing.T) {
	f := NewTriggerFlow[string, string, string]()
	f.AddOperator(&Operator{ID: "op-1", Kind: OpChunk, Name: "c1"})

	var cf ChildFlow = f
	out, err := cf.RunChild("hello")
	if err != nil {
		t.Fatalf("RunChild failed: %v", err)
	}
	if out != "hello" {
		t.Errorf("RunChild output = %v, want hello", out)
	}
}

// Check: RunChild 输入类型不匹配时返回 error
func TestTriggerFlowRunChildTypeMismatch(t *testing.T) {
	f := NewTriggerFlow[string, string, string]()
	f.AddOperator(&Operator{ID: "op-1", Kind: OpChunk, Name: "c1"})

	var cf ChildFlow = f
	_, err := cf.RunChild(123) // int 不是 string
	if err == nil {
		t.Error("expected error for type mismatch")
	}
	if !strings.Contains(err.Error(), "type mismatch") {
		t.Errorf("error should mention 'type mismatch', got: %v", err)
	}
}

// Check: nil TriggerFlow 的 RunChild 返回 error
func TestTriggerFlowRunChildNil(t *testing.T) {
	var f *TriggerFlow[string, string, string]
	_, err := f.RunChild("x")
	if err == nil {
		t.Error("expected error for nil TriggerFlow")
	}
}

// ============================================================================
// SubFlowHandler - 通过 child_flow_executor 测试
// ============================================================================

// Check: SubFlowHandler 通过 child_flow_executor 调用并返回结果
func TestSubFlowHandlerExecutorWithFrame(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	h := &SubFlowHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpSubFlow,
			Name: "sub",
			Options: map[string]any{
				"child_flow_executor": func(in any) (any, error) {
					return in.(string) + "_processed", nil
				},
			},
		},
		Input: "data",
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "data_processed" {
		t.Errorf("output = %v, want data_processed", out)
	}

	// 验证 frame 被注册
	if GlobalSubFlowRegistry().Count() != 1 {
		t.Errorf("registry Count = %d, want 1", GlobalSubFlowRegistry().Count())
	}
}

// Check: SubFlowHandler executor 错误传播
func TestSubFlowHandlerExecutorErrorWithFrame(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	h := &SubFlowHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpSubFlow,
			Name: "sub",
			Options: map[string]any{
				"child_flow_executor": func(in any) (any, error) {
					return nil, errors.New("child flow failed")
				},
			},
		},
		Input: "data",
	}
	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// 验证 frame 被注册并记录错误
	frames := GlobalSubFlowRegistry().List()
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	if frames[0].Error == "" {
		t.Error("frame should record error message")
	}
	if !frames[0].IsCompleted() {
		t.Error("frame should be marked completed (even on error)")
	}
}

// Check: SubFlowHandler 无 executor 也无 child_flow 时原样返回 Input
func TestSubFlowHandlerNoChildWithFrame(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	h := &SubFlowHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:      "op-1",
			Kind:    OpSubFlow,
			Name:    "sub",
			Options: map[string]any{},
		},
		Input: "passthrough",
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "passthrough" {
		t.Errorf("output = %v, want passthrough", out)
	}

	// 骨架模式不应注册 frame
	if GlobalSubFlowRegistry().Count() != 0 {
		t.Errorf("registry Count = %d, want 0 (skeleton mode)", GlobalSubFlowRegistry().Count())
	}
}

// ============================================================================
// SubFlowHandler - 通过 child_flow (ChildFlow 接口) 测试
// ============================================================================

// Check: SubFlowHandler 通过 child_flow 接口调用 TriggerFlow
func TestSubFlowHandlerChildFlowInterface(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	// 构造子流
	child := NewTriggerFlow[int, int, int]()
	child.AddOperator(&Operator{ID: "child-op-1", Kind: OpChunk, Name: "child-chunk"})

	h := &SubFlowHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "parent-op-1",
			Kind: OpSubFlow,
			Name: "parent-sub",
			Options: map[string]any{
				"child_flow": child,
			},
		},
		Input: 42,
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != 42 {
		t.Errorf("output = %v, want 42", out)
	}

	// 验证 frame 被注册
	if GlobalSubFlowRegistry().Count() != 1 {
		t.Errorf("registry Count = %d, want 1", GlobalSubFlowRegistry().Count())
	}
}

// Check: child_flow 与 child_flow_executor 同时存在时优先使用 executor
func TestSubFlowHandlerExecutorTakesPrecedence(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	child := NewTriggerFlow[int, int, int]()
	child.AddOperator(&Operator{ID: "child-op-1", Kind: OpChunk, Name: "child-chunk"})

	h := &SubFlowHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "parent-op-1",
			Kind: OpSubFlow,
			Name: "parent-sub",
			Options: map[string]any{
				"child_flow": child,
				"child_flow_executor": func(in any) (any, error) {
					return in.(int) * 100, nil
				},
			},
		},
		Input: 5,
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != 500 {
		t.Errorf("output = %v, want 500 (executor takes precedence)", out)
	}
}

// Check: child_flow 类型不匹配（非 ChildFlow 接口）时退回到骨架模式
func TestSubFlowHandlerChildFlowInvalidType(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	h := &SubFlowHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpSubFlow,
			Name: "sub",
			Options: map[string]any{
				"child_flow": "not a child flow",
			},
		},
		Input: "passthrough",
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	// 无有效子流引用，骨架模式原样返回
	if out != "passthrough" {
		t.Errorf("output = %v, want passthrough", out)
	}
	if GlobalSubFlowRegistry().Count() != 0 {
		t.Errorf("registry Count = %d, want 0 (skeleton)", GlobalSubFlowRegistry().Count())
	}
}

// ============================================================================
// SubFlowFrame 完整生命周期测试
// ============================================================================

// Check: frame 在成功执行后正确记录结果和 CompletedAt
func TestSubFlowFrameSuccessLifecycle(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	h := &SubFlowHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpSubFlow,
			Name: "sub",
			Options: map[string]any{
				"child_flow_executor": func(in any) (any, error) {
					return fmt.Sprintf("result_for_%v", in), nil
				},
			},
		},
		Input: "input",
	}

	before := time.Now()
	out, err := h.Execute(oc)
	after := time.Now()

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if out != "result_for_input" {
		t.Errorf("output = %v, want result_for_input", out)
	}

	frames := GlobalSubFlowRegistry().List()
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	f := frames[0]

	// 验证 frame 字段
	if f.ParentID != "op-1" {
		t.Errorf("ParentID = %q, want op-1", f.ParentID)
	}
	if !f.CreatedAt.After(before.Add(-time.Millisecond)) || !f.CreatedAt.Before(after.Add(time.Millisecond)) {
		t.Errorf("CreatedAt = %v, expected between %v and %v", f.CreatedAt, before, after)
	}
	if f.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
	if !f.IsCompleted() {
		t.Error("frame should be completed")
	}
	if f.Result != "result_for_input" {
		t.Errorf("Result = %v, want result_for_input", f.Result)
	}
	if f.Error != "" {
		t.Errorf("Error = %q, want empty", f.Error)
	}
}

// Check: frame 在失败执行后正确记录错误
func TestSubFlowFrameErrorLifecycle(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	h := &SubFlowHandler{}
	oc := &OperatorContext{
		Ctx: context.Background(),
		Operator: &Operator{
			ID:   "op-1",
			Kind: OpSubFlow,
			Name: "sub",
			Options: map[string]any{
				"child_flow_executor": func(in any) (any, error) {
					return nil, errors.New("intentional failure")
				},
			},
		},
		Input: "input",
	}

	_, err := h.Execute(oc)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	frames := GlobalSubFlowRegistry().List()
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
	f := frames[0]

	if !f.IsCompleted() {
		t.Error("frame should be completed (even on error)")
	}
	if f.Result != nil {
		t.Errorf("Result = %v, want nil on error", f.Result)
	}
	if f.Error != "intentional failure" {
		t.Errorf("Error = %q, want 'intentional failure'", f.Error)
	}
}

// ============================================================================
// 嵌套子流测试
// ============================================================================

// Check: 多级嵌套子流（父 → 子 → 孙）
func TestSubFlowNested(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	// 构造孙子流：x -> x * 2
	grandchild := NewTriggerFlow[int, int, int]()
	grandchild.AddOperator(&Operator{
		ID:   "grandchild-op",
		Kind: OpSubFlow,
		Name: "grandchild-sub",
		Options: map[string]any{
			"child_flow_executor": func(in any) (any, error) {
				return in.(int) * 2, nil
			},
		},
	})

	// 构造子流：调用孙子流
	child := NewTriggerFlow[int, int, int]()
	child.AddOperator(&Operator{
		ID:   "child-op",
		Kind: OpSubFlow,
		Name: "child-sub",
		Options: map[string]any{
			"child_flow": grandchild,
		},
	})

	// 父流：调用子流
	parent := NewTriggerFlow[int, int, int]()
	parent.AddOperator(&Operator{
		ID:   "parent-op",
		Kind: OpSubFlow,
		Name: "parent-sub",
		Options: map[string]any{
			"child_flow": child,
		},
	})

	out, err := parent.Run(5)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	// 5 -> 5*2 = 10
	if out != 10 {
		t.Errorf("output = %v, want 10", out)
	}

	// 应注册 3 个 frame（parent/child/grandchild 各一个）
	if GlobalSubFlowRegistry().Count() != 3 {
		t.Errorf("registry Count = %d, want 3", GlobalSubFlowRegistry().Count())
	}
}

// ============================================================================
// 并发子流测试
// ============================================================================

// Check: 并发执行多个子流不产生 race，frame 全部注册
func TestSubFlowConcurrent(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	h := &SubFlowHandler{}

	const n = 10
	var wg sync.WaitGroup
	results := make([]int, n)
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		idx, val := i, i+1
		go func() {
			defer wg.Done()
			oc := &OperatorContext{
				Ctx: context.Background(),
				Operator: &Operator{
					ID:   fmt.Sprintf("op-%d", idx),
					Kind: OpSubFlow,
					Name: fmt.Sprintf("sub-%d", idx),
					Options: map[string]any{
						"child_flow_executor": func(in any) (any, error) {
							return in.(int) * 10, nil
						},
					},
				},
				Input: val,
			}
			out, err := h.Execute(oc)
			if err != nil {
				errs[idx] = err
				return
			}
			results[idx] = out.(int)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d failed: %v", i, err)
		}
	}
	for i, want := range []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100} {
		if results[i] != want {
			t.Errorf("results[%d] = %d, want %d", i, results[i], want)
		}
	}

	if GlobalSubFlowRegistry().Count() != n {
		t.Errorf("registry Count = %d, want %d", GlobalSubFlowRegistry().Count(), n)
	}
}

// ============================================================================
// nil 安全
// ============================================================================

// Check: SubFlowHandler 对 nil OperatorContext 返回 error
func TestSubFlowHandlerNilContext(t *testing.T) {
	h := &SubFlowHandler{}
	_, err := h.Execute(nil)
	if err == nil {
		t.Error("expected error for nil context")
	}
}

// Check: SubFlowHandler 对 nil Operator 走骨架模式（原样返回 Input）
func TestSubFlowHandlerNilOperator(t *testing.T) {
	ResetGlobalSubFlowRegistry()
	defer ResetGlobalSubFlowRegistry()

	h := &SubFlowHandler{}
	oc := &OperatorContext{
		Ctx:      context.Background(),
		Operator: nil,
		Input:    "x",
	}
	out, err := h.Execute(oc)
	if err != nil {
		t.Errorf("expected no error for nil operator (skeleton mode), got: %v", err)
	}
	if out != "x" {
		t.Errorf("output = %v, want x (passthrough)", out)
	}
}
