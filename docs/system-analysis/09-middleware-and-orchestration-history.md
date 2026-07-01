# 编排层与中间层：历史成因与发展分析

> 归档 InferGlow 架构中两个核心聚类——编排层（Orchestration）与中间层（Middleware）——的演化历史、设计动机与关系辨析。
> 
> 编写日期：2026-07-30 | 来源：commit e6da191 评审过程中的架构追溯

---

## 一、术语定义

| 术语 | 指代 | 所在路径 |
|------|------|----------|
| **编排层** | Agent 执行引擎（TurnLoop、executeLoop、ActionDispatcher） | `orchestrator/agent/` + `orchestrator/actionruntime/` |
| **Flow 编排** | LCEL 链式编排 + Stage 模块 | `flow/` |
| **中间层（unified middleware）** | 共享的类型定义：Handler/Middleware/Input/Output | `orchestrator/middleware/` |
| **legacy middleware** | agent 包内部的原始 Middleware 类型（已 deprecated） | `orchestrator/agent/middleware.go` |
| **server middleware** | HTTP 层面的 net/http middleware（无关概念） | `server/middleware/` |

---

## 二、演化时间线

```
时间轴 (2026年)
  
  P1-P3 时期
  ──────────
  项目有 6 个独立 Go module（session, model, action, sandbox, schema, flow），
  但全是"零件"（parts）——没有组装线把它们串起来。
  
  G2/G3 时期
  ──────────
  ★ 编排层诞生 ★
  
  产生需求：零件之间如何通信？
  答案：创建 orchestrator/ 包作为"组装线"。
  
  核心产出：
  - orchestrator/agent/engine.go — executeLoop (PLAN→EXECUTE 循环)
  - orchestrator/agent/turnloop/ — TurnLoop 状态机 (idle→planning→active)
  - orchestrator/actionruntime/ — ActionDispatcher + 安全门控
  - flow/ — Flow 引擎、Stage 算子、TriggerFlow
  
  这是有状态的、重型的基础设施——import model, session, action, sandbox。
  关注的是"怎么跑"（执行语义）。
  
  Wave V2 (2dce2a2)
  ─────────────────
  ★ 中间层（agent 内嵌版本）诞生 ★
  
  产生需求：编排层稳定后，横切关注点（rate limit、OTel tracing、audit、auth）
  需要注入执行边界，但不应污染 executeLoop 内部逻辑。
  
  答案：装饰器模式——func(next AgentHandler) AgentHandler
  
  核心产出：
  - orchestrator/agent/middleware.go
  - AgentHandler / Middleware / WithMiddleware
  
  此时中间层是 agent 包内专属，不是独立层。
  关注的是"跑的时候边上挂什么"（横切拦截）。
  
  V6 S3 (2246dde)
  ───────────────
  ★ 中间层独立化为共享包 ★
  
  产生需求：team/（多 Agent 协调器）和 workflow/ 也需要横切关注点。
  如果每个执行包各自定义一套 Handler/Middleware 类型，一个 OTelMiddleware
  需要实现三遍。
  
  决策（引自 ARCHITECTURE_V6.md §5）：
  "虽然不应强行统一 Agent/Flow/RAG 的执行模型，但横切关注点（OTel tracing、
  audit、recovery、rate limit）在 agent 和 team 包中都存在。值得做的不是统一
  Step 接口，而是统一 Middleware 的类型签名。"
  
  核心产出：
  - orchestrator/middleware/（新建，零依赖）
  - Handler / Middleware / Input / Output / Chain
  - agent/middleware.go 标记 Deprecated，适配桥接
  
  后续演进
  ────────
  - V6 L1-L4: prefix cache, SSE, eval, sandbox
  - e6da191: 三模式抢占（PreemptMode API + InputQueue）
  - d291a89: CLI Agent（通过 agent.Run() 同步调用，不使用 middleware 桥接）
  - 43430af: LCEL 风格流编排 + Stage 模块
  - e5ee08a: Server REST API + Trigger/Flow/Memory 管理
```

---

## 三、两个聚类的本质差异

| 维度 | 编排层 | 中间层 |
|------|--------|--------|
| **解决的问题** | 零件之间的通信与协调 | 执行边界的包裹与拦截 |
| **类比** | 汽车的发动机 + 变速箱 | 汽车的外漆 + 隔音棉 |
| **性质** | **有状态**（TurnLoop、Session） | **无状态**（函数管道） |
| **生命周期** | 有——idle→planning→active→idle | 无——仅请求/响应拦截 |
| **组合方式** | 数据流链式（LCEL Pipe） | 装饰器链（一层包一层） |
| **依赖** | 重型——import model, session, action... | 零依赖——只有 context + stdlib |
| **共享范围** | agent 专有（每个执行模型不同） | agent / team 共享 |
| **犯错代价** | 高——改错破坏执行语义 | 低——加一个 middleware 只影响横切面 |

**关系**：正交的。编排管纵向的生命周期，中间层管横向的拦截面。两者不竞争，不重叠。

---

## 四、实际消费现状（2026-07-30）

### orchestrator/middleware/ 的导入方

| 消费方 | 使用方式 | 是否必需 |
|--------|----------|----------|
| `orchestrator/agent/agent.go` | 存储 `unifiedMiddlewares []middleware.Middleware` | ✅ |
| `orchestrator/agent/middleware.go` | 桥接 adapter + Deprecated 旧类型 | ✅（过渡期） |
| `orchestrator/team/coordinator.go` | 直接使用 `middleware.Chain` / `middleware.Input` / `middleware.Output` | ✅ |
| `orchestrator/team/types.go` | 注释引用 | ✅ |

**结论**：中间层不是理论抽象——team 包确实直接消费它。两个聚类之间不互相 import（agent 不 import team，team 不 import agent），中间层是它们共享的公共词汇表。

### 未消费方

| 组件 | 为何未消费 | 是否应该 |
|------|-----------|----------|
| `flow/` | Flow 是独立 Go module（`go.mod`），通过 `FlowContext.RunAgent` 桥接，不直接走 middleware 链 | 合理——Flow 是编排模型而非请求处理管道 |
| `server/` | HTTP 层有自己的 net/http middleware | 合理——不同抽象层级 |
| CLI | 直接调 `agent.Run()`，不走 middleware 链 | 当前合理——CLI 不需要 OTel/audit/rate limit |

---

## 五、关于中间层独立化的设计决策记录

### V6 S3 之前的问题

```
agent/ 定义: type AgentHandler func(ctx, string) (string, error)
team/  需要: type TeamHandler  func(ctx, *Input) (*Output, error)
workflow/ 需要: type WorkflowHandler func(ctx, *Input) (*Output, error)
```

一个 `OTelMiddleware` 需要实现三套签名。

### V6 S3 的决策

**不统一执行模型**（Agent、Team、Flow 的执行循环天然不同），但**统一装饰器签名**：

```go
// orchestrator/middleware/middleware.go
type Handler func(ctx context.Context, input *Input) (*Output, error)
type Middleware func(next Handler) Handler
```

agent 和 team 各自保持自己的执行循环，共享 `Middleware` 类型。
agent 通过 `adaptUnifiedToLegacy` 桥接新旧签名，保持向后兼容。

### 决策依据

- net/http.Handler 模式的 Go 惯用法——零学习成本
- 零依赖包，不引入重型模块
- 避免 agent/ 与 team/ 之间的循环依赖
- 一个 OTelMiddleware 实现可用于 agent 和 team

---

## 六、遗留的复杂性

当前仍存在一段过渡期残留：`orchestrator/agent/middleware.go` 维护了两套类型系统：

1. **旧**：`agent.AgentHandler` / `agent.Middleware` / `agent.WithMiddleware`（Deprecated）
2. **新**：`middleware.Handler` / `middleware.Middleware` / `agent.WithUnifiedMiddleware`

通过 `adaptUnifiedToLegacy` 桥接。这在项目正式开放共享前可以清理——直接移除旧类型，将 `WithUnifiedMiddleware` 重命名为 `WithMiddleware`。

---

> 参见：ARCHITECTURE_V6.md §5「统一 Middleware 类型签名」、ARCHITECTURE.md §8「V2 优化波次」
