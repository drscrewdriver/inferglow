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

// 示例：如何通过声明式 FlowDef 构建和执行 Flow
//
// 运行: go run example_flowdef.go
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/inferglow/flow"
	"github.com/inferglow/flow/flowdef"
	"github.com/inferglow/flow/stage"
)

func main() {
	ctx := context.Background()

	// --- 步骤 1: 创建 stage.Registry 并注册两个 stage ---
	fmt.Println("=== Step 1: Register stages ===")

	stages := stage.NewRegistry()

	// "greet" stage: 接收 name 字段，返回问候语
	stages.Register("greet", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		name, _ := in["name"].(string)
		return stage.Outputs{"greeting": fmt.Sprintf("Hello, %s!", name)}, nil
	})
	fmt.Println("  Registered: greet")

	// "uppercase" stage: 接收 greeting 字段，返回大写版本
	stages.Register("uppercase", func(ctx context.Context, in stage.Inputs, fctx flow.FlowContext) (stage.Outputs, error) {
		greeting, _ := in["greeting"].(string)
		return stage.Outputs{"shout": strings.ToUpper(greeting)}, nil
	})
	fmt.Println("  Registered: uppercase")
	fmt.Println()

	// --- 步骤 2: 以编程方式构建 FlowDef ---
	fmt.Println("=== Step 2: Build FlowDef programmatically ===")

	def := &flowdef.FlowDef{
		APIVersion: "flowdef/v1",
		Kind:       "Flow",
		Metadata: flowdef.Metadata{
			Name:        "greet-and-shout",
			Version:     "1.0.0",
			Description: "A simple two-step flow that greets and then uppercases the greeting",
		},
		Spec: flowdef.Spec{
			Inputs: []flowdef.InputDef{
				{Name: "name", Type: "string", Required: true},
			},
			Steps: []flowdef.StepDef{
				{
					Name:     "greet",
					Operator: "stage",
					Stage:    "greet",
				},
				{
					Name:      "uppercase",
					Operator:  "stage",
					Stage:     "uppercase",
					DependsOn: []string{"greet"},
				},
			},
			Outputs: map[string]string{
				"greeting": "greet.greeting",
				"shout":    "uppercase.shout",
			},
		},
	}

	fmt.Printf("  Flow name: %s\n", def.Metadata.Name)
	fmt.Printf("  Steps: %d\n", len(def.Spec.Steps))
	for _, s := range def.Spec.Steps {
		fmt.Printf("    - %s (stage=%s, depends_on=%v)\n", s.Name, s.Stage, s.DependsOn)
	}
	fmt.Println()

	// --- 步骤 3: 验证 FlowDef ---
	fmt.Println("=== Step 3: Validate FlowDef ===")

	if err := flowdef.Validate(def); err != nil {
		fmt.Printf("  Validation failed: %v\n", err)
		return
	}
	fmt.Println("  Validation passed")
	fmt.Println()

	// --- 步骤 4: 使用 Adapter 将 FlowDef 转换为可执行的 Flow ---
	fmt.Println("=== Step 4: Convert FlowDef to Flow via Adapter ===")

	adapter := flowdef.NewAdapter(stages)
	f, err := adapter.ToFlow(def)
	if err != nil {
		fmt.Printf("  ToFlow error: %v\n", err)
		return
	}
	fmt.Println("  Flow built successfully")
	fmt.Println()

	// --- 步骤 5: 执行 Flow ---
	fmt.Println("=== Step 5: Execute the Flow ===")

	input := map[string]any{"name": "InferGlow"}
	fmt.Printf("  Input: %v\n", input)

	exec := f.Execute(ctx, input)

	fmt.Printf("  Status: %s\n", exec.State.Status)
	fmt.Println()

	// --- 步骤 6: 打印结果 ---
	fmt.Println("=== Step 6: Results ===")

	if exec.State.Status != flow.StatusCompleted {
		fmt.Printf("  Flow failed with errors: %v\n", exec.State.Errors)
		return
	}

	result, ok := exec.State.Result.(map[string]any)
	if !ok {
		fmt.Println("  Unexpected result type")
		return
	}

	fmt.Printf("  Steps executed: %d\n", len(exec.State.StepLog))
	for _, entry := range exec.State.StepLog {
		fmt.Printf("    - %s: %v\n", entry.StepName, entry.Output)
	}
	fmt.Println()

	// 按步骤名访问结果
	greetResult := result["greet"].(map[string]any)
	fmt.Printf("  greet.greeting = %v\n", greetResult["greeting"])

	upperResult := result["uppercase"].(map[string]any)
	fmt.Printf("  uppercase.shout = %v\n", upperResult["shout"])

	fmt.Println()
	fmt.Println("=== Example completed ===")
}