# V6 剩余任务实施计划

## 决策：F2 推迟到 v7

`PausePoint{Input any}` → `PausePoint[T]` 是唯一的 breaking change，波及 `flow/pause.go`、`flow/persistence.go`、5+ 测试文件。当前 `any` 在生产中运行良好，收益不足以证明风险。F1 桥接层内部提供运行时类型断言作为替代。

## Phase A: F4 — Prompt 版本标记 (~50 行)

**目标**: Session 元数据记录 prompt template version，为 F3 回放测试铺路。

1. `session/session.go` — `Session` struct 增加 `PromptVersion string` 字段
2. `session/persistence.go` — `SessionData` struct 增加 `PromptVersion string` + json/yaml tag；`toSessionDataLocked()` 和 `LoadFromData()` 双向同步
3. `session/persistence_test.go` — 追加 ToJSON/LoadJSON 往返测试

## Phase B: F5 — 成本模型 (~80 行)

**目标**: `Pricing` 结构 + `Cost(UsageInfo) float64`，每轮对话成本可见。

1. 新建 `model/pricing.go`:
   - `Pricing` struct: `CacheHit`, `Input`, `Output float64`, `Currency string`
   - `Cost(u *UsageInfo) float64` 方法 — 利用 `PromptTokensDetails["cached_tokens"]` 区分缓存命中
   - `SessionCost` struct 用于累积: `TotalCost`, `CachedTokens`, `TotalTokens`, `Currency`
   - `(sc *SessionCost) Add(u *UsageInfo, p *Pricing)` 累积方法
   - 与现有 `PricingInfo`(router.go) 共存：`PricingInfo` 用于 Router 选路排序，`Pricing` 用于实际成本计算
2. 新建 `model/pricing_test.go` — 表驱动测试：纯 input/output、缓存命中、nil 安全、累积

## Phase C: F1 — HITL 审批集成 (~180 行)

**目标**: flow↔approval 桥接，DAG 任意节点暂停 → 审批决策 → checkpoint 精确恢复。

1. `approval/types.go` — `Request` struct 增加:
   - `Timeout time.Duration` — 审批等待超时
   - `Escalation string` — 超时策略（"auto_deny"/"escalate_to_admin"）
2. 新建 `orchestrator/hitl/bridge.go`:
   - `Bridge` struct 持有 `*approval.PolicyApprovalManager`
   - `PauseForApproval(ctx, flow, exec, req)` — 调用 exec.Pause() → SaveCheckpoint → manager.Submit() → 等待 ResolveRecord → Resume
   - 放在 orchestrator 层因为 orchestrator 已同时依赖 flow 和 approval（flow 不依赖 approval，桥接不能放 flow 内）
3. 新建 `orchestrator/hitl/bridge_test.go` — auto-approve 快速路径、pending→resolve、timeout 路径

## Phase D: S4 — team/ 包 (~3d)

**目标**: Multi-Agent 最小可行 — Coordinator 协调多个 Agent 协作。

1. 新建 `orchestrator/team/types.go`:
   - `AgentRunner` interface: `Run(ctx, userMessage string) (string, error)`
   - `AgentAdapter` struct 包装 `*agent.Agent`（因 Agent.Run 有 variadic opts，不直接满足接口）
   - `Member` struct: `Agent AgentRunner`, `Role string`, `Handoff []string`
   - `Result` struct: `FinalResponse`, `MemberOutputs`, `Rounds`
2. 新建 `orchestrator/team/bus.go`:
   - 私有 `messageBus` struct — `sync.RWMutex` + `[]Message`（非 channel，因消息需多读者查看）
3. 新建 `orchestrator/team/coordinator.go`:
   - `Coordinator` struct: `members`, `bus`, `maxRounds`, `middlewares []middleware.Middleware`
   - `NewCoordinator(members, opts...)` 构造函数
   - `Round(ctx, task) (*Result, error)` — 通过 `middleware.Chain()` 包装协调循环
4. 新建 `orchestrator/team/coordinator_test.go` — mock AgentRunner 测试 Round 流程、handoff、maxRounds、middleware

## Phase E: F3 — Agent 确定性回放测试 (~300 行)

**目标**: 从 JSON session 重放 + tool call 拦截 + 行为对比。

1. 新建 `orchestrator/agent/replay.go`:
   - `ReplayConfig`: `SessionFile`, `PromptVersion`, `ToolInterceptor`
   - `ReplayResult`: `Match`, `Expected`, `Actual`, `ToolCalls`, `Diffs`
   - `Replay(ctx, agent, cfg)` — 加载 golden session → 验证 PromptVersion → 重放消息 → 拦截 tool call → 对比输出
2. 新建 `orchestrator/agent/replay_test.go` — 演示回放测试用法

## Phase F: S5 — EXTENDING.md (2h)

**目标**: 文档化 7 种扩展机制，降低新贡献者困惑。

1. 新建 `docs/EXTENDING.md`:
   - Interface injection (MessageHook, PIIMasker)
   - Middleware 链 (middleware.Handler + legacy agent.Middleware)
   - Callbacks (AgentCallbacks)
   - Build tags (with_sandbox)
   - contextmgr modes (passthrough, three_zone, hybrid)
   - Block registry (FlowBlock + BlockRegistry)
   - Resize handlers (SimpleCut, SummaryFirst, TokenAware)
   - 决策原则："不增加第 8 种机制"

## 执行顺序与依赖

```
F4 (1h) ──────────────────→ F3 (1d)
F5 (1h) ─── 独立
F1 (1d) ─── 独立
S4 (3d) ─── 独立（依赖已完成的 middleware/）
S5 (2h) ─── 建议最后（可文档化 team/）
```

**推荐**: F4 → F5 → F1 → S4 → F3 → S5

## 关键风险

| 风险 | 缓解 |
|------|------|
| Agent.Run variadic 签名不满足 AgentRunner 接口 | AgentAdapter 包装器固定 opts |
| F1 桥接引入 flow↔approval 循环依赖 | 桥接放 orchestrator/hitl/，不放 flow/ |
| F4 PromptVersion 影响 JSON 序列化兼容 | omitempty tag，旧 JSON 无此字段反序列化为空串 |
| S4 messageBus 并发安全 | sync.RWMutex + race detector 测试 |
| F3 回放对 LLM 非确定性敏感 | ToolInterceptor 固定返回；比较 tool call 序列而非精确文本 |

## 拒绝的替代方案

- **F2 立即实施**: PausePoint[T] 是 breaking change，波及 5+ 测试文件，风险/收益比不合理
- **HITL 桥接放 flow/ 内**: flow/go.mod 无 approval 依赖，会引入新模块依赖
- **team/ 用 channel 做 messageBus**: channel 消费后即消失，handoff 场景需多读者
- **AgentRunner 接口包含 variadic opts**: 丢失接口简洁性，且 team 不需要传递 RunOption
