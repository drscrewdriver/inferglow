// go:build ignore
//go:build ignore

// 示例：如何使用 flow 模块编排步骤
//
// 运行: go run example_flow.go
package main

import (
	"context"
	"fmt"

	"github.com/inferglow/flow"
)

func main() {
	ctx := context.Background()

	// --- 示例 1: 简单的线性流程 ---
	fmt.Println("=== Example 1: Simple Linear Flow ===")

	parseStep := flow.NewStep("parse", func(ctx context.Context, input any) (any, error) {
		text := input.(string)
		return fmt.Sprintf("Parsed: %s", text), nil
	}).Build()

	validateStep := flow.NewStep("validate", func(ctx context.Context, input any) (any, error) {
		text := input.(string)
		valid := len(text) > 5
		return fmt.Sprintf("Valid: %v", valid), nil
	}).Build()

	formatStep := flow.NewStep("format", func(ctx context.Context, input any) (any, error) {
		text := input.(string)
		return fmt.Sprintf("Formatted: [%s]", text), nil
	}).Build()

	simpleFlow := flow.NewFlow().
		AddStep(parseStep).
		To(validateStep).
		To(formatStep).
		Build()

	exe := simpleFlow.Execute(ctx, "Hello World!")
	fmt.Printf("Input: Hello World!\n")
	fmt.Printf("Status: %s\n", exe.State.Status)
	fmt.Printf("Result: %v\n", exe.State.Result)
	fmt.Printf("Steps executed: %d\n", len(exe.State.StepLog))
	for _, entry := range exe.State.StepLog {
		fmt.Printf("  - %s: %v\n", entry.StepName, entry.Output)
	}
	fmt.Println()

	// --- 示例 2: 带条件分支的流程 ---
	fmt.Println("=== Example 2: Flow with Conditional Branch ===")

	analyzeStep := flow.NewStep("analyze", func(ctx context.Context, input any) (any, error) {
		num := input.(int)
		if num >= 0 {
			return map[string]any{"result": "positive", "value": num}, nil
		}
		return map[string]any{"result": "negative", "value": num}, nil
	}).Build()

	handlePositive := flow.NewStep("handle_positive", func(ctx context.Context, input any) (any, error) {
		data := input.(map[string]any)
		return fmt.Sprintf("Positive: %v", data["value"]), nil
	}).Build()

	handleNegative := flow.NewStep("handle_negative", func(ctx context.Context, input any) (any, error) {
		data := input.(map[string]any)
		return fmt.Sprintf("Negative: %v", data["value"]), nil
	}).Build()

	branchFlow := flow.NewFlow().
		AddStep(analyzeStep).
		If(func(output any) bool {
			data := output.(map[string]any)
			return data["result"] == "positive"
		}, handlePositive, handleNegative).
		Build()

	// 测试正数
	posExe := branchFlow.Execute(ctx, 42)
	fmt.Printf("Input: 42\n")
	fmt.Printf("Status: %s\n", posExe.State.Status)
	for _, entry := range posExe.State.StepLog {
		fmt.Printf("  - %s: %v\n", entry.StepName, entry.Output)
	}

	// 测试负数
	negExe := branchFlow.Execute(ctx, -10)
	fmt.Printf("\nInput: -10\n")
	fmt.Printf("Status: %s\n", negExe.State.Status)
	for _, entry := range negExe.State.StepLog {
		fmt.Printf("  - %s: %v\n", entry.StepName, entry.Output)
	}
	fmt.Println()

	// --- 示例 3: 带 Schema 校验的步骤 ---
	fmt.Println("=== Example 3: Flow with Output Schema ===")

	type WeatherResult struct {
		City     string  `json:"city"`
		Temp     float64 `json:"temp"`
		Humidity int     `json:"humidity,omitempty"`
	}

	wrapperStep := flow.NewStep("weather_wrapper", func(ctx context.Context, input any) (any, error) {
		// 模拟返回 WeatherResult
		return WeatherResult{
			City:     "Beijing",
			Temp:     25.5,
			Humidity: 60,
		}, nil
	}).Build()

	wrapExe := wrapperStep
	_ = wrapExe
	fmt.Printf("Step with Schema created (Schema field available for validation)\n")

	fmt.Println("\n=== All examples completed ===")
}
