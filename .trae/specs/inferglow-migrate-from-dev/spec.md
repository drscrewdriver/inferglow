# inferglow-github 迁移 Spec（从 dev 仓库代码对应迁移）

## Why

目标仓库应为 `inferglow-github`（`01e30aa`），但此前在错误的 dev 仓库 `inferglow`（`615aa32`）上落地了「8 个 agent 失败修复」与「inferglow-verified-parity-optimize」两类改动（均未 commit）。两仓库已分叉，**不能按 commit 打补丁**，须按文件内容做**代码对应迁移**。

迁移范围（用户已确认）：
1. **8 个 agent 失败修复**（loop-guard stale-dedup → synthesis）
2. **上一 spec（parity-optimize）** 的 6 个任务产物（Pre/Post 干预钩子、会话 Fork/Ephemeral、Rollout 记录器、声明式策略加载、并行安全声明）

用户明确要求：**先审 spec 再执行**。本 spec 未经批准不得改动任何代码。

## 迁移判定结论（已实测）

| 文件 | dev HEAD 基线 vs ghub HEAD 基线 | 迁移方式 |
|---|---|---|
| `orchestrator/agent/engine.go` | **HEAD-DIFFERS** | 逐 hunk 代码对应重做（仅迁移两 spec 相关改动） |
| `orchestrator/agent/agent.go` | **HEAD-DIFFERS** | 逐 hunk 代码对应重做 |
| `orchestrator/agent/agent_test.go` | HEAD-IDENTICAL | 直接按内容同步 |
| `orchestrator/agent/engine_test.go` | HEAD-IDENTICAL | 直接按内容同步 |
| `orchestrator/agent/engine_audit_test.go` | HEAD-IDENTICAL | 直接按内容同步 |
| `orchestrator/agent/callbacks.go` | HEAD-IDENTICAL | 直接按内容同步 |
| `orchestrator/agent/cancel.go` | HEAD-IDENTICAL | 直接按内容同步 |
| `orchestrator/agent/input_queue.go` | HEAD-IDENTICAL | 直接按内容同步 |
| `orchestrator/agent/input_queue_test.go` | HEAD-IDENTICAL | 直接按内容同步 |
| `orchestrator/agent/internal/cancel/cancel.go` | HEAD-IDENTICAL | 直接按内容同步 |
| `session/session.go` | HEAD-IDENTICAL | 直接按内容同步 |
| `session/persistence.go` | HEAD-IDENTICAL | 直接按内容同步 |
| `action/action.go` | HEAD-IDENTICAL | 直接按内容同步 |
| `action/spec.go` | HEAD-IDENTICAL | 直接按内容同步 |
| `orchestrator/actionruntime/dispatcher.go` | HEAD-IDENTICAL | 直接按内容同步 |
| `server/handlers_session.go` | **HEAD-DIFFERS** | 逐 hunk 代码对应（fork 路由） |
| `server/router.go` | **HEAD-DIFFERS** | 逐 hunk 代码对应（fork 路由） |

**新增文件**（dev 已存在、ghub 缺失，直接复制内容）：
- `orchestrator/agent/engine_stale_synthesis_test.go`（8 失败修复）
- `orchestrator/agent/callbacks_intervene_test.go`（parity T2）
- `orchestrator/agent/rollout_integration_test.go`（parity T4）
- `session/rollout.go`、`session/rollout_test.go`（parity T4）
- `session/fork_test.go`（parity T3）
- `sandbox/policy_loader.go`、`sandbox/policy_loader_test.go`（parity T5）
- `orchestrator/actionruntime/dispatcher_parallel_safe_test.go`（parity T6）

**不迁移**（仅属 dev 仓库或无关内容）：
- `cli/`、`web/`、`model/`、`server/message_store.go`、`server/handlers_messages.go`、`server/handlers_context_layers.go`、`server/handlers_trace.go`、`server/trace_store.go`、`sandbox/go.mod` 等不属上述两类 spec 的改动。
- `engine.go` 中与两 spec 无关的 hunk（若含）一律不迁。

## What Changes

按上述判定，分两批：

- **批次 A（8 失败修复，先做）**
  - `engine.go`：stale-dedup L783-789 硬停 `return ErrLoopDetected` → 返回空 execute decision（`FinalResponse=""`）触发 synthesis；不再抢在轮次上限前报错，保留 loopGuard 硬停。
  - 测试校准：`agent_test.go`（3 个 stale→synthesis 断言）、`engine_test.go`（cap 路径）、`engine_audit_test.go`（轮次计数）。
  - 新增 `engine_stale_synthesis_test.go`（卡死→synthesis + loopGuard 硬停覆盖）。

- **批次 B（上一 spec parity-optimize 产物）**
  - T2：`callbacks.go` Pre/Post 钩子类型 + `engine.go` 工具执行段接线（Pre→Block/Rewrite/AppendContext、Post→AppendContext、原始参数审计）+ `callbacks_intervene_test.go`。
  - T3：`session.Fork()` + `NewEphemeralSession()` + `server` fork 路由 + `fork_test.go`。
  - T4：`session/rollout.go` RolloutRecorder + `engine.go` 接线（user/assistant/tool_call/tool_result）+ `rollout.go`/`rollout_integration_test.go`。
  - T5：`sandbox/policy_loader.go` yaml 严格解码 + 求交 + 测试。
  - T6：`action/spec.go` `ParallelSafe *bool` + `dispatcher.go` 串/并发分流 + `dispatcher_parallel_safe_test.go`。

## Impact

- `orchestrator/agent/*`、`session/*`、`action/*`、`orchestrator/actionruntime/*`、`sandbox/*`、`server/{handlers_session,router}.go`
- 行为语义变化：卡死识别→synthesis（非报错）；新增可干预钩子 / Fork / Rollout / 策略加载 / 并行安全，均向后兼容。
- **验收**：`go test ./orchestrator/agent/... ./session/... ./action/... ./actionruntime/... ./sandbox/... ./server/...` 全绿；`git status` 确认只含上述迁移文件。

## Requirements

### REQ-1 代码对应迁移
系统 SHALL 仅迁移本 spec 判定表列出的文件与 hunk；对 HEAD-DIFFERS 文件必须逐 hunk 代码对应，禁止整文件覆盖引入无关改动；HEAD-IDENTICAL 文件按内容直接同步。

#### Scenario: HEAD-DIFFERS 文件
- **WHEN** 迁移 `engine.go` / `agent.go` / `handlers_session.go` / `router.go`
- **THEN** 仅应用两 spec 相关改动，保留 ghub 自身其余实现，编译通过

#### Scenario: HEAD-IDENTICAL 文件
- **WHEN** 迁移 `agent_test.go` 等基线一致文件
- **THEN** 迁移后文件内容与 dev 仓库一致，测试断言准确定位

### REQ-2 8 失败修复语义
`engine.go` stale-dedup SHALL 在连续相同调用达阈值（3）时返回空 execute decision 触发 synthesis，而非 `ErrLoopDetected`；loopGuard 硬停保留。新增测试覆盖「卡死→synthesis」与「loopGuard 硬停」。

### REQ-3 上一 spec 产物齐全且通过
T2–T6 产物 SHALL 全部迁入并各自既有测试全绿；新增测试文件一并迁入。

### REQ-4 审阅门槛
任何代码改动 SHALL 在本 spec 通过用户批准后才允许执行；批准前禁止改动 `inferglow-github` 任何文件。