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

---

## 六、flow 模块内部双范式分析

当前 `flow/` 模块共 19,314 行（19 个非测试文件），内部实际包含**两套独立的编排范式**，共存于同一个 Go package 中：

### 6.1 范式 A：Step-based 线性执行

面向**确定性 DAG/管道**编排，由 `Flow` → `Step` → `Engine.Execute()` 构成。

| 文件 | 行数 | 核心类型 |
|------|------|----------|
| `flow.go` | 165 | `Flow`, `Edge`, `Branch`, `FlowBuilder` |
| `step.go` | 67 | `Step`, `StepFunc`, `StepBuilder` |
| `engine.go` | 305 | `Execution`, `ExecutionState`, `ExecutionStatus`, `Flow.Execute()` |
| `step_validate.go` | 52 | Step 校验逻辑 |
| `flow_context.go` | 137 | `FlowContext` 接口, `Span`, `SpanKind` |
| `pause.go` | 176 | `PausePoint`, Pause/Resume 基础 |

**总计约 900 行**，是 flow 模块中较小的一部分。

**外部消费者**：
- `orchestrator/agent/` — `WithFlow(f *flow.Flow)` 切换到 flow 编排模式
- `orchestrator/taskdag/compiler.go` — `Compile(dag *TaskDAG) *flow.Flow` 将 DAG 编译为 Flow
- `orchestrator/agent/flow_exec.go` — `executeFlow()` 执行 Flow 并处理 Pause/Resume

### 6.2 范式 B：Signal-driven 算子系统（TriggerFlow）

面向**事件驱动/信号传播**编排，由 `TriggerFlow` → `Operator` → `SignalNet` 构成。

| 文件 | 行数 | 核心类型 |
|------|------|----------|
| `operator.go` | 174 | `Operator`, `OperatorKind`, `OperatorHandler`, `OperatorContext`, `OperatorRegistry` |
| `operator_handlers.go` | 1013 | 12 种内置 Handler（Chunk/SignalGate/BatchFanout/BatchCollect/MatchRoute/ForEach/MatchCase/MatchCollect/CollectBranch/InterventionPoint/SubFlow/ResultSink） |
| `operator_runtime.go` | 78 | `OperatorRuntime` |
| `triggerflow_blueprint.go` | 573 | `TriggerFlowBlueprint`, `TriggerFlow<T,S,R>`, `FlowTriggerFlowDefinition` |
| `triggerflow_contract.go` | 183 | `Contract<T,S,R>` 泛型契约 |
| `signal.go` | 460 | `Signal`, `SignalNet`, `TriggerFlowRuntimeData`, `DynamicBinding` |
| `action_operator.go` | 79 | `OpAction`, `ActionOperatorHandler`（桥接 action/） |

**总计约 2,560 行**（不含基础设施）。

**外部消费者**：
- `orchestrator/blocks/` — 使用 `flow.Operator`, `flow.OpResultSink`, `flow.OpMatchRoute` 编译 Block 为算子图

### 6.3 范式 C：基础设施 / 横切能力

服务于上述两套范式的共享基础设施：

| 文件 | 行数 | 核心类型 | 服务对象 |
|------|------|----------|----------|
| `persistence.go` | 716 | `ExecutionPersistence`, `CheckpointStore`, `ExecutionSnapshot`, `CheckpointManager` | 主要服务 Flow（方法挂在 `*Flow` 上） |
| `subflow.go` | 543 | `SubFlowFrame`, `ChildFlow`, `SubFlowRegistry` | 两套范式均可使用 |
| `inputsource.go` | 572 | `InputSource`, `StaticValueSource`, `EnvSource`, `SessionSource`, `HTTPSource`, `MultiSource` | 主要服务 TriggerFlow |
| `lifecycle.go` | 254 | `LifecycleMachine`, `LifecycleState` | 两套范式共享 |
| `goroutine_pool.go` | 228 | `WorkerPool` | TriggerFlow 并发控制 |
| `callable_ref.go` | 308 | `HandlerRegistry` | Operator 可序列化引用 |

**总计约 2,621 行**。

### 6.4 耦合分析

两套范式在同一个 Go package 内存在**双向耦合**：

```
Step-based (A)                    Signal-driven (B)
─────────────                    ─────────────────
engine.go                         
  └── PauseSignalFrom(ctx) ──────▶ signal.go (PauseSignal)
                                  
action_operator.go                
  └── OpAction, ActionOperator ──▶ operator.go (OperatorKind, OperatorHandler)
  
                                  persistence.go
                                    └── func (f *Flow) SaveCheckpoint() ──▶ flow.go (*Flow)
                                    └── func (f *Flow) Pause() ──────────▶ flow.go (*Flow)
```

**关键耦合点**：
1. `persistence.go` 的 Checkpoint/Pause/Resume 方法直接挂在 `*Flow` 上 → 与 Step-based 强绑定
2. `engine.go` 引用 `PauseSignalFrom(ctx)` → Step-based 执行使用了 signal 包的信号机制
3. `action_operator.go` 是桥接层 → 将 action/ 暴露为 Operator，纯属于范式 B
4. `blocks/` 只使用 `Operator` 类型 → 与 Step-based 完全无关

### 6.5 当前架构的合理性

**现状合理之处**：
- 两套范式共享 `FlowContext`、`LifecycleMachine`、`CheckpointStore` 等基础设施，避免重复实现
- 同一个 Go package 允许类型互相引用，无需额外接口层
- `flow` 作为独立 Go module，对外提供完整的编排能力

**现状的问题**：
- `flow` 模块名过于笼统，实际包含两个截然不同的编排模型
- 仅使用 Step-based 的用户被迫携带 TriggerFlow 的 10K+ 行代码
- `blocks/` 只依赖 `Operator` 相关类型，但概念上属于不同层次
- 模块职责边界模糊：flow 既是「DAG 执行引擎」又是「信号驱动算子框架」

---

## 七、理想架构调整方案（脱离版本限制）

如果脱离当前版本兼容性限制，最科学的调整方式是**按范式拆分为独立模块**，保留共享基础设施层：

### 7.1 目标拓扑

```
当前:                                  理想:
                                       
flow/ (19K行, 双范式混合)              flow/ (~900行, 纯 Step-based)
├── flow.go                            ├── flow.go
├── step.go                            ├── step.go
├── engine.go                          ├── engine.go
├── operator.go          ◀── 耦合      ├── step_validate.go
├── operator_handlers.go               └── flow_context.go
├── triggerflow_blueprint.go           
├── signal.go                      triggerflow/ (~2.5K行, 纯信号驱动)
├── persistence.go         ◀── 跨范式  ├── operator.go
├── subflow.go                       ├── operator_handlers.go
├── inputsource.go                   ├── operator_runtime.go
├── lifecycle.go                     ├── triggerflow_blueprint.go
├── goroutine_pool.go                ├── triggerflow_contract.go
└── callable_ref.go                  ├── signal.go
                                     ├── action_operator.go
orchestrator/                        └── lifecycle.go
├── agent/ (uses Flow, Step)         
├── taskdag/ (compiles to Flow)   flowinfra/ (~2.6K行, 共享基础设施)
└── blocks/ (uses Operator)        ├── persistence.go
                                   ├── subflow.go
                                   ├── inputsource.go
                                   ├── goroutine_pool.go
                                   └── callable_ref.go
                                   
                               orchestrator/
                               ├── agent/ (uses flow/)
                               ├── taskdag/ (compiles to flow/)
                               └── blocks/ (uses triggerflow/)
```

### 7.2 拆分方案详细说明

#### 模块 1：`flow/`（保留原名，纯 Step-based）

**保留文件**：`flow.go`, `step.go`, `engine.go`, `step_validate.go`, `flow_context.go`

**职责**：确定性 DAG/管道编排。`Flow` 定义步骤图，`Engine.Execute()` 按拓扑顺序执行。

**对外接口不变**：`Flow`, `Step`, `FlowBuilder`, `Execution`, `FlowContext`, `Span` 等类型保持原签名。

**依赖**：`schema`（Step 可选 OutputSchema 校验）

**从 flowinfra/ 引入**：`CheckpointStore`, `ExecutionSnapshot`（persistence 核心类型，因为 `*Flow` 的方法需要它们）

#### 模块 2：`triggerflow/`（新模块，纯信号驱动）

**包含文件**：`operator.go`, `operator_handlers.go`, `operator_runtime.go`, `triggerflow_blueprint.go`, `triggerflow_contract.go`, `signal.go`, `action_operator.go`, `lifecycle.go`

**职责**：事件驱动算子编排。`TriggerFlow<T,S,R>` 泛型定义 + `SignalNet` 信号传播 + 13 种内置算子。

**依赖**：`flow`（`FlowContext` 接口、`Span`）、`schema`

**关键**：`TriggerFlow` 不再与 `Flow` 类型耦合，而是通过 `FlowContext` 接口间接使用。

#### 模块 3：`flowinfra/`（新模块，共享基础设施）

**包含文件**：`persistence.go`, `subflow.go`, `inputsource.go`, `goroutine_pool.go`, `callable_ref.go`

**职责**：为 `flow` 和 `triggerflow` 提供共用的持久化、子流程、输入源、并发池等基础设施。

**依赖**：`schema`

### 7.3 拆分后的依赖关系

```
orchestrator ──▶ flow ──▶ schema
    │             │
    │             └──▶ flowinfra (persistence types)
    │
    ├──▶ triggerflow ──▶ flow (FlowContext interface)
    │                └──▶ flowinfra
    │
    └──▶ blocks ──▶ triggerflow (Operator types only)
```

### 7.4 拆分的收益

| 维度 | 当前 | 拆分后 |
|------|------|--------|
| 模块职责 | `flow` = DAG + 信号驱动（模糊） | `flow` = DAG, `triggerflow` = 信号驱动（清晰） |
| 最小依赖 | 使用 Step-based 需携带全部 19K 行 | 使用 Step-based 仅依赖 ~900 行 + flowinfra |
| 概念清晰度 | 「flow 是什么」需要长篇解释 | 模块名即职责 |
| 独立演进 | 两套范式互相制约 | 可独立发展，共享基础设施 |
| blocks/ 归属 | 依赖 flow.Operator（概念上属于 triggerflow） | 直接依赖 triggerflow（语义正确） |

### 7.5 拆分的成本与风险

| 成本项 | 评估 |
|--------|------|
| 新增 2 个 Go module | 低：go.mod + replace 指令 |
| 跨模块类型引用 | 中：`persistence.go` 的 `*Flow` 方法需要重构为独立函数或接口 |
| `FlowContext` 接口归属 | 低：保留在 `flow/` 中，`triggerflow/` 依赖 `flow/` |
| 测试迁移 | 低：测试文件随源文件移动 |
| 向后兼容 | 中：`orchestrator/blocks/` 的 import 路径从 `flow` 改为 `triggerflow` |

### 7.6 迁移路径（如需执行）

1. **Phase 1**：创建 `flowinfra/` 模块，将 `persistence.go`、`subflow.go`、`inputsource.go`、`goroutine_pool.go`、`callable_ref.go` 移入。`flow/` 通过 re-export 保持向后兼容。
2. **Phase 2**：创建 `triggerflow/` 模块，将算子相关文件移入。`flow/` 保留类型别名（`type Operator = triggerflow.Operator`）过渡。
3. **Phase 3**：更新 `orchestrator/blocks/` 的 import 路径直接引用 `triggerflow/`。移除 `flow/` 中的类型别名。
4. **Phase 4**：重构 `persistence.go` 中挂在 `*Flow` 上的方法为独立的 `CheckpointManager` 方法，解除 flow ↔ flowinfra 循环依赖。

> **注**：当前阶段不建议立即执行此拆分。TriggerFlow 功能刚经历增强，处于活跃开发期。建议在 TriggerFlow API 稳定后再考虑模块拆分。当前文档记录此方案作为架构演进方向。

---

## 八、范式选择指南与 Agent 桥接

### 8.1 三种范式的定位

| 范式 | 定义位置 | 控制流驱动方式 | 适合场景 |
|------|---------|------------------|----------|
| Step-based Flow | `flow/` (flow.go, step.go, engine.go) | 引擎按拓扑序主动推进 | 确定性 DAG/管道 |
| Agent executeLoop | `orchestrator/agent/engine.go` | LLM 多轮 PLAN→EXECUTE 决策 | 智能体迭代任务 |
| TriggerFlow 算子系统 | `flow/` (triggerflow_blueprint.go, signal.go) | 信号传播触发 Operator | 事件驱动/扇出扇入 |

### 8.2 场景→范式 选择矩阵

| 你的场景 | 推荐范式 | 原因 |
|----------|----------|------|
| 顺序固定步骤，无 LLM 决策 | Step-based Flow | 最简单、确定性最高 |
| 步骤内需要 LLM 多轮决策 | Step-based + AgentStep | Agent loop 嵌套在 Step 中，通过 `FlowContext.RunAgent` |
| 多个子 Agent 需并行处理 | Step-based + ParallelAgentStep | 通过 `FlowContext.RunAgentParallel`，预留真并行升级 |
| 流程结构动态变化/扇出扇入 | TriggerFlow | 信号驱动、Operator 组合 |
| 混合：部分固定+部分智能 | Step-based + AgentStep | 用同一个 Flow 串联不同能力 |
| 需要人工干预点/外部信号 | TriggerFlow | InterventionPoint + Pause/Resume |

### 8.3 FlowContext Agent 桥接

为解决「Step-based Flow 无法嵌入 Agent 多轮迭代」的架构缺口，`FlowContext` 接口新增了两个方法：

```go
// FlowContext 新增方法（flow/flow_context.go）
type AgentRunOptions struct {
    MaxRounds        int   // 最大迭代轮数，0 = 默认 10
    SessionIsolation bool  // 是否使用独立 Session
}

type AgentSubTask struct {
    Label        string
    UserMessage  string
    SystemPrompt string
    MaxRounds    int
}

RunAgent(ctx, userMessage, systemPrompt, opts *AgentRunOptions) (string, error)
RunAgentParallel(ctx, agents []AgentSubTask) ([]string, error)
```

**实现机制**：`executeFlow` 构建 `flowContextImpl` 时注入 `engine` 引用，`RunAgent` 直接调用 `engine.executeLoop`，复用全部 PLAN→EXECUTE / LoopGuard / Cancel / L3-L4 校验逻辑。

**并行预留**：`RunAgentParallel` 当前为顺序降级执行（每个子任务依次调用 RunAgent）。后续可升级为 goroutine + WaitGroup + WorkerPool 真并行，调用方代码无需修改。

### 8.4 便利工厂函数

`orchestrator/agent` 包提供两个工厂函数，一行代码即可将 Agent 能力嵌入 Flow Step：

```go
// 单个 Agent Step
agent.NewAgentStepFunc(agent.AgentStepConfig{
    SystemPrompt: "You are a code modification agent...",
    MaxRounds:    5,
    InputKey:     "task_description",
    OutputKey:    "modified_code",
})

// 并行多子 Agent Step
agent.NewParallelAgentStepFunc(agent.ParallelAgentStepConfig{
    SubTasks: []agent.SubTaskSpec{
        {Label: "reviewer", SystemPrompt: "Review...", MaxRounds: 3, OutputKey: "review"},
        {Label: "tester",   SystemPrompt: "Test...",   MaxRounds: 2, OutputKey: "test"},
    },
})
```

### 8.5 典型流程建模示例

```go
// GitLab Issue → 收集信息 → 分析方案 → 修改代码(多轮) → 代码审计(多轮)
flow := flow.NewFlow().
    AddStep(collectStep).   // 普通 StepFunc: 单次 toolcall
    To(analyzeStep).        // 普通 StepFunc: 单次 LLM
    To(agent.NewAgentStepFunc(agent.AgentStepConfig{
        SystemPrompt: "You are a code modification agent...",
        MaxRounds:    5,
    })).Build().
    To(agent.NewAgentStepFunc(agent.AgentStepConfig{
        SystemPrompt: "You are a code reviewer...",
        MaxRounds:    3,
    })).Build().
    Build()
```

### 8.6 并行子 Agent 升级路径

当前实现已预留以下升级空间，后续可在不改调用方代码的情况下完成：

| 升级项 | 当前状态 | 升级后 |
|--------|---------|----------|
| `RunAgentParallel` | 顺序降级执行 | goroutine + WaitGroup + WorkerPool 真并行 |
| `SessionIsolation` | 字段存在，退化为共享 | Session fork，子 Agent 写入不影响外层 |
| `AgentRunOptions` 新字段 | MaxRounds + SessionIsolation | TokenBudget / PerAgentTimeout / ConcurrencyLimit |
| 活动任务管理 | 无 | 接入 `orchestrator/taskcontext` TaskContext，子 Agent 注册为活跃任务 |
| Cancel 传播 | Engine 级别共享 ctx | 每个子 Agent 独立 CancelManager，支持选择性取消 |

> **关键约束**：Phase 2 真并行需解决 Session 隔离（多 Agent 同时写 Session 会 race）。`SessionIsolation` 字段 + `RunAgentParallel` 自动隔离标志，使调用方无需关心此细节。

### 8.7 风险评估

| 风险 | 严重度 | 缓解 |
|------|--------|------|
| 嵌套 Agent 共享 Session 历史污染 | 中 | SessionIsolation 预留，Phase 2 实现 fork |
| maxRounds 消耗过多资源 | 低 | 显式控制 + ctx deadline 自然传播 |
| FlowContext 接口变更影响外部实现 | 低 | flowContextImpl 是唯一正式实现 |
| Cancel 信号冲突 | 低 | CancelManager 共享，信号自然传播 |
| 顺序降级不够“并行” | 低 | 当前场景以顺序为主；并行升级仅改内部实现 |
