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

//go:build ignore

// 示例：如何使用 Flow 的暂停/恢复/检查点机制
//
// 运行: go run example_flow_pause.go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/inferglow/flow"
)

func main() {
	ctx := context.Background()

	// --- 示例 1: 简单暂停/恢复 ---
	fmt.Println("=== Example 1: Simple Pause / Resume ===")

	stepA := flow.NewStep("stepA", func(ctx context.Context, input any) (any, error) {
		return fmt.Sprintf("(%s -> A)", input), nil
	}).Build()

	stepB := flow.NewStep("stepB", func(ctx context.Context, input any) (any, error) {
		return fmt.Sprintf("(%s -> B)", input), nil
	}).Build()

	stepC := flow.NewStep("stepC", func(ctx context.Context, input any) (any, error) {
		return fmt.Sprintf("(%s -> C)", input), nil
	}).Build()

	pauseFlow := flow.NewFlow().
		AddStep(stepA).
		To(stepB).
		To(stepC).
		Build()

	// 先完整执行一次，展示完整流程
	fullExec := pauseFlow.Execute(ctx, "start")
	fmt.Printf("Full execution result: %v\n", fullExec.State.Result)
	fmt.Printf("Full execution status: %s\n", fullExec.State.Status)
	for _, entry := range fullExec.State.StepLog {
		fmt.Printf("  - %s: %v\n", entry.StepName, entry.Output)
	}
	fmt.Println()

	// 模拟：stepA 执行完后暂停，然后从 stepB 恢复执行
	partialExec := &flow.Execution{
		State: flow.ExecutionState{
			Status: flow.StatusRunning,
			StepLog: map[string]*flow.StepLogEntry{
				"stepA": {StepName: "stepA", Input: "hello", Output: "(hello -> A)"},
			},
			StepExecLog: []string{"stepA"},
			Result:      "(hello -> A)",
		},
	}

	pp := partialExec.Pause("需要人工审核")
	fmt.Printf("Paused at step: %s\n", pp.StepName)
	fmt.Printf("Pause reason: 需要人工审核\n")
	fmt.Printf("PausePoint input: %v\n", pp.Input)
	fmt.Println()

	// 从暂停点恢复，从 stepB 继续执行
	resumedExec := pauseFlow.Resume(ctx, pp, "(hello -> A)")
	fmt.Printf("Resumed execution result: %v\n", resumedExec.State.Result)
	fmt.Printf("Resumed execution status: %s\n", resumedExec.State.Status)
	for _, entry := range resumedExec.State.StepLog {
		fmt.Printf("  - %s: %v\n", entry.StepName, entry.Output)
	}
	fmt.Println()

	// --- 示例 2: 检查点持久化 ---
	fmt.Println("=== Example 2: Checkpoint Persistence ===")

	// 创建临时目录作为检查点存储
	checkpointDir, err := os.MkdirTemp("", "flow-checkpoint-*")
	if err != nil {
		fmt.Printf("创建临时目录失败: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(checkpointDir) // 示例结束清理

	store := flow.NewFileCheckpointStore(checkpointDir)

	step1 := flow.NewStep("step1", func(ctx context.Context, input any) (any, error) {
		return fmt.Sprintf("(%s -> step1)", input), nil
	}).Build()

	step2 := flow.NewStep("step2", func(ctx context.Context, input any) (any, error) {
		return fmt.Sprintf("(%s -> step2)", input), nil
	}).Build()

	step3 := flow.NewStep("step3", func(ctx context.Context, input any) (any, error) {
		return fmt.Sprintf("(%s -> step3)", input), nil
	}).Build()

	checkpointFlow := flow.NewFlow().
		AddStep(step1).
		To(step2).
		To(step3).
		WithOptions(
			flow.WithAutoCheckpoint(store),
			flow.WithCheckPointID("my-checkpoint"),
		).
		Build()

	// 模拟：step1 执行完毕，然后暂停（自动保存检查点）
	checkpointExec := &flow.Execution{
		State: flow.ExecutionState{
			Status: flow.StatusRunning,
			StepLog: map[string]*flow.StepLogEntry{
				"step1": {StepName: "step1", Input: "data", Output: "(data -> step1)"},
			},
			StepExecLog: []string{"step1"},
			Result:      "(data -> step1)",
		},
	}

	ckPP := checkpointFlow.Pause(checkpointExec, "crash recovery")
	fmt.Printf("Paused at: %s\n", ckPP.StepName)
	fmt.Printf("Checkpoint saved with ID: %s\n", ckPP.CheckpointID)
	fmt.Printf("Checkpoint directory: %s\n", checkpointDir)
	fmt.Println()

	// 验证检查点文件已持久化
	checkpointPath := filepath.Join(checkpointDir, ckPP.CheckpointID+".json")
	if _, err := os.Stat(checkpointPath); err == nil {
		fmt.Printf("Checkpoint file found: %s\n", checkpointPath)
	}
	fmt.Println()

	// 模拟崩溃：丢弃所有内存引用
	checkpointExec = nil
	checkpointFlow = nil

	// 重建 Flow 并从检查点恢复
	recoveredFlow := flow.NewFlow().
		AddStep(step1).
		To(step2).
		To(step3).
		WithOptions(
			flow.WithAutoCheckpoint(store),
			flow.WithCheckPointID("my-checkpoint"),
		).
		Build()

	// 加载检查点
	recovered, err := recoveredFlow.LoadCheckpoint()
	if err != nil {
		fmt.Printf("加载检查点失败: %v\n", err)
		os.Exit(1)
	}
	if recovered == nil {
		fmt.Println("未找到检查点")
		os.Exit(1)
	}

	fmt.Printf("Loaded checkpoint: paused at %s\n", recovered.PausedAt)
	fmt.Printf("Checkpoint execution ID: %s\n", recovered.ExecutionID)
	fmt.Println()

	// 从快照恢复执行，从 step2 继续
	recoveredExec := recoveredFlow.ResumeFromSnapshot(recovered)
	fmt.Printf("Recovered execution result: %v\n", recoveredExec.State.Result)
	fmt.Printf("Recovered execution status: %s\n", recoveredExec.State.Status)
	fmt.Println("Steps executed (including restored history):")
	for _, name := range recoveredExec.State.StepExecLog {
		if entry, ok := recoveredExec.State.StepLog[name]; ok {
			fmt.Printf("  - %s: %v\n", name, entry.Output)
		}
	}

	fmt.Println("\n=== All examples completed ===")
}