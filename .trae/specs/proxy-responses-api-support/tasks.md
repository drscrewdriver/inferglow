# Tasks — Responses API 支持

> 基线：`proxy-compressor-auditor` 已完成的 Go Phase 1 + Python 镜像。本任务在其上**纯新增** `/v1/responses` 分支，不重构既有 Chat 路径。

## 阶段 A：Go 实现（先行，作为协议语义样板）

- [x] Task 1: Go 审计模型扩展 — `go/internal/audit/models.go` 增加 `Protocol`（chat/responses）、`ResponseID`、`PreviousResponseID`、`ConversationID` 字段；DB 迁移（PRAGMA table_info 检查 + ALTER ADD COLUMN，仿照 storage 既有迁移模式）
  - [x] Sub 1.1: AuditRecord 新增字段 + JSON tag
  - [x] Sub 1.2: 建表/迁移逻辑同步

- [x] Task 2: Go schema 校验 — `go/internal/proxy/validate.go` 新增 `ResponsesRequest` 结构体 + 校验（model 必填、input 非空 string/items、store/previous_response_id 类型）
  - [x] Sub 2.1: ResponsesRequest + ResponsesInputItem 结构体
  - [x] Sub 2.2: ValidateResponsesRequest 函数（校验失败不阻塞审计）

- [x] Task 3: Go Responses 代理 — `go/internal/proxy/proxy.go` 新增 `handleResponses` + `RegisterRoutes` 注册 `POST /v1/responses`
  - [x] Sub 3.1: 非流式转发（复用 forward 逻辑，usage 用 input/output 映射）
  - [x] Sub 3.2: 流式事件流处理（聚合 output_text.delta，detect completed/failed）
  - [x] Sub 3.3: session 关联扩展（conversation_id → previous_response_id → user message fallback）
  - [x] Sub 3.4: protocol=responses 标记 + response_id 捕获

- [x] Task 4: Go 编译验证 — `go build ./cmd/proxy-compressor` 通过，`/v1/responses` 路由注册成功

## 阶段 B：Python 镜像实现

- [x] Task 5: Python schema — `py/src/proxy/schema.py` 新增 `ResponsesRequest` Pydantic 模型（model/input/instructions/store/previous_response_id）
- [x] Task 6: Python Responses 代理 — `py/src/proxy/handler.py` 新增 `@router.post("/v1/responses")`
  - [x] Sub 6.1: 非流式转发 + usage 映射
  - [x] Sub 6.2: 流式事件流（聚合 output_text.delta，detect completed/failed）
  - [x] Sub 6.3: session 关联扩展（复用 storage 的 session 计算入口）
  - [x] Sub 6.4: protocol=responses 标记

- [x] Task 7: Python session 扩展 — `py/src/audit/storage.py` 扩展 session 派生优先级（conversation_id → previous_response_id → user message）；AuditRecord 模型加 protocol/response 字段 + 迁移

## 阶段 C：验证

- [x] Task 8: Go 端到端 — 起 Go 服务，curl 非流式/流式 `/v1/responses`，核对审计记录（protocol、usage、output_text、session_id）
- [x] Task 9: Python 端到端 — 起 py 服务，同样验证 Responses 全链路
- [x] Task 10: Chat 回归 — 确认 `/v1/chat/completions`（流式/非流式）行为不变，协议分支互不干扰
- [x] Task 11: Dashboard/查询 protocol 过滤（可选）— 列表按 protocol 过滤

# Task Dependencies

- [Task 1] 无（基础字段）
- [Task 2] 无（独立结构体）
- [Task 3] 依赖 [Task 1, Task 2]
- [Task 4] 依赖 [Task 3]
- [Task 5] 无
- [Task 6] 依赖 [Task 5]；session 部分依赖 [Task 7]
- [Task 7] 无（可与 Task 5/6 并行）
- [Task 8] 依赖 [Task 4]
- [Task 9] 依赖 [Task 6, Task 7]
- [Task 10] 依赖 [Task 8, Task 9]
- [Task 11] 依赖 [Task 8, Task 9]
