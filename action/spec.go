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

package action

import "time"

// SideEffectLevel 副作用级别
type SideEffectLevel string

const (
	// SideEffectNone indicates an Action has no side effects.
	SideEffectNone SideEffectLevel = "none"
	// SideEffectRead indicates an Action only reads state without mutating it.
	SideEffectRead SideEffectLevel = "read"
	// SideEffectWrite indicates an Action writes or mutates local state.
	SideEffectWrite SideEffectLevel = "write"
	// SideEffectNetwork indicates an Action performs network I/O.
	SideEffectNetwork SideEffectLevel = "network"
	// SideEffectExec indicates an Action executes external processes.
	SideEffectExec SideEffectLevel = "exec"
)

// ActionPolicy defines execution constraints for an Action.
type ActionPolicy struct { //nolint:revive
	Timeout        time.Duration `json:"timeout"`
	TimeoutSeconds float64       `json:"timeout_seconds"`
	Retries        int           `json:"retries"`
	MaxRetries     int           `json:"max_retries"`
	RetryDelay     time.Duration `json:"retry_delay"`
	MaxOutputBytes int           `json:"max_output_bytes"`
	NetworkAccess  string        `json:"network_access"` // "inherit" | "enabled" | "disabled"
	ReadOnly       bool          `json:"read_only"`
	PathAllowlist  []string      `json:"path_allowlist"`
	PathDenylist   []string      `json:"path_denylist"`
}

// DefaultActionPolicy 默认策略
var DefaultActionPolicy = &ActionPolicy{
	Timeout:        60 * time.Second,
	TimeoutSeconds: 60,
	MaxOutputBytes: 1024 * 1024, // 1MB
	NetworkAccess:  "inherit",
	MaxRetries:     3,
}

// ActionSpec 完整 Action 规格，包含 14+ 字段
type ActionSpec struct { //nolint:revive
	ActionID           string          `json:"action_id"`
	Name               string          `json:"name"`
	Description        string          `json:"desc"`
	Kwargs             map[string]any  `json:"kwargs"`
	Returns            map[string]any  `json:"returns"`
	Tags               []string        `json:"tags"`
	DefaultPolicy      *ActionPolicy   `json:"default_policy"`
	SideEffectLevel    SideEffectLevel `json:"side_effect_level"`
	ApprovalRequired   bool            `json:"approval_required"`
	SandboxRequired    bool            `json:"sandbox_required"`
	ReplaySafe         bool            `json:"replay_safe"`
	ExposeToModel      bool            `json:"expose_to_model"`
	ExecutorType       string          `json:"executor_type"`
	ExecutionResources map[string]any  `json:"execution_resources"`
	Meta               map[string]any  `json:"meta"`
}

// DecisionAction 决策类型
type DecisionAction string

const (
	// DecisionPlan indicates the agent should plan the Action before executing.
	DecisionPlan DecisionAction = "plan"
	// DecisionExecute indicates the agent should execute the Action directly.
	DecisionExecute DecisionAction = "execute"
	// DecisionSkip indicates the agent should skip the Action.
	DecisionSkip DecisionAction = "skip"
)

// PlannedStep 规划步骤
type PlannedStep struct {
	ActionID string         `json:"action_id"`
	Kwargs   map[string]any `json:"kwargs"`
	Reason   string         `json:"reason"`
}

// ActionCall 单次 Action 调用
type ActionCall struct { //nolint:revive
	ActionCallID string         `json:"action_call_id"`
	ActionID     string         `json:"action_id"`
	ToolName     string         `json:"tool_name"`
	Kwargs       map[string]any `json:"kwargs"`
}

// ActionDecision Agent 对 Action 的决策
type ActionDecision struct { //nolint:revive
	ActionID    string         `json:"action_id"`
	Decision    DecisionAction `json:"decision"`
	Reason      string         `json:"reason"`
	PlannedStep *PlannedStep   `json:"planned_step"`
}
