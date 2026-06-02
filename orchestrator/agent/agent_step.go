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

package agent

import (
	"context"
	"fmt"

	"github.com/inferglow/flow"
)

// AgentStepConfig 配置 AgentStepFunc 的行为。
// 返回的 StepFunc 从 ctx 中提取 FlowContext，调用 RunAgent 执行 PLAN→EXECUTE 循环，
// 使 Flow 中的任意 step 可以嵌入 Agent 级别的多轮智能决策能力。
type AgentStepConfig struct {
	// SystemPrompt 是 Agent 循环的系统提示词。
	SystemPrompt string
	// MaxRounds 是最大迭代轮数。0 = 使用默认值 10。
	MaxRounds int
	// SessionIsolation 为 true 时，内嵌 Agent 使用独立 Session。
	// 默认为 false（共享外层 Session）。
	SessionIsolation bool
	// InputKey 是从 step input 中提取 userMessage 的键名。
	// 当 input 是 map[string]any 时使用该键取值；否则整个 input 转为字符串作为 userMessage。
	InputKey string
	// OutputKey 是 Agent 回复写入 step output 的键名。
	// 空字符串表示直接返回字符串；非空时写入 map[string]any（保留原 input 的其他键）。
	OutputKey string
}

// NewAgentStepFunc 创建一个 StepFunc，在 step 内部运行完整的多轮 Agent 循环。
// 返回的 StepFunc 从 ctx 中提取 FlowContext，调用 RunAgent 执行 PLAN→EXECUTE 循环。
// 这允许 Flow 中的任意 step 嵌入 Agent 级别的多轮智能决策能力。
//
// 用法示例：
//
//	flow.NewStep("modify_code", agent.NewAgentStepFunc(agent.AgentStepConfig{
//	    SystemPrompt: "You are a code modification agent...",
//	    MaxRounds:    5,
//	    InputKey:     "task_description",
//	    OutputKey:    "modified_code",
//	})).Build()
func NewAgentStepFunc(cfg AgentStepConfig) flow.StepFunc {
	return func(ctx context.Context, input any) (any, error) {
		fc, ok := flow.FlowContextFrom(ctx)
		if !ok {
			return nil, fmt.Errorf("agent step: FlowContext not found in context")
		}

		userMessage := extractInputString(input, cfg.InputKey)

		response, err := fc.RunAgent(ctx, userMessage, cfg.SystemPrompt, &flow.AgentRunOptions{
			MaxRounds:        cfg.MaxRounds,
			SessionIsolation: cfg.SessionIsolation,
		})
		if err != nil {
			return nil, fmt.Errorf("agent step %q failed: %w", "agent_loop", err)
		}

		return wrapOutput(input, cfg.OutputKey, response), nil
	}
}

// ParallelAgentStepConfig 配置并行多子 Agent 执行。
// 每个子 Agent 由一个 SubTaskSpec 定义，共享相同的 step input。
type ParallelAgentStepConfig struct {
	// SubTasks 是并行执行的子任务列表。
	SubTasks []SubTaskSpec
}

// SubTaskSpec 描述 ParallelAgentStepFunc 中的一个子 Agent 任务。
type SubTaskSpec struct {
	// Label 是子 Agent 的标识（用于日志/审计区分）。
	Label string
	// SystemPrompt 是该子 Agent 的系统提示词。
	SystemPrompt string
	// MaxRounds 该子 Agent 的最大迭代轮数。0 = 默认 10。
	MaxRounds int
	// InputKey 是从 step input 中提取该子任务 userMessage 的键名。
	InputKey string
	// OutputKey 是该子任务结果写入 step output 的键名。
	// 必须非空（并行场景下各子任务结果通过 key 区分）。
	OutputKey string
}

// NewParallelAgentStepFunc 创建一个 StepFunc，在 step 内部并行运行多个子 Agent。
// 当前实现为顺序降级执行（每个子任务依次调用 RunAgentParallel）；
// Phase 2 升级为真并行后，调用方代码无需修改。
//
// 用法示例：
//
//	flow.NewStep("parallel_review", agent.NewParallelAgentStepFunc(agent.ParallelAgentStepConfig{
//	    SubTasks: []agent.SubTaskSpec{
//	        {Label: "reviewer", SystemPrompt: "Review...", MaxRounds: 3, OutputKey: "review"},
//	        {Label: "tester",   SystemPrompt: "Test...",   MaxRounds: 2, OutputKey: "test"},
//	    },
//	})).Build()
func NewParallelAgentStepFunc(cfg ParallelAgentStepConfig) flow.StepFunc {
	return func(ctx context.Context, input any) (any, error) {
		fc, ok := flow.FlowContextFrom(ctx)
		if !ok {
			return nil, fmt.Errorf("parallel agent step: FlowContext not found in context")
		}

		// 构建 AgentSubTask 列表
		agents := make([]flow.AgentSubTask, len(cfg.SubTasks))
		for i, sub := range cfg.SubTasks {
			agents[i] = flow.AgentSubTask{
				Label:        sub.Label,
				UserMessage:  extractInputString(input, sub.InputKey),
				SystemPrompt: sub.SystemPrompt,
				MaxRounds:    sub.MaxRounds,
			}
		}

		results, err := fc.RunAgentParallel(ctx, agents)
		if err != nil {
			return nil, fmt.Errorf("parallel agent step failed: %w", err)
		}

		// 合并结果到 output map
		out := copyInputMap(input)
		for i, sub := range cfg.SubTasks {
			key := sub.OutputKey
			if key == "" {
				key = sub.Label // 降级：无 OutputKey 时用 Label
			}
			out[key] = results[i]
		}
		return out, nil
	}
}

// extractInputString 从 input 中提取字符串。
// 当 input 是 map[string]any 且 key 非空时，取 input[key] 转为字符串；
// 否则将整个 input 通过 fmt.Sprint 转为字符串。
func extractInputString(input any, key string) string {
	if key != "" {
		if m, ok := input.(map[string]any); ok {
			if v, ok := m[key]; ok {
				return fmt.Sprint(v)
			}
		}
	}
	return fmt.Sprint(input)
}

// wrapOutput 构造 step output。
// outputKey 为空时直接返回 value；非空时将 value 写入 map（保留原 input 的其他键）。
func wrapOutput(input any, outputKey string, value any) any {
	if outputKey == "" {
		return value
	}
	out := copyInputMap(input)
	out[outputKey] = value
	return out
}

// copyInputMap 将 input 浅拷贝为 map[string]any。
// input 不是 map 时返回空 map。
func copyInputMap(input any) map[string]any {
	if m, ok := input.(map[string]any); ok {
		out := make(map[string]any, len(m)+1)
		for k, v := range m {
			out[k] = v
		}
		return out
	}
	return make(map[string]any, 1)
}
