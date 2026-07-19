# Flow 多范式共存架构调整

## 背景

InferGlow 当前存在三种执行范式，但**无法互相嵌入**：

| 范式 | 能力 | 局限 |
|------|------|------|
| Step-based Flow | 确定性 DAG 编排 | 每个 Step 只能执行一次，无法迭代 |
| Agent executeLoop | 多轮 PLAN→EXECUTE 循环 | 无法嵌套在 Flow 步骤中 |
| TriggerFlow 算子系统 | 信号驱动事件编排 | 复杂度高，不适合简单场景 |

**目标场景**：`收集信息(单次) → 分析方案(单次LLM) → 修改代码(多轮Agent) → 代码审计(多轮+打回)`

## 设计哲学：三层分离

```
Layer 1: Flow 拓扑层（决定"流程怎么连"）  → 已有，不变
Layer 2: 节点执行语义（决定"每个节点怎么执行"）→ 新增抽象
Layer 3: 横切能力（Session/Audit/Model/Action）→ 已有 FlowContext，扩展 RunAgent
```

核心原则：**Flow 定义"骨架"，Step 定义"节点能力"**。一个 Step 可以是简单函数、Agent 多轮循环、或信号驱动的算子图，但 Flow 只关心拓扑顺序。

## 实现方案（分两阶段）

### 阶段一：FlowContext.RunAgent 桥接（最小可用变更，~130行）

让 Flow 的任意 Step 能内嵌完整的 Agent 多轮循环，不改现有 API。

#### Task 1.1: 扩展 FlowContext 接口

**文件**: `flow/flow_context.go` (L105 后)

新增 1 个方法：

```go
// RunAgent 在 step 内部触发一次完整的多轮 Agent 循环（PLAN→EXECUTE）。
// 返回 Agent 最终回复文本。未配置 Agent 运行时返回 error。
RunAgent(ctx context.Context, userMessage string, systemPrompt string, maxRounds int) (string, error)
```

- 仅新增方法，不修改现有 11 个方法
- 外部若实现了 FlowContext 接口，需补充该方法（可返回 sentinel error）

#### Task 1.2: flowContextImpl 实现 RunAgent

**文件**: `orchestrator/agent/flow_context_impl.go`

(a) 结构体新增字段（L57 后）：
```go
engine *Engine  // 可选。nil 时 RunAgent 返回错误。由 executeFlow 注入。
```

(b) 文件末尾新增实现（~25行）：
```go
func (fc *flowContextImpl) RunAgent(ctx context.Context, userMessage string, systemPrompt string, maxRounds int) (string, error) {
    if fc.engine == nil {
        return "", fmt.Errorf("flow: RunAgent not available (no engine configured)")
    }
    if maxRounds <= 0 { maxRounds = 10 }
    decision, err := fc.engine.executeLoop(ctx, userMessage, maxRounds, systemPrompt)
    if err != nil { return "", err }
    if decision == nil { return "", fmt.Errorf("flow: RunAgent returned nil decision") }
    return decision.FinalResponse, nil
}
```

- 复用全部 executeLoop 能力：PLAN→EXECUTE / LoopGuard / Cancel / L3-L4
- engine 为 nil 时安全降级，不 panic

#### Task 1.3: executeFlow 注入 engine 引用

**文件**: `orchestrator/agent/flow_exec.go` (L83-91)

构建 flowContextImpl 时增加 `engine: e`：

```go
fc := &flowContextImpl{
    session:    e.session,
    actionExt:  e.actionExt,
    modelReq:   e.modelReq,
    auditHook:  e.auditHook,
    tracer:     tracer,
    piiMasker:  nil,
    outputHook: nil,
    engine:     e,  // 新增
}
```

#### Task 1.4: 便利工厂 NewAgentStepFunc

**新文件**: `orchestrator/agent/agent_step.go` (~80行)

```go
type AgentStepConfig struct {
    SystemPrompt string
    MaxRounds    int     // 0 = 默认10
    InputKey     string  // 从 map[string]any input 取值的 key
    OutputKey    string  // Agent 回复写入 output 的 key
}

func NewAgentStepFunc(cfg AgentStepConfig) flow.StepFunc { ... }
```

使用示例：
```go
// 用户场景：GitLab Issue → 收集 → 分析 → 修改(多轮) → 审计(多轮)
flow := flow.NewFlow().
    AddStep(collectStep).   // 普通 StepFunc: 单次 toolcall
    To(analyzeStep).        // 普通 StepFunc: 单次 LLM
    To(agent.NewAgentStepFunc(agent.AgentStepConfig{
        SystemPrompt: "You are a code modification agent...",
        MaxRounds: 5,
    })).Build().            // AgentStep: 多轮 toolcall
    To(agent.NewAgentStepFunc(agent.AgentStepConfig{
        SystemPrompt: "You are a code reviewer...",
        MaxRounds: 3,
    })).Build().            // AgentStep: 多轮审计
    Build()
```

#### Task 1.5: 测试

- `orchestrator/agent/agent_step_test.go`: 测试 NewAgentStepFunc 的 InputKey/OutputKey 行为
- `orchestrator/agent/flow_exec_test.go`: 测试 executeFlow 中 RunAgent 可用

### 阶段二：架构文档 + 范式选择指南（文档更新）

#### Task 2.1: 更新 03-flow.md 新增"范式选择指南"

**文件**: `docs/system-analysis/03-flow.md`

新增第八节，内容：

```
| 你的场景 | 推荐范式 | 原因 |
|----------|----------|------|
| 顺序固定步骤，无LLM决策 | Step-based Flow | 最简单、确定性最高 |
| 步骤内需要LLM多轮决策 | Step-based + AgentStep | Agent loop 嵌套在 Step 中 |
| 流程结构动态变化/扇出扇入 | TriggerFlow | 信号驱动、Operator 组合 |
| 混合: 部分固定+部分智能 | Step-based + AgentStep | 用同一个 Flow 串联 |
| 需要人工干预点/外部信号 | TriggerFlow | InterventionPoint + Pause |
```

#### Task 2.2: 更新架构总览文档

**文件**: `docs/system-analysis/01-architecture-overview.md`

在依赖图中标注 FlowContext.RunAgent 的桥接关系。

## 文件变更清单

| 文件 | 操作 | 行数 |
|------|------|------|
| `flow/flow_context.go` | 修改: 接口新增 RunAgent | +5 |
| `orchestrator/agent/flow_context_impl.go` | 修改: engine 字段 + RunAgent 实现 | +30 |
| `orchestrator/agent/flow_exec.go` | 修改: 注入 engine: e | +1 |
| `orchestrator/agent/agent_step.go` | **新建**: AgentStepConfig + NewAgentStepFunc | +85 |
| `orchestrator/agent/agent_step_test.go` | **新建**: 测试 | +100 |
| `docs/system-analysis/03-flow.md` | 修改: 范式选择指南 | +40 |
| `docs/system-analysis/01-architecture-overview.md` | 修改: 桥接关系 | +10 |
| **总计** | | **~271** |

## 依赖关系

```
Task 1.1 (FlowContext 接口)
    ↓
Task 1.2 (flowContextImpl 实现)
    ↓
Task 1.3 (flow_exec 注入)
    ↓
Task 1.4 (NewAgentStepFunc 工厂)
    ↓
Task 1.5 (测试)
    ↓
Task 2.1-2.2 (文档)
```

## 风险评估

| 风险 | 严重度 | 缓解 |
|------|--------|------|
| 嵌套 Agent 共享 Session 导致历史污染 | 中 | 文档说明 Session 共享语义；后续可选 Session fork |
| RunAgent 的 maxRounds 消耗过多资源 | 低 | maxRounds 显式控制 + ctx deadline 自然传播 |
| FlowContext 接口变更影响外部实现 | 低 | 接口仅在 flow 包定义，flowContextImpl 是唯一正式实现 |
| AgentStep 内 Cancel 与外层 Flow 信号冲突 | 低 | CancelManager 是 Engine 级别共享，信号自然传播 |

## 被否决的方案

| 方案 | 否决原因 |
|------|---------|
| 新增 `NodeExecutor` 接口改 Step 类型 (Agent A) | 需要修改 `flow.Step` 结构体，破坏现有 Step 使用方式。当前方案通过 `flow.StepFunc` 间接实现等价能力，零破坏 |
| 并行执行 + ParadigmAdapter (Agent B) | 当前用户场景（顺序流水线）不需要并行执行。过度设计，增加大量复杂度。WorkerPool 已存在，需要时可直接使用 |
| 用 TriggerFlow 建模（InterventionPoint + SignalNet）| 用户场景是顺序流程，TriggerFlow 杀鸡用牛刀。且 blocks/ 编译的 Operator 图与 Step-based 是不同抽象层 |
| 独立 `triggerflow/` 模块拆分 | 用户明确表示不做代码整合，TriggerFlow 处于活跃开发期，API 不稳定时拆分无意义 |
| Step 内直接 new Agent 实例 | 会造成循环依赖（flow 不能依赖 orchestrator）。通过 FlowContext 接口桥接是唯一无循环依赖的方案 |
