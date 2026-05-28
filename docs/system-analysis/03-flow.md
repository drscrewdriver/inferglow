# 03 · flow 模块

## 一、职责

`flow` 模块（`github.com/inferglow/flow`）提供两层编排引擎：

1. **线性 Flow 引擎**（[engine.go](../../flow/engine.go)）：简单管道，从起始步骤沿边遍历到终点，支持条件分支。
2. **TriggerFlow 事件驱动引擎**（[triggerflow_blueprint.go](../../flow/triggerflow_blueprint.go)）：基于 `SignalNet` 信号网络 + 13 种算子（Operator）的复杂编排，支持 Pause/Resume/Persist。

依赖 `schema` 模块（Step 可选绑定 `OutputSchema` 做输出校验）。

## 二、核心类型

### 2.1 线性 Flow

#### Flow 与构建器（[flow.go](../../flow/flow.go)）

```go
type Edge struct {
    From string
    To   string
}

type Branch struct {
    From      string
    Cond      func(any) bool
    TrueStep  *Step
    FalseStep *Step
}

type Flow struct {
    steps          map[string]*Step
    edges          []Edge
    branches       []Branch
    startStep      *Step
    // Checkpoint 相关
    autoCheckpoint  bool
    checkpointStore CheckpointStore
    serializer      Serializer
    checkPointID    string
    // ...
}

type FlowBuilder struct { ... }

func NewFlow() *FlowBuilder
func (fb *FlowBuilder) AddStep(step *Step) *FlowBuilder    // 添加步骤
func (fb *FlowBuilder) To(step *Step) *FlowBuilder          // 连接边
func (fb *FlowBuilder) If(cond func(any) bool, trueStep, falseStep *Step) *FlowBuilder
func (fb *FlowBuilder) WithOptions(opts ...FlowOption) *FlowBuilder
func (fb *FlowBuilder) Build() *Flow
```

#### Step（[step.go](../../flow/step.go)）

```go
type StepFunc func(ctx context.Context, input any) (any, error)

type Step struct {
    Name   string
    Func   StepFunc
    Schema *schema.OutputSchema   // 可选：执行后校验输出
}

func NewStep(name string, fn StepFunc) *StepBuilder
func (b *StepBuilder) WithOutputSchema(s *schema.OutputSchema) *StepBuilder
func (b *StepBuilder) Build() *Step
```

> **关键设计**：`Schema` 是可选指针。不设置时 Step 正常执行无校验；设置时执行后额外校验输出格式。Flow 不强依赖 schema 也能运行。

#### 执行状态（[engine.go](../../flow/engine.go)）

```go
type ExecutionStatus string
const (
    StatusCreated   ExecutionStatus = "created"
    StatusRunning   ExecutionStatus = "running"
    StatusCompleted ExecutionStatus = "completed"
    StatusFailed    ExecutionStatus = "failed"
    StatusPaused    ExecutionStatus = "paused"
)

type ExecutionState struct {
    Status      ExecutionStatus
    Result      any
    Errors      []error
    StepLog     map[string]*StepLogEntry
    StepExecLog []string              // 按执行顺序记录步骤名
}

type Execution struct {
    State ExecutionState
}

func (f *Flow) Execute(ctx context.Context, input any) *Execution
func (f *Flow) Resume(ctx context.Context, snapshot *ExecutionSnapshot) (*Execution, error)
```

### 2.2 TriggerFlow 与算子

#### 13 种算子（[operator.go](../../flow/operator.go) L33-L60）

| `OperatorKind` | 常量 | 职责 |
|----------------|------|------|
| `chunk` | `OpChunk` | 将单个输入拆分为流式块 |
| `signal_gate` | `OpSignalGate` | 等待信号到达后放行 |
| `batch_fanout` | `OpBatchFanout` | 单输入扇出到并行批次分支 |
| `batch_collect` | `OpBatchCollect` | 收集批次扇出结果 |
| `for_each_split` | `OpForEachSplit` | 按 iterable 元素拆分为多分支 |
| `for_each_collect` | `OpForEachCollect` | 收集 for-each 结果 |
| `match_route` | `OpMatchRoute` | 派发到首个匹配的 case |
| `match_case` | `OpMatchCase` | match 中的单个 case |
| `match_collect` | `OpMatchCollect` | 收集 matched cases 结果 |
| `collect_branch` | `OpCollectBranch` | 收集条件分支输出 |
| `intervention_point` | `OpIntervention` | 暂停等待外部干预 |
| `sub_flow` | `OpSubFlow` | 调用另一个 Flow 作为子流程 |
| `result_sink` | `OpResultSink` | 收集 Flow 终端结果 |

```go
type Operator struct {
    ID            string
    Kind          OperatorKind
    Name          string
    ListenSignals []string         // 监听的信号
    EmitSignals   []string         // 发射的信号
    Options       map[string]any
    HandlerRef    *CallableRef     // 可序列化的 handler 引用
}

// 算子 Handler 接口
type OperatorHandler interface {
    Kind() OperatorKind
    Execute(oc *OperatorContext) (any, error)
}

type OperatorContext struct {
    Ctx        context.Context
    Operator   *Operator
    Input      any
    SignalNet  *SignalNet
    EmitSignal func(signal Signal)
}
```

#### 算子注册中心

```go
type OperatorRegistry struct {
    mu        sync.RWMutex
    operators map[string]*Operator
}

func NewOperatorRegistry() *OperatorRegistry
func (r *OperatorRegistry) Register(op *Operator) error
func (r *OperatorRegistry) Get(id string) (*Operator, error)
func (r *OperatorRegistry) List() []*Operator
func (r *OperatorRegistry) FindByListenSignal(signal string) []*Operator
```

### 2.3 生命周期与 Pause/Resume

| 文件 | 内容 |
|------|------|
| [lifecycle.go](../../flow/lifecycle.go) | `LifecycleMachine` 状态机：created→running→paused/resumed→completed/failed |
| [pause.go](../../flow/pause.go) | `Pause()` / `Resume()` 实现，基于 `ExecutionSnapshot` |
| [persistence.go](../../flow/persistence.go) | `CheckpointStore` 接口 + `ExecutionSnapshot` 序列化 |
| [signal.go](../../flow/signal.go) | `Signal` / `SignalNet` 信号网络 |
| [subflow.go](../../flow/subflow.go) | 子流程嵌套执行 |
| [goroutine_pool.go](../../flow/goroutine_pool.go) | goroutine 池（控制并发度） |
| [inputsource.go](../../flow/inputsource.go) | 输入源抽象 |

## 三、关键调用链

### 链 A：线性 Flow 执行

```
flow := NewFlow().
    AddStep(NewStep("fetch", fetchFunc).Build()).
    To(NewStep("parse", parseFunc).Build()).
    To(NewStep("save", saveFunc).Build()).
    Build()

execution := flow.Execute(ctx, input)
    │
    ├──[1] findStartStep()
    │       找到没有入边的起始步骤 (fetch)
    │
    ├──[2] 循环遍历步骤链:
    │       for {
    │           step := f.steps[currentStepName]
    │           output, err := step.Func(ctx, currentInput)
    │           exec.State.StepLog[step.Name] = {Input, Output, Duration, Error}
    │           exec.State.StepExecLog = append(..., step.Name)
    │           // 沿 edges 找下一个步骤
    │           currentStepName = nextStep
    │           // 处理 branches: cond(output) → trueStep/falseStep
    │       }
    │
    └──[3] 返回 Execution{State: {Status: Completed, Result: lastOutput}}
```

### 链 B：TriggerFlow 信号驱动

```
SignalNet (信号网络)
    │
    ├── 发射信号 "input_ready"
    │
    ▼
OperatorRegistry.FindByListenSignal("input_ready")
    │
    ├── 找到 OpChunk 算子
    │     │
    │     └── handler.Execute(oc)
    │           ├── 将 input 拆分为 chunks
    │           └── oc.EmitSignal(Signal{Name:"chunk_ready", Data:chunk})
    │
    ▼
FindByListenSignal("chunk_ready")
    │
    ├── 找到 OpBatchFanout
    │     ├── 并发扇出到 N 个分支
    │     └── 每个分支发射 "branch_done"
    │
    ▼
OpBatchCollect
    ├── 收集所有 branch_done 结果
    └── EmitSignal("batch_complete")
    │
    ▼
OpResultSink
    └── 写入终端结果
```

### 链 C：Pause / Resume

```
flow.Execute(ctx, input)
    │
    ├── 检测到 InterventionPoint 算子
    │     │
    │     └── 生成 ExecutionSnapshot{
    │             LastStepName: "intervention_1",
    │             StepExecLog:  ["fetch","parse","intervention_1"],
    │             IntermediateData: ...,
    │         }
    │
    └── State.Status = StatusPaused
          返回 Execution (paused)

// 外部干预完成后:
flow.Resume(ctx, snapshot)
    │
    ├── 从 snapshot.LastStepName 恢复
    ├── 跳过已执行的步骤 (依据 StepExecLog)
    ├── 注入干预结果作为下一步输入
    └── 继续执行后续步骤
```

## 四、并发安全

`Flow` 使用 `sync.RWMutex` 保护 `steps`/`edges`/`branches`/`startStep`：
- `Execute` / `Resume` 加**读锁**（允许多读并发）
- `FlowBuilder.AddStep` / `To` / `If` 加**写锁**（独占）

> **注意**：构建完成后不应再修改 Flow，否则运行中的 Execute 可能 panic。

## 五、与 orchestrator 的关系

`flow` 模块是编排引擎，**被 `orchestrator` 直接依赖**（orchestrator/go.mod 中 require flow）。orchestrator 通过 `FlowContext` 接口将横切能力（Action 执行、Model 调用、Session 读写、Audit）注入到 flow 步骤中，实现两种运行模式：
- **oneshot 模式（默认）**：未配置 flow 时，Agent.Run 走 `executeLoop`（PLAN→EXECUTE 循环）
- **flow 编排模式**：通过 `WithFlow(f)` RunOption 设置 flow 后，Agent.Run 走 `executeFlow`，flow 步骤可通过 `flow.FlowContextFrom(ctx)` 获取 FlowContext
- `OpSubFlow` 算子允许 Flow 嵌套调用另一个 Flow

两者是互补的编排能力：Flow 面向**确定性流程**，Agent 面向**LLM 驱动的非确定性决策**。通过 FlowContext 注入，flow 步骤可复用 orchestrator 的全部横切能力。
