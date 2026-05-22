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
	"sort"
	"testing"
)

// ============================================================================
// BUG-15 / F-MEDIUM-1: findStartStep 按 ID 排序取首个
//
// 现状（修复前）：findStartStep 在 f.startStep 为 nil 时遍历 f.steps map
// 寻找"非任何 edge 的 target"的 step。Go map 迭代顺序是随机的，因此
// 当存在多个候选 start step（都没有 incoming edge）时，findStartStep
// 每次调用可能返回不同的 step，导致 Execute 行为不确定。
//
// 修复要求：对候选 step 按 Name 排序，取首个（最小的 Name）。
// ============================================================================

// TestFindStartStep_DeterministicWhenMultipleCandidates 验证当 f.startStep
// 为 nil 且有多个候选 start step 时，findStartStep 返回结果是确定的
// （总是 Name 最小的那个）。
func TestFindStartStep_DeterministicWhenMultipleCandidates(t *testing.T) {
	// 构造一个 Flow，包含 3 个 step，没有任何 edge。
	// 所有 step 都没有 incoming edge，因此都是候选 start step。
	// 不通过 FlowBuilder.AddStep 构造，确保 f.startStep 为 nil。
	f := &Flow{
		steps: make(map[string]*Step),
	}
	// 故意以非字母序添加，凸显排序问题
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		stepName := name
		f.steps[stepName] = &Step{
			Name: stepName,
			Func: func(ctx context.Context, input any) (any, error) {
				return stepName, nil
			},
		}
	}

	// 多次调用 findStartStep，验证总是返回 "alpha"（字母序最小）
	const iterations = 100
	for i := 0; i < iterations; i++ {
		got := f.findStartStep()
		if got == nil {
			t.Fatalf("iter %d: findStartStep returned nil", i)
		}
		if got.Name != "alpha" {
			t.Fatalf("iter %d: findStartStep returned %q, want %q (sorted first)", i, got.Name, "alpha")
		}
	}
}

// TestFindStartStep_SortedByNamWhenNoExplicitStart 验证 findStartStep 在
// 没有 explicit startStep 时按 Name 排序取首个。
func TestFindStartStep_SortedByNameWhenNoExplicitStart(t *testing.T) {
	// 构造一个 Flow：stepB 没有 incoming edge，stepA 也没有 incoming edge
	// 但 stepA 的 Name 小于 stepB
	f := &Flow{
		steps: make(map[string]*Step),
	}
	for _, name := range []string{"zStep", "aStep", "mStep"} {
		stepName := name
		f.steps[stepName] = &Step{
			Name: stepName,
			Func: func(ctx context.Context, input any) (any, error) {
				return stepName, nil
			},
		}
	}

	got := f.findStartStep()
	if got == nil {
		t.Fatal("findStartStep returned nil")
	}
	// 期望返回字母序最小的 "aStep"
	if got.Name != "aStep" {
		t.Errorf("findStartStep returned %q, want %q", got.Name, "aStep")
	}
}

// TestFindStartStep_RespectsExplicitStartStep 验证当 f.startStep 显式设置时，
// findStartStep 直接返回它，不依赖排序。
func TestFindStartStep_RespectsExplicitStartStep(t *testing.T) {
	f := &Flow{
		steps: make(map[string]*Step),
	}
	zStep := &Step{Name: "zStep", Func: func(ctx context.Context, input any) (any, error) { return "z", nil }}
	aStep := &Step{Name: "aStep", Func: func(ctx context.Context, input any) (any, error) { return "a", nil }}
	f.steps["zStep"] = zStep
	f.steps["aStep"] = aStep
	// 显式设置 startStep 为 zStep（不是字母序最小）
	f.startStep = zStep

	got := f.findStartStep()
	if got == nil {
		t.Fatal("findStartStep returned nil")
	}
	if got.Name != "zStep" {
		t.Errorf("findStartStep returned %q, want %q (explicit startStep)", got.Name, "zStep")
	}
}

// TestFindStartStep_FallbackWhenAllAreTargets 验证当所有 step 都是 edge target
// 时（没有候选 start step），findStartStep 仍能按 Name 排序返回一个 step。
func TestFindStartStep_FallbackWhenAllAreTargets(t *testing.T) {
	f := &Flow{
		steps: make(map[string]*Step),
	}
	stepA := &Step{Name: "aaa", Func: func(ctx context.Context, input any) (any, error) { return "a", nil }}
	stepB := &Step{Name: "bbb", Func: func(ctx context.Context, input any) (any, error) { return "b", nil }}
	f.steps["aaa"] = stepA
	f.steps["bbb"] = stepB
	// 构造一个循环：aaa -> bbb -> aaa
	f.edges = []Edge{
		{From: "aaa", To: "bbb"},
		{From: "bbb", To: "aaa"},
	}

	// 所有 step 都是 target，进入 fallback 路径
	got := f.findStartStep()
	if got == nil {
		t.Fatal("findStartStep returned nil in fallback")
	}
	// 期望返回字母序最小的 "aaa"
	if got.Name != "aaa" {
		t.Errorf("findStartStep fallback returned %q, want %q", got.Name, "aaa")
	}
}

// TestFindStartStep_SortedCandidatesList 是一个辅助测试，验证候选列表确实
// 需要排序（暴露 map 迭代的非确定性）。
func TestFindStartStep_SortedCandidatesList(t *testing.T) {
	f := &Flow{
		steps: make(map[string]*Step),
	}
	names := []string{"echo", "delta", "alpha", "charlie", "bravo"}
	for _, name := range names {
		stepName := name
		f.steps[stepName] = &Step{
			Name: stepName,
			Func: func(ctx context.Context, input any) (any, error) {
				return stepName, nil
			},
		}
	}

	// 期望的排序顺序
	expected := make([]string, len(names))
	copy(expected, names)
	sort.Strings(expected)

	// 多次验证
	for i := 0; i < 50; i++ {
		got := f.findStartStep()
		if got == nil {
			t.Fatalf("iter %d: findStartStep returned nil", i)
		}
		if got.Name != expected[0] {
			t.Fatalf("iter %d: findStartStep returned %q, want %q", i, got.Name, expected[0])
		}
	}
}
