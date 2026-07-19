# Checklist

## 阶段 0：master 备份归档
- [x] tag `pre-arch-adjust-20260724` 已创建，指向 commit `3642d93`
- [x] tag 已推送到 origin
- [x] tag message 包含审计报告路径（`docs/architecture-consistency-audit.md`）和回滚说明

## 阶段 1：删除死代码
- [x] `components/tool/` 整包已删除（5 个 .go 文件，保留 components/go.mod 因 prompt 子包仍存在）
- [x] 全局搜索确认无包引用 `github.com/inferglow/components/tool`
- [x] `model.ActionResult` 死类型已删除
- [x] `ModelRequest.Actions` 死字段已删除
- [x] `go build ./...` 在 inferglow 根模块通过
- [x] `go test ./model/...` 通过

## 阶段 2：P0 安全声明与实现脱节修复
- [x] `buildPolicyFromInput` 引入 `ServerPolicyBaseline`，LLM 参数只能在基线范围内收紧
- [x] `approval_required` 改由服务端策略决定，不从 input map 读取
- [x] Docker 后端根据 `network_access` 设置 `NetworkDisabled`
- [x] gVisor 后端根据 `network_access` 配置网络隔离
- [x] `ModeAuto` 不再自动回落到 `trusted_local`，仅显式配置允许
- [x] 新增测试：LLM 放宽策略被拒、LLM 收紧策略被接受
- [x] 新增测试：LLM 传 `approval_required: false` 但服务端要求审批时仍触发
- [x] 新增测试：`network_access: "none"` 时网络被禁用
- [x] 新增测试：未配置 `allow_trusted_fallback` 时 ModeAuto 无可用后端返回错误

## 阶段 3：P0 间接注入修复
- [x] `AddMessageWithMeta` 在写入 session 前调用 prompt injection detector（AddToolResult 通过 AddMessageWithMeta 自动覆盖）
- [x] `AddMessageWithMeta` 在写入 session 前调用 PII masker
- [x] MCP 输出经过同一检测链路
- [x] 新增测试：工具结果含注入模式触发告警
- [x] 新增测试：工具结果含 PII 被脱敏

## 阶段 4：P0 数据链路修复
- [x] Anthropic `RequestModel` 中 `anthropicMessages` 按 Provider 分支转换 `tool_calls` / `tool_call_id` 为 content block 格式
- [x] `tool_calls`（assistant 消息）转换为 `tool_use` content block
- [x] `tool_call_id`（tool 消息）转换为 `tool_result` content block
- [x] 新增测试：多轮 tool call 场景 Anthropic API 请求格式正确
- [x] `ThreeZoneSession.LoadJSON` 已实现
- [x] 新增测试：`SaveJSON` → `LoadJSON` 往返一致性

## 阶段 5：合并重复逻辑
- [x] 两套审批系统已合并为统一 `approval.PolicyApprovalManager`
- [x] `sandbox.ApprovalService` 独立实现已移除
- [x] `SandboxExecutor` 调用统一审批接口
- [x] `go test ./sandbox/... ./approval/...` 通过
- [x] `model/internal/ssestream/` 公共包已创建
- [x] `EffectiveHTTPClient` / `RunLines[T]`（goroutine 骨架+emit+EOF/error） / `ParseDataLine` / `MapRole` 已提取到 ssestream
- [x] `openai.go` / `anthropic.go` / `openai_responses.go` 复用 ssestream 包
- [x] `go test ./model/...` 三个 Provider 测试全部通过
- [x] `session/TotalContentBytes` 辅助函数已创建
- [x] 4 处重复循环复用 `TotalContentBytes`（SimpleCutResizeHandler / DefaultAnalysisHandler / Session.applyResizeLocked / ThreeZoneSession.totalHistoryBytes）
- [x] `go test ./session/...` 通过

## 阶段 6：God Package 拆分
- [ ] `ModelRequest` / `ModelResponse` / `ChatMessage` 等核心类型迁移到 `schema/` 包 — 未完成（高风险，阻塞 failover/pool 迁移）
- [ ] `model/` 包保留 re-export type alias — 部分完成（attempt 已有 alias）
- [x] `attempt.go` 已迁移到 `model/internal/attempt/`（泛型化 `AttemptRunner[T Chunk]`）
- [ ] `failover.go` / `pool.go` 迁移到 `model/internal/` — 阻塞（循环依赖）
- [ ] Provider 文件迁移到 `model/providers/` 子包 — 未启动（高风险）
- [x] `model/` 包保留接口定义、工厂函数和 re-export
- [x] `orchestrator/agent/` turnloop 逻辑迁移到 `internal/turnloop/`
- [ ] `orchestrator/agent/` step 逻辑迁移到 `internal/step/` — 未发现独立 step 文件
- [ ] `orchestrator/agent/` strategy 逻辑迁移到 `internal/strategy/` — 未发现独立 strategy 文件
- [ ] `orchestrator/agent/` hooks 逻辑迁移到 `hooks/` 子包 — 跳过（With* 绑定 runConfig）
- [ ] `orchestrator/agent/` streaming 逻辑迁移到 `streaming/` 子包 — 跳过（StreamRun 是 Agent 方法）
- [x] `orchestrator/agent/` extension 逻辑迁移到 `internal/extension/`（session_ext + action_ext）
- [ ] `orchestrator/agent/` features 逻辑迁移到 `features/` 子包 — 未启动
- [x] `agent.go` 和 `engine.go` 作为公共 API 入口保留（零修改）
- [x] `model/internal/` 目录下包不对外暴露，外部模块无法 import
- [x] `orchestrator/agent/internal/` 目录下包不对外暴露
- [x] `go build ./...` 全量编译通过
- [x] `go test ./...` 全量测试通过（含 race test）

## 阶段 7：接口拆分
- [x] `StreamRequester` 接口已定义（`Name()` + `GenerateRequestData` + `RequestModel`）
- [x] `ResponseBroadcaster` 接口已定义（`BroadcastResponse`）
- [x] `ModelRequester` 作为组合接口保持向后兼容
- [x] `engine.go` 和 `flow_context_impl.go` 仅依赖 `StreamRequester`
- [x] 新增测试：仅实现 `StreamRequester` 的 mock 可被 engine 使用
- [x] `MessageStore` 接口已定义
- [x] `SessionPersistor` 接口已定义（SaveJSON；LoadJSON 因签名不兼容未纳入）
- [x] `ZoneManager` 接口已定义
- [x] `MaskableStore` 接口已定义
- [x] `orchestrator/agent/session_ext.go` 中 3 处类型断言已消除
- [x] `go test ./session/...` 通过

## 阶段 8：inferflow 依赖适配
- [x] inferflow 所有 `github.com/inferglow/*` import 已检查 — 全部兼容
- [x] inferflow 的 `model.ModelRequest` / `model.ChatMessage` 等 import 兼容（核心类型未迁移）
- [x] inferflow 的 stub ModelRequester 实现适配拆分后的接口（组合接口保持兼容，无需改动）
- [x] inferflow 的 `agent.New` / `agent.NewEngine` / `agent.NewSessionExtension` / `agent.NewActionExtension` import 正确
- [x] `inferflow/` 目录 `go build ./...` 编译通过
- [x] `inferflow/` 目录 `go test ./...` 全部测试通过（16 个包全部 ok）
