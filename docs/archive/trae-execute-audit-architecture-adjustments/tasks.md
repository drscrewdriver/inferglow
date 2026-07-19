# Tasks

* [x] Task 0: master 备份归档

  * [x] SubTask 0.1: 在当前 master HEAD 创建 annotated tag `pre-arch-adjust-20260724`，message 包含审计报告路径和回滚说明

  * [x] SubTask 0.2: 推送 tag 到 origin（`git push origin pre-arch-adjust-20260724`）

  * [x] SubTask 0.3: 验证 tag 已创建且指向正确 commit（3642d93）

* [x] Task 1: 删除 components/tool 死代码包（P0-8）

  * [x] SubTask 1.1: 全局搜索确认无任何包引用 `github.com/inferglow/components/tool`（排除 components/tool 自身）

  * [x] SubTask 1.2: 删除 `components/tool/` 目录下所有文件（5 个 .go 文件，保留 components/go.mod 因 components/prompt 仍存在）

  * [x] SubTask 1.3: components/ 目录下仍有 prompt 子包，保留 components/go.mod、components/go.sum

  * [x] SubTask 1.4: 运行 `go build ./...` 验证 inferglow 根模块编译通过

* [x] Task 2: 删除 model.ActionResult 死类型与 ModelRequest.Actions 死字段（P0-13、P2-1）

  * [x] SubTask 2.1: 全局搜索确认无代码引用 `model.ActionResult`（排除定义本身）和 `ModelRequest.Actions`

  * [x] SubTask 2.2: 删除 `model/model.go` 中 `ActionResult` 类型定义

  * [x] SubTask 2.3: 删除 `ModelRequest` 结构体的 `Actions` 字段

  * [x] SubTask 2.4: 运行 `go build ./...` 和 `go test ./model/...` 验证通过

* [x] Task 3: 修复沙箱策略 deny-by-default 基线（P0-3）

  * [x] SubTask 3.1: 在 `sandbox/` 包定义 `ServerPolicyBaseline` 结构，包含 `NetworkAccess` / `PathAllowlist` / `MaxOutputBytes` / `Timeout` 的服务端默认值

  * [x] SubTask 3.2: 修改 `buildPolicyFromInput`（`executor_sandbox.go:182-223`），将 LLM input map 参数与服务端基线求交集（取更严格值）

  * [x] SubTask 3.3: 新增策略收紧日志，记录 LLM 参数被基线覆盖的情况

  * [x] SubTask 3.4: 添加测试覆盖：LLM 放宽策略被拒、LLM 收紧策略被接受

* [x] Task 4: 修复 approval\_required 服务端决定（P0-4）

  * [x] SubTask 4.1: 修改 `executor_sandbox.go:96`，`approval_required` 不再从 input map 读取，改由服务端策略基线决定

  * [x] SubTask 4.2: 在 `ServerPolicyBaseline` 中增加 `ApprovalRequired` 字段

  * [x] SubTask 4.3: 添加测试：LLM 传 `approval_required: false` 但服务端要求审批时仍触发审批

* [x] Task 5: 修复 Docker/gVisor 网络策略强制执行（P0-5）

  * [x] SubTask 5.1: 修改 `sandbox/docker.go:236`，根据 `network_access` 策略设置 `NetworkDisabled` 字段

  * [x] SubTask 5.2: 修改 `sandbox/gvisor.go`，根据 `network_access` 策略配置网络隔离

  * [x] SubTask 5.3: 添加测试覆盖 `network_access: "none"` 时网络被禁用

* [x] Task 6: 修复 ModeAuto 不回落到无隔离（P0-6）

  * [x] SubTask 6.1: 修改 `sandbox/manager.go:93`，移除 `ModeAuto` 自动回落到 `trusted_local` 的逻辑

  * [x] SubTask 6.2: 新增显式配置开关 `allow_trusted_fallback`（默认 false），仅显式启用时允许回落

  * [x] SubTask 6.3: 添加测试：未配置时 ModeAuto 无可用后端返回错误，配置时允许回落

* [x] Task 7: 修复工具结果间接注入检测（P0-7）

  * [x] SubTask 7.1: 修改 `session/session.go` 的 `AddMessageWithMeta`，在写入 session 前调用 prompt injection detector（AddToolResult 通过 AddMessageWithMeta 流转，自动覆盖）

  * [x] SubTask 7.2: 修改 `AddMessageWithMeta` 在写入 session 前调用 PII masker

  * [x] SubTask 7.3: 确保 MCP 输出（通过 AddToolResult → AddMessageWithMeta 回传）经过同一检测链路

  * [x] SubTask 7.4: 添加测试：工具结果含注入模式触发告警、含 PII 被脱敏

* [x] Task 8: 修复 Anthropic 多轮 native tool call（P0-1）

  * [x] SubTask 8.1: 在 `model/anthropic.go` 的 `RequestModel` 中增加 `anthropicMessages` 转换函数，检测历史消息中的 `tool_calls` / `tool_call_id` 格式

  * [x] SubTask 8.2: 将 OpenAI 格式的 `tool_calls`（assistant 消息）转换为 Anthropic 的 `tool_use` content block

  * [x] SubTask 8.3: 将 OpenAI 格式的 `tool_call_id`（tool 消息）转换为 Anthropic 的 `tool_result` content block，合并连续 tool 结果保持 user/assistant 交替

  * [x] SubTask 8.4: 添加测试覆盖多轮 tool call 场景，验证 Anthropic API 请求格式正确

* [x] Task 9: 修复 ThreeZoneSession LoadJSON（P0-2）

  * [x] SubTask 9.1: 实现 `LoadJSON(path string) error` 方法，从 `SaveJSON` 写入的文件恢复三区状态

  * [x] SubTask 9.2: 确保恢复后 `immutablePrefix` / `appendOnlyHistory` / `volatileScratch` 数据完整

  * [x] SubTask 9.3: 添加测试：`SaveJSON` → `LoadJSON` 往返一致性

  * [x] SubTask 9.4: 选择实现 LoadJSON（非移除声明）

* [x] Task 10: 合并两套审批系统（P1-20）

  * [x] SubTask 10.1: 统一使用 `approval.PolicyApprovalManager` 作为审批接口

  * [x] SubTask 10.2: `sandbox/approval.go` / `sandbox/approval_test.go` 已移除

  * [x] SubTask 10.3: `SandboxExecutor` 通过 `ApprovalManager *approval.PolicyApprovalManager` 调用统一审批接口

  * [x] SubTask 10.4: `sandbox/` 中的 `ApprovalService` 独立实现已移除

  * [x] SubTask 10.5: `go build ./...` 和 `go test ./sandbox/... ./approval/...` 验证通过

* [x] Task 11: 提取 Provider SSE 公共逻辑（D7-4）

  * [x] SubTask 11.1: 新建 `model/internal/ssestream/` 包（使用 model/internal/ 而非 providers/internal/，因 provider 文件在 model 根包）

  * [x] SubTask 11.2: 提取 `EffectiveHTTPClient`、`RunLines[T]`（泛型 goroutine 骨架+emit 闭包+EOF/error 处理）、`ParseDataLine`、`MapRole` 到 ssestream 包

  * [x] SubTask 11.3: 重构 `openai.go` 复用 ssestream 包，仅保留自身特有逻辑

  * [x] SubTask 11.4: 重构 `anthropic.go` 复用 ssestream 包

  * [x] SubTask 11.5: 重构 `openai_responses.go` 复用 ssestream 包

  * [x] SubTask 11.6: `go test ./model/...` 三个 Provider 测试全部通过

* [x] Task 12: 提取 resize TotalContentBytes 辅助函数（D7）

  * [x] SubTask 12.1: 在 `session/resize.go` 新增 `TotalContentBytes(msgs []ChatMessage) int` 辅助函数

  * [x] SubTask 12.2: 重构 `SimpleCutResizeHandler` / `DefaultAnalysisHandler` / `Session.applyResizeLocked` / `ThreeZoneSession.totalHistoryBytes` 复用该函数

  * [x] SubTask 12.3: 运行 `go test ./session/...` 验证通过

* [x] Task 13: 拆分 model/ God Package（P0-9、P0-11、P0-12）— 部分完成

  * [x] SubTask 13.1: `model/attempt.go` 迁移到 `model/internal/attempt/attempt.go`，泛型化 `AttemptRunner[T Chunk]`

  * [x] SubTask 13.2: model 根包保留 re-export type alias（`AttemptRunner = attempt.AttemptRunner[*StreamChunk]` 等）保持向后兼容

  * [ ] SubTask 13.3: `failover.go` / `pool.go` 迁移到 `model/internal/` — **阻塞**：循环依赖（实现 ModelRequester 接口，引用核心类型），需先迁移核心类型到共享包

  * [ ] SubTask 13.4: Provider 文件迁移到 `model/providers/` 子包 — 未启动（高风险）

  * [ ] SubTask 13.5: 核心类型迁移到 `schema/` 包 — 未启动（高风险，是 13.3 的前置条件）

  * [x] SubTask 13.6: `go build ./...` 和 `go test ./...` 验证通过（attempt 迁移后）

  * **注**：failover/pool 迁移被循环依赖阻塞，核心类型迁移到 schema/ 是前置条件但属高风险操作，建议作为独立后续任务

* [x] Task 14: 拆分 orchestrator/agent/ God Package（P0-10）— 部分完成

  * [x] SubTask 14.1: `turn_loop.go` 迁移到 `internal/turnloop/`（type alias 保持兼容）

  * [x] SubTask 14.2: `cancel.go` 迁移到 `internal/cancel/`（test 一同迁移）

  * [x] SubTask 14.3: `session_ext.go` + `action_ext.go` 迁移到 `internal/extension/`

  * [ ] SubTask 14.4: hooks 逻辑迁移到 `hooks/` 子包 — 跳过（With\* 函数绑定 runConfig，无法独立）

  * [ ] SubTask 14.5: streaming 逻辑迁移到 `streaming/` 子包 — 跳过（StreamRun 是 Agent 方法，访问内部字段）

  * [ ] SubTask 14.6: extension 逻辑迁移到 `extension/` 子包 — 已迁移到 `internal/extension/`

  * [ ] SubTask 14.7: features 逻辑迁移到 `features/` 子包 — 未启动

  * [x] SubTask 14.8: `agent.go` 和 `engine.go` 作为公共 API 入口保留（零修改）

  * [x] SubTask 14.9: `go build ./...` 和 `go test ./orchestrator/...` 验证通过（含 race test）

* [x] Task 15: 拆分 ModelRequester 接口（P1-21）

  * [x] SubTask 15.1: 定义 `StreamRequester` 接口（`Name()` + `GenerateRequestData` + `RequestModel`）

  * [x] SubTask 15.2: 定义 `ResponseBroadcaster` 接口（`BroadcastResponse`）

  * [x] SubTask 15.3: 定义组合接口 `ModelRequester = StreamRequester + ResponseBroadcaster` 保持向后兼容

  * [x] SubTask 15.4: 修改 `engine.go` 和 `flow_context_impl.go` 使其仅依赖 `StreamRequester`

  * [x] SubTask 15.5: 添加测试验证仅实现 `StreamRequester` 的 mock 可被 engine 使用

* [x] Task 16: 拆分 SessionBackend 接口（P1-22）

  * [x] SubTask 16.1: 定义 `MessageStore` 接口（AddMessage / AddMessageWithMeta / PreparePrompt）

  * [x] SubTask 16.2: 定义 `SessionPersistor` 接口（SaveJSON；LoadJSON 因签名不兼容未纳入）

  * [x] SubTask 16.3: 定义 `ZoneManager` 接口（SetImmutablePrefix / ClearVolatileScratch / BuildPrompt）

  * [x] SubTask 16.4: 定义 `MaskableStore` 接口（SetMessageMasker）

  * [x] SubTask 16.5: 消除 `orchestrator/agent/session_ext.go` 中的 3 处类型断言，改为依赖拆分接口

  * [x] SubTask 16.6: `go test ./session/...` 验证通过

* [x] Task 17: inferflow 依赖适配

  * [x] SubTask 17.1: 检查 inferflow 所有 `github.com/inferglow/*` import — 全部兼容（type alias + 组合接口保持向后兼容）

  * [x] SubTask 17.2: `model.ModelRequest` / `model.ChatMessage` 等 import 兼容（核心类型未迁移，仍在 model 包）

  * [x] SubTask 17.3: `ModelRequester` 接口拆分对 inferflow stub 无影响（组合接口 `ModelRequester = StreamRequester + ResponseBroadcaster` 保持兼容）

  * [x] SubTask 17.4: `agent.New` / `agent.NewEngine` / `agent.NewSessionExtension` / `agent.NewActionExtension` import 正确（public API 未变，内部迁移用 alias）

  * [x] SubTask 17.5: `inferflow/` 目录 `go build ./...` 编译通过

  * [x] SubTask 17.6: `inferflow/` 目录 `go test ./...` 全部测试通过（16 个包全部 ok）

# Task Dependencies

* \[Task 0] 无依赖，必须最先执行

* \[Task 1] 独立，可在 Task 0 后并行执行

* \[Task 2] 独立，可在 Task 0 后并行执行

* \[Task 3] \~ \[Task 6] 独立，可在 Task 0 后并行执行（沙箱安全修复互不依赖）

* \[Task 7] 独立，可在 Task 0 后并行执行（session 防注入）

* \[Task 8] 独立，可在 Task 0 后并行执行（Anthropic 格式转换）

* \[Task 9] 独立，可在 Task 0 后并行执行（LoadJSON）

* \[Task 10] 依赖 \[Task 3] / \[Task 4]（审批系统合并需先完成策略基线）

* \[Task 11] 独立，可在 Task 0 后并行执行（SSE 提取）

* \[Task 12] 独立，可在 Task 0 后并行执行（resize 辅助函数）

* \[Task 13] 依赖 \[Task 1] / \[Task 2]（先清除死代码再拆分）和 \[Task 11]（SSE 提取后 Provider 文件结构清晰）

* \[Task 14] 依赖 \[Task 13]（model 拆分后 agent 的 import 才稳定）

* \[Task 15] 依赖 \[Task 13]（接口拆分需在类型迁移后）

* \[Task 16] 依赖 \[Task 7] / \[Task 9]（session 修改需先完成防注入和 LoadJSON）

* \[Task 17] 依赖 \[Task 13] / \[Task 14] / \[Task 15] / \[Task 16]（所有 inferglow 架构调整完成后才能适配 inferflow）

* 可并行：Task 1+2+3+4+5+6+7+8+9+11+12 在 Task 0 完成后可大部分并行

