package action

import "time"

// SideEffectLevel 副作用级别
type SideEffectLevel string

const (
	SideEffectNone    SideEffectLevel = "none"
	SideEffectRead    SideEffectLevel = "read"
	SideEffectWrite   SideEffectLevel = "write"
	SideEffectNetwork SideEffectLevel = "network"
)

// ActionPolicy defines execution constraints for an Action.
type ActionPolicy struct {
	Timeout       time.Duration `json:"timeout"`
	TimeoutSeconds float64      `json:"timeout_seconds"`
	Retries       int           `json:"retries"`
	MaxRetries    int           `json:"max_retries"`
	RetryDelay    time.Duration `json:"retry_delay"`
	MaxOutputBytes int           `json:"max_output_bytes"`
	NetworkAccess string        `json:"network_access"` // "inherit" | "enabled" | "disabled"
	ReadOnly      bool          `json:"read_only"`
	PathAllowlist []string      `json:"path_allowlist"`
	PathDenylist  []string      `json:"path_denylist"`
}

// DefaultActionPolicy 默认策略
var DefaultActionPolicy = &ActionPolicy{
	Timeout:       60 * time.Second,
	TimeoutSeconds: 60,
	MaxOutputBytes: 1024 * 1024, // 1MB
	NetworkAccess:  "inherit",
	MaxRetries:     3,
}

// ActionSpec 完整 Action 规格，包含 14+ 字段
type ActionSpec struct {
	ActionID           string            `json:"action_id"`
	Name               string            `json:"name"`
	Description        string            `json:"desc"`
	Kwargs             map[string]any    `json:"kwargs"`
	Returns            map[string]any    `json:"returns"`
	Tags               []string          `json:"tags"`
	DefaultPolicy      *ActionPolicy     `json:"default_policy"`
	SideEffectLevel    SideEffectLevel   `json:"side_effect_level"`
	ApprovalRequired   bool              `json:"approval_required"`
	SandboxRequired    bool              `json:"sandbox_required"`
	ReplaySafe         bool              `json:"replay_safe"`
	ExposeToModel      bool              `json:"expose_to_model"`
	ExecutorType       string            `json:"executor_type"`
	ExecutionResources map[string]any    `json:"execution_resources"`
	Meta               map[string]any    `json:"meta"`
}

// DecisionAction 决策类型
type DecisionAction string

const (
	DecisionPlan    DecisionAction = "plan"
	DecisionExecute DecisionAction = "execute"
	DecisionSkip    DecisionAction = "skip"
)

// PlannedStep 规划步骤
type PlannedStep struct {
	ActionID string            `json:"action_id"`
	Kwargs   map[string]any    `json:"kwargs"`
	Reason   string            `json:"reason"`
}

// ActionCall 单次 Action 调用
type ActionCall struct {
	ActionCallID string         `json:"action_call_id"`
	ActionID     string         `json:"action_id"`
	ToolName     string         `json:"tool_name"`
	Kwargs       map[string]any `json:"kwargs"`
}

// ActionDecision Agent 对 Action 的决策
type ActionDecision struct {
	ActionID    string         `json:"action_id"`
	Decision    DecisionAction `json:"decision"`
	Reason      string         `json:"reason"`
	PlannedStep *PlannedStep   `json:"planned_step"`
}
