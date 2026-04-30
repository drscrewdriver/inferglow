# flow - TriggerFlow 编排引擎

**模块路径**: `github.com/inferglow/flow`

## 概述

flow 模块提供两层流引擎架构：**线性 Flow 引擎**（简单管道编排）和 **TriggerFlow 事件驱动引擎**（复杂业务流程编排）。

## 设计定位

- **被谁依赖**: 上层业务逻辑（agently 主模块的 Agent 类）
- **依赖谁**: `github.com/inferglow/schema`（Step 的 Schema 字段用于输出验证）
- **对标 Python**: `agently/builtins/blocks/` 下的 Flow Block + `agently/trigger_flow/`
- **独立可用性**: 依赖 schema 模块

## 两层引擎架构

### 第一层：线性 Flow 引擎 — 简单管道编排

链式 API 构建可执行的步骤管道：

```go
flow := NewFlow().
    AddStep(parseInput).
    To(validate).
    To(transform).
    To(FormatOutput).
    Build()

// 执行
execution := flow.Execute(ctx, input)
result := execution.Result
```

支持条件分支：

```go
flow := NewFlow().
    AddStep(processData).
    If(func(output any) bool { return isErroneous(output) },
        handleErr,       // true branch
        handleOK,        // false branch
    ).
    To(finish).
    Build()
```

#### Step — 可执行步骤

```go
type Step struct {
    Name   string
    Func   StepFunc                // func(ctx, input) (output, error)
    Schema *schema.OutputSchema    // 输出校验契约
}

// 构建器
step := NewStep("myStep", myFunc).WithOutputSchema(schema).Build()
```

#### FlowBuilder — 链式构建

```go
NewFlow().AddStep(step).To(step).To(step).Build()
```

#### Execution — 执行状态

```go
type Execution struct {
    Steps       map[string]*Step
    State       ExecutionState    // created/running/completed/failed/paused
    StepLog     []StepLogEntry    // 每步执行记录
}
```

### 第二层：TriggerFlow — 事件驱动编排

基于信号（Signal）和算子（Operator）的事件驱动引擎：

#### Operator — 编排基本单元

```go
type Operator struct {
    ID            string
    Name          string
    Kind          OperatorKind       // 算子类型
    ListenSignals []string           // 监听哪些信号
    EmitSignals   []string           // 发射哪些信号
    HandlerRef    *CallableRef       // 可序列化的 handler 引用
}
```

**13 种算子类型：**

| 算子 | 用途 |
|------|------|
| `chunk` | 基础处理单元 |
| `signal_gate` | 信号门控 |
| `batch_fanout` | 批量扇出 |
| `batch_collect` | 批量收集 |
| `for_each_split` | 拆分循环 |
| `for_each_collect` | 循环收集 |
| `match_route` | 路由匹配 |
| `match_case` | 条件分支 |
| `match_collect` | 匹配收集 |
| `collect_branch` | 分支收集 |
| `intervention_point` | 干预点（暂停） |
| `sub_flow` | 子流程 |
| `result_sink` | 结果汇 |

#### SignalNet — 信号路由网络

```go
// 静态 handler（编译期注册）
signalNet.RegisterStaticHandler(triggerEvent, name, handler)

// 动态 handler（运行时注册）
signalNet.RegisterDynamicHandler(triggerEvent, handler, opts...)

// 信号路由
route := signalNet.Route(sig *Signal) []Handler
```

#### LifecycleMachine — 生命周期状态机

```
open → running → waiting → running → ... (可循环)
   \-> sealed → closed
    \-> failed → closed
     \-> closed (最终状态)
```

#### Persistence — 持久化

支持 Execution 状态序列化（JSON/YAML），包括暂停恢复点、干预状态、子流帧、ResumeToken 等扩展字段。

## 核心接口一览

```
Flow                → 线性流程
FlowBuilder         → 链式构建器
Step / StepFunc     → 可执行步骤
StepBuilder         → 步骤构建器
Execution / ExecutionState → 执行状态

OperatorKind        → 算子类型枚举
Operator            → 编排算子
SignalNet           → 信号路由网络
Signal              → 信号事件
Handler             → 信号处理函数

LifecycleMachine    → 生命周期状态机
TriggerFlow         → 泛型 TriggerFlow 入口
SubFlowFrame        → 子流程帧
WorkerPool          → Goroutine 池
```

## 与 schema 的关系

flow 的 `Step` 携带 `*schema.OutputSchema` 字段，在 Step 执行后可以自动验证输出是否符合 Schema 定义。StepBuilder 提供 `WithOutputSchema()` 方法绑定校验契约。

## 与上层的关系

```
agently 主模块 (Agent 类)
  ├── 简单任务 → FlowBuilder 链式编排
  ├── 复杂业务 → TriggerFlow 事件驱动编排
  ├── 干预暂停 → intervention_point 算子 → PausePoint 持久化
  ├── 后续恢复 → ExecutionPersistence.Load() → 恢复执行
  └── Blueprint → 流程定义版本化管理
```
