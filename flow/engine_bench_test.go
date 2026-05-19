package flow

import (
	"context"
	"strings"
	"testing"
)

// G1-07.6: Flow 执行 Benchmark
// 覆盖：不同步数（1/5/20）、含分支的 Flow、错误处理路径。

// appendSuffix 构造一个向 input 追加 suffix 的 StepFunc。
func appendSuffix(suffix string) StepFunc {
	return func(ctx context.Context, input any) (any, error) {
		return input.(string) + suffix, nil
	}
}

// BenchmarkFlowExecuteSequential 测试顺序步链执行的吞吐，不同步数。
func BenchmarkFlowExecuteSequential(b *testing.B) {
	cases := []struct {
		name  string
		steps int
	}{
		{"steps_1", 1},
		{"steps_5", 5},
		{"steps_20", 20},
	}
	ctx := context.Background()
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			fb := NewFlow()
			first := NewStep("s0", appendSuffix("0")).Build()
			fb.AddStep(first)
			for i := 1; i < c.steps; i++ {
				fb.To(NewStep("s"+itoaFlowBench(i), appendSuffix(string(rune('0'+i)))).Build())
			}
			flow := fb.Build()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = flow.Execute(ctx, "start")
			}
		})
	}
}

// BenchmarkFlowExecuteSingleStep 测试单步 Flow 的基线开销。
func BenchmarkFlowExecuteSingleStep(b *testing.B) {
	step := NewStep("only", func(ctx context.Context, input any) (any, error) {
		return input.(string) + "-done", nil
	}).Build()
	flow := NewFlow().AddStep(step).Build()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = flow.Execute(ctx, "input")
	}
}

// BenchmarkFlowExecuteWithBranch 测试含条件分支的 Flow 执行。
func BenchmarkFlowExecuteWithBranch(b *testing.B) {
	mainStep := NewStep("main", appendSuffix("-main")).Build()
	trueStep := NewStep("onTrue", appendSuffix("-true")).Build()
	falseStep := NewStep("onFalse", appendSuffix("-false")).Build()
	flow := NewFlow().
		AddStep(mainStep).
		If(func(out any) bool { return true }, trueStep, falseStep).
		Build()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = flow.Execute(ctx, "start")
	}
}

// BenchmarkFlowExecuteLargePayload 测试大 payload 传递的开销。
func BenchmarkFlowExecuteLargePayload(b *testing.B) {
	payload := strings.Repeat("x", 4096)
	step := NewStep("passthrough", func(ctx context.Context, input any) (any, error) {
		return input, nil
	}).Build()
	flow := NewFlow().AddStep(step).Build()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = flow.Execute(ctx, payload)
	}
}

// itoaFlowBench 是 flow benchmark 专用的简易 int→string，避免与其他 test 文件冲突。
func itoaFlowBench(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
