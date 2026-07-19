# Inferglow Agently 等价补齐 — 完整实施计划

## 现状总结

Inferglow 已有 14 个 Go 模块，具备 model/schema/flow/action/session/sandbox/audit/security/observability/builtins/components/workspace/orchestrator。orchestrator 层已有 Agent 类（PLAN→EXECUTE 循环）、ActionDispatcher、TurnLoop、FlowContext 集成。

**缺失的 10 个等价组件：**

| # | 组件 | Agently 对应 | 优先级 |
|---|------|-------------|--------|
| 1 | ExecutionResource | `core/operation/ExecutionResource/` | P0 |
| 2 | RecordStore | `builtins/agent_extensions/RecordStoreExtension.py` | P0 |
| 3 | PolicyApproval | `core/operation/PolicyApproval/` | P1 |
| 4 | TaskContext | `core/context/` | P1 |
| 5 | TaskWorkspace 增强 | `core/TaskWorkspace/` | P1 |
| 6 | DynamicTask/TaskDAG | `core/orchestration/TaskDAG/` | P1 |
| 7 | SkillLibrary | `core/application/SkillLibrary/` | P2 |
| 8 | ActionFlow DAG | `builtins/plugins/ActionFlow/` | P2 |
| 9 | Blocks 框架 | `core/application/Blocks/` + `builtins/blocks/` | P2 |
| 10 | AgentOrchestrator 增强 | `builtins/plugins/AgentOrchestrator/` | P2 |

---

## Phase 1: 基础层 — ExecutionResource + RecordStore

### 1.1 ExecutionResource（执行资源生命周期管理）

**新建模块**: `inferglow/resource/`（独立 Go module）

**文件结构**:
```
resource/
├── go.mod
├── provider.go        — ResourceProvider 接口 + Resource 接口
├── manager.go         — ResourceManager（capability 匹配 + handle 复用）
├── handle.go          — ResourceHandle（生命周期 + 健康检查）
├── types.go           — Requirement, ResourceConfig, ResourceStatus
├── builtin_providers.go — 内置 Provider 注册（bash/python/sqlite 占位）
├── provider_test.go
├── manager_test.go
└── handle_test.go
```

**核心接口设计**:
```go
// Resource 代表一个有生命周期的执行资源实例
type Resource interface {
    ID() string
    Type() string  // "bash", "python", "sqlite", "browser", "mcp"
    Execute(ctx context.Context, cmd any) (*ResourceResult, error)
    HealthCheck(ctx context.Context) error
    Close() error
}

// ResourceProvider 创建和管理特定类型的资源
type ResourceProvider interface {
    Type() string
    Capabilities() []string
    Create(ctx context.Context, config ResourceConfig) (Resource, error)
    Probe(ctx context.Context) error  // 检测可用性
}

// ResourceManager 统一管理所有资源
type ResourceManager struct { ... }
func (m *ResourceManager) Declare(req Requirement) (*ResourceHandle, error)
func (m *ResourceManager) Ensure(ctx context.Context, req Requirement) (*ResourceHandle, error)
func (m *ResourceManager) Release(handle *ResourceHandle) error
func (m *ResourceManager) ReleaseScope(scope string) error
func (m *ResourceManager) Inspect(handle *ResourceHandle) ResourceStatus
func (m *ResourceManager) List() []*ResourceHandle
func (m *ResourceManager) RegisterProvider(p ResourceProvider) error
```

**go.mod 依赖**: 无内部依赖（纯 Go 模块）

### 1.2 RecordStore（统一执行记录存储）

**新建包**: `orchestrator/recordstore/`（在 orchestrator 模块内）

**文件结构**:
```
orchestrator/recordstore/
├── store.go       — RecordStore 接口 + 内存实现
├── record.go      — Record, Checkpoint, Snapshot, Event 类型
├── scope.go       — Scope 管理（agent/session/execution 维度隔离）
├── persistence.go — JSON/YAML 序列化
├── store_test.go
└── scope_test.go
```

**核心接口设计**:
```go
type RecordStore interface {
    AppendRecord(rec Record) error
    GetRecord(id string) (*Record, error)
    SaveCheckpoint(executionID string, cp *Checkpoint) error
    LoadCheckpoint(executionID string) (*Checkpoint, error)
    SaveSnapshot(executionID string, snap *Snapshot) error
    LoadSnapshot(executionID string) (*Snapshot, error)
    AppendEvent(executionID string, evt Event) error
    QueryEvents(filter EventFilter) ([]Event, error)
}

type Scope struct {
    AgentID     string
    SessionID   string
    ExecutionID string
}

type Record struct {
    ID        string
    Scope     Scope
    Kind      string  // "action_result", "decision", "model_response", "flow_step"
    Timestamp time.Time
    Data      any
    Metadata  map[string]string
}
```

---

## Phase 2: 安全层 — PolicyApproval

### 2.1 PolicyApproval（策略审批框架）

**新建模块**: `inferglow/approval/`（独立 Go module）

**文件结构**:
```
approval/
├── go.mod
├── manager.go      — PolicyApprovalManager（可插拔 handler）
├── handler.go      — ApprovalHandler 接口
├── decision.go     — Decision, Request, DecisionStatus
├── builtin_handlers.go — 内置 handler: auto_approve, fail_closed, input_timeout_fail
├── access_policy.go    — 访问控制策略合并
├── manager_test.go
└── handler_test.go
```

**核心接口设计**:
```go
type DecisionStatus string
const (
    DecisionApproved DecisionStatus = "approved"
    DecisionDenied   DecisionStatus = "denied"
    DecisionPending  DecisionStatus = "pending"
)

type Request struct {
    RequestID  string
    Source     string  // "action", "resource", "task_node"
    Capability string
    Subject    string
    Risk       string  // "none", "low", "medium", "high"
    Payload    map[string]any
    Policy     *AccessPolicy
}

type Decision struct {
    Status   DecisionStatus
    Approved bool
    Reason   string
    Handler  string
}

type ApprovalHandler interface {
    Name() string
    Resolve(ctx context.Context, req *Request) (*Decision, error)
}

type PolicyApprovalManager struct { ... }
func (m *PolicyApprovalManager) RegisterHandler(h ApprovalHandler) error
func (m *PolicyApprovalManager) SetDefaultHandler(name string) error
func (m *PolicyApprovalManager) Resolve(ctx context.Context, req *Request) (*Decision, error)
```

**go.mod 依赖**: 无内部依赖

---

## Phase 3: 上下文层 — TaskContext + TaskWorkspace 增强

### 3.1 TaskContext（任务上下文聚合）

**新建包**: `orchestrator/taskcontext/`

**文件结构**:
```
orchestrator/taskcontext/
├── context.go       — TaskContext 聚合器
├── reader.go        — ContextReader（budget 控制 + 渐进披露）
├── source.go        — ContextSource 接口
├── budget.go        — ContextBudget（max_chars, max_blocks）
├── selection.go     — SemanticSelector 接口
├── context_test.go
└── reader_test.go
```

**核心接口设计**:
```go
type ContextSource interface {
    EnumerateDescriptors(ctx context.Context, cursor string, limit int) (*DescriptorPage, error)
    ReadExact(ctx context.Context, ref SourceRef, maxChars int) (*ContextSourceRead, error)
}

type TaskContext struct { ... }
func NewTaskContext() *TaskContext
func (tc *TaskContext) Attach(source ContextSource) *TaskContext
func (tc *TaskContext) Put(entry ContextEntry) *TaskContext
func (tc *TaskContext) Remove(key string) *TaskContext
func (tc *TaskContext) Snapshot() *TaskContextSnapshot
func (tc *TaskContext) Reader(opts ...ReaderOption) *ContextReader

type ContextReader struct { ... }
func (r *ContextReader) Read(ctx context.Context) (*ContextConsumption, error)
func (r *ContextReader) Refresh()
func (r *ContextReader) SetBudget(budget ContextBudget)

type ContextBudget struct {
    MaxChars      int
    MaxBlocks     int
    MaxBlockChars int
}
```

### 3.2 TaskWorkspace 增强（执行访问控制 + 内容身份）

**增强现有模块**: `workspace/`

**新增文件**:
```
workspace/
├── execution_access.go  — ExecutionAccessGrant + 生命周期
├── identity.go          — ContentIdentity（SHA-256 版本化追踪）
├── context_source.go    — WorkspaceContextSource（实现 ContextSource 接口）
├── execution_access_test.go
├── identity_test.go
└── context_source_test.go
```

**核心新增**:
```go
type ExecutionAccessGrant struct {
    ExecutionID string
    WorkspaceID string
    ReadPaths   []string
    WritePaths  []string
    ExpiresAt   time.Time
}

type ContentIdentity struct {
    Path           string
    Digest         string  // SHA-256
    Size           int64
    ContentVersion string
}

type IdentityCatalog struct { ... }
func (c *IdentityCatalog) ObservePath(path string) (*ContentIdentity, error)
func (c *IdentityCatalog) GetVersion(path string) string
```

---

## Phase 4: 编排层 — DynamicTask/TaskDAG + SkillLibrary + ActionFlow

### 4.1 DynamicTask/TaskDAG（模型生成的 DAG 执行）

**新建包**: `orchestrator/taskdag/`

**文件结构**:
```
orchestrator/taskdag/
├── dag.go          — TaskDAG, TaskNode, Dependency 类型
├── validator.go    — 拓扑排序 + 循环检测 + 可选任务裁剪
├── resolver.go     — Handler 解析器（按 binding → id → kind 解析）
├── executor.go     — TaskDAGExecutor（编译到 flow 执行）
├── compiler.go     — 编译 DAG 为 flow.Flow
├── context.go      — TaskDAGContext（运行时上下文）
├── helpers.go      — 辅助函数
├── dag_test.go
├── validator_test.go
├── executor_test.go
└── compiler_test.go
```

**核心接口设计**:
```go
type TaskDAG struct {
    ID       string
    Tasks    []TaskNode
    Outputs  map[string]string  // semantic_outputs
}

type TaskNode struct {
    ID        string
    Kind      string  // "model", "action", "local"
    Binding   string
    DependsOn []string
    Optional  bool
    Input     map[string]any
}

type Handler interface {
    Execute(ctx context.Context, tctx *TaskDAGContext) (any, error)
}

type TaskDAGExecutor struct { ... }
func NewTaskDAGExecutor(resolver *Resolver) *TaskDAGExecutor
func (e *TaskDAGExecutor) Run(ctx context.Context, dag *TaskDAG, input any) (map[string]any, error)
func (e *TaskDAGExecutor) Compile(dag *TaskDAG) (*flow.Flow, error)

type TaskDAGContext struct {
    DAG               *TaskDAG
    CurrentNode       *TaskNode
    NodeInput         map[string]any
    DAGInput          any
    DependencyResults map[string]any
}
```

### 4.2 SkillLibrary（技能库管理）

**新建包**: `orchestrator/skill/`

**文件结构**:
```
orchestrator/skill/
├── library.go       — SkillLibrary（安装 + 版本管理）
├── package.go       — SkillPackage, SkillRevision 类型
├── source.go        — SourceProvider 接口（本地路径、git）
├── binding.go       — SkillBinding（Agent 级技能声明）
├── local_source.go  — 本地路径 SourceProvider 实现
├── library_test.go
├── binding_test.go
└── local_source_test.go
```

**核心接口设计**:
```go
type SkillLibrary struct { ... }
func NewSkillLibrary(root string) *SkillLibrary
func (lib *SkillLibrary) Install(source string, scope string) (*SkillPackageRevision, error)
func (lib *SkillLibrary) GetRevision(source string, revision string) (*SkillPackageRevision, error)
func (lib *SkillLibrary) ListInstalled() []*SkillPackageRevision

type SkillBinding struct {
    Skills   []SkillRef
    Mode     SkillMode  // "model_decision" | "required"
}

type SkillRef struct {
    Source   string
    Revision string  // 空 = 最新
}
```

### 4.3 ActionFlow DAG（并发动作执行）

**新建文件**: `orchestrator/actionruntime/dag_flow.go`

**核心设计**:
```go
// DAGActionFlow 将 PLAN→EXECUTE 循环中的 action 执行从串行升级为 DAG 并行
type DAGActionFlow struct {
    registry    *action.ActionRegistry
    dispatcher  *ActionDispatcher
    maxRounds   int
    concurrency int
}

func NewDAGActionFlow(reg *action.ActionRegistry, disp *ActionDispatcher) *DAGActionFlow
func (f *DAGActionFlow) Run(ctx context.Context, calls []ActionCall) ([]*action.ActionResult, error)
// 内部: 分析 calls 间的依赖 → 构建 DAG → 拓扑排序 → 按层并发执行
```

---

## Phase 5: 高级层 — Blocks + AgentOrchestrator 增强

### 5.1 Blocks 框架（结构化可组合执行块）

**新建包**: `orchestrator/blocks/`

**文件结构**:
```
orchestrator/blocks/
├── block.go        — FlowBlock 接口
├── registry.go     — BlockRegistry（注册 + 查找）
├── compiler.go     — 编译 Block 到 flow.Operator
├── builtin_blocks.go — 内置块: ReasonBlock, ActBlock, IntentBlock
├── block_test.go
└── compiler_test.go
```

**核心接口设计**:
```go
type FlowBlock interface {
    Name() string
    BuildOperators(ctx context.Context, blueprint *BlockBlueprint) ([]*flow.Operator, error)
    Execute(ctx context.Context, input any) (any, error)
}

type BlockBlueprint struct {
    Blocks []BlockRef
}

type BlockRef struct {
    BlockName string
    Config    map[string]any
}

// 内置块
type ReasonBlock struct { ModelReq model.ModelRequester; Schema *schema.OutputSchema }
type ActBlock struct { Registry *action.ActionRegistry; AllowedActions []string }
type IntentBlock struct { ModelReq model.ModelRequester; IntentSchema *schema.OutputSchema }
```

### 5.2 AgentOrchestrator 增强

**增强现有文件**: `orchestrator/agent/agent.go`, `engine.go`

**新增能力**:
1. **ExecutionResource 集成**: Agent 持有 `*resource.ResourceManager`，通过 `WithResourceManager` 注入
2. **RecordStore 集成**: Agent 持有 `recordstore.RecordStore`，通过 `WithRecordStore` 注入
3. **PolicyApproval 集成**: Agent 持有 `*approval.PolicyApprovalManager`
4. **TaskContext 集成**: executeLoop 中构建 TaskContext 供 action 使用
5. **SkillLibrary 集成**: Agent 持有 `*skill.SkillLibrary`，通过 `WithSkillLibrary` 注入
6. **多执行策略**: 新增 `ExecutionStrategy` 接口，支持 `DirectStrategy`（当前 PLAN→EXECUTE）和 `TaskDAGStrategy`

**新增 RunOption**:
```go
func WithResourceManager(m *resource.ResourceManager) RunOption
func WithRecordStore(rs recordstore.RecordStore) RunOption
func WithPolicyApproval(m *approval.PolicyApprovalManager) RunOption
func WithSkillLibrary(lib *skill.SkillLibrary) RunOption
func WithExecutionStrategy(s ExecutionStrategy) RunOption
```

**新增文件**:
```
orchestrator/agent/
├── resource_ext.go      — ResourceManager 集成
├── recordstore_ext.go   — RecordStore 集成
├── approval_ext.go      — PolicyApproval 集成
├── skill_ext.go         — SkillLibrary 集成
├── strategy.go          — ExecutionStrategy 接口 + DirectStrategy/TaskDAGStrategy
└── 对应 _test.go 文件
```

---

## 依赖关系

```
Phase 1: ExecutionResource + RecordStore (无依赖)
    ↓
Phase 2: PolicyApproval (无依赖)
    ↓
Phase 3: TaskContext + TaskWorkspace 增强 (依赖 Phase 1 的 Resource)
    ↓
Phase 4: DynamicTask/TaskDAG + SkillLibrary + ActionFlow DAG
         (依赖 Phase 1/2/3)
    ↓
Phase 5: Blocks + AgentOrchestrator 增强
         (依赖所有 Phase)
```

## 代码规范

- 所有文件使用标准 MIT 版权头（Copyright 2026 InferGlow Authors）
- 新模块使用独立 go.mod + replace 指令引用兄弟模块
- 接口定义在消费方，实现在提供方
- 编译期接口检查: `var _ Interface = (*Impl)(nil)`
- 函数式选项: `With*` 前缀
- 哨兵错误: `var Err* = errors.New(...)`
- 表驱动测试，标准 testing 包
- 所有导出符号必须有 GoDoc 注释

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| 模块数量膨胀（14→17+） | ExecutionResource/Approval 独立模块；其余放在 orchestrator 子包 |
| 循环依赖 | 严格单向依赖：resource/approval 无内部依赖；orchestrator 消费它们 |
| 接口设计过度 | 先实现最小可用接口，后续按需扩展 |
| 测试覆盖下降 | 每个新包必须同步编写单元测试，`make test` 验证 |
| 与现有 Agent API 不兼容 | 所有增强通过 With* 可选注入，零注入 = 零变化 = 向后兼容 |

## 被拒绝的方案

1. **单一大模块方案**: 所有新组件放一个 `agently/` 模块 — 违反现有 multi-module 模式，增加编译耦合
2. **Python 风格直接翻译**: 逐行翻译 Agently Python 代码 — Go 的并发模型和类型系统不同，应利用 Go 优势
3. **修改现有模块接口**: 为集成而修改 flow/action/session — 破坏稳定性，应通过适配层桥接
4. **先做 Blocks 再做 TaskDAG**: Blocks 依赖 TaskDAG 做复杂编排，应先有 TaskDAG 基础

## 关键文件清单

| 文件 | 作用 |
|------|------|
| `resource/provider.go` | ExecutionResource 核心接口 |
| `resource/manager.go` | 资源生命周期管理 |
| `orchestrator/recordstore/store.go` | 统一记录存储 |
| `approval/manager.go` | 策略审批框架 |
| `orchestrator/taskcontext/context.go` | 任务上下文聚合 |
| `orchestrator/taskdag/executor.go` | DAG 任务执行 |
| `orchestrator/blocks/block.go` | 结构化执行块 |
| `orchestrator/agent/strategy.go` | 多执行策略 |
| `orchestrator/agent/agent.go` | Agent 增强集成点 |
| `workspace/execution_access.go` | 执行访问控制 |
