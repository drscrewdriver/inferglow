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

// 示例：如何使用 team 模块编排多 Agent 协作
//
// 运行: go run example_team.go
package main

import (
	"context"
	"fmt"

	"github.com/inferglow/orchestrator/team"
)

// echoRunner is a mock AgentRunner that echoes the task with a role prefix.
type echoRunner struct {
	role string
}

func (e *echoRunner) Run(ctx context.Context, userMessage string) (string, error) {
	return fmt.Sprintf("[%s] processed: %s", e.role, userMessage), nil
}

func main() {
	ctx := context.Background()

	// --- 示例: 多 Agent 协作 (planner → coder → reviewer) ---
	fmt.Println("=== Multi-Agent Team Collaboration ===")

	planner := &echoRunner{role: "planner"}
	coder := &echoRunner{role: "coder"}
	reviewer := &echoRunner{role: "reviewer"}

	members := []team.Member{
		{
			Agent:   planner,
			Role:    "planner",
			Handoff: []string{"coder"},
		},
		{
			Agent:   coder,
			Role:    "coder",
			Handoff: []string{"reviewer"},
		},
		{
			Agent:   reviewer,
			Role:    "reviewer",
			Handoff: []string{},
		},
	}

	coordinator := team.NewCoordinator(members)

	task := "Build a REST API endpoint for user management"
	result, err := coordinator.Round(ctx, task)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Task: %s\n", task)
	fmt.Printf("Rounds: %d\n", result.Rounds)
	fmt.Printf("Final response: %s\n", result.FinalResponse)
	fmt.Println("\nMember outputs:")
	for role, output := range result.MemberOutputs {
		fmt.Printf("  - %s: %s\n", role, output)
	}
	fmt.Println()

	// --- 示例: 带选项的 Coordinator ---
	fmt.Println("=== Coordinator with MaxRounds ===")

	task2 := "Design a caching strategy"
	coordinator2 := team.NewCoordinator(members, team.WithMaxRounds(5))

	result2, err := coordinator2.Round(ctx, task2)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Task: %s\n", task2)
	fmt.Printf("Rounds: %d\n", result2.Rounds)
	fmt.Printf("Final response: %s\n", result2.FinalResponse)
	fmt.Println()

	fmt.Println("=== All team examples completed ===")
}