# InferGlow GUI：基座计划（后端）+ 界面计划（HTML 原型）Spec

## Why

`prototypes/GUI功能路线图.md` 已给出 InferGlow 三层桌面 GUI 的完整路线图（Phase 0–5），但内容庞大、无法一次落地。需要拆成两条可独立推进的轨道：

- **基座计划**：先把后端能力补齐（路线图第 7 节缺口 + desktop 模块激活），让 GUI 有可靠的数据底座。
- **界面计划**：先用 HTML 静态原型把「合理界面」做出来供浏览器预览，确定交互后再考虑 React 实现。预览先行、执行后置。

## What Changes

### A. 基座计划（后端实现）
- 补齐会话元数据扩展（分组/置顶/重命名/归档字段 + `PATCH /v1/sessions/{id}`）。
- 新增聊天历史分页 `GET /v1/sessions/{id}/messages?before=&limit=`。
- 复用 `POST /v1/runs/{id}/input` 承载线程级审批决策（允许/拒绝）。
- 暴露用量聚合报表端点（复用 `session.LoadUsage` + `ReportGenerator`）。
- 激活 desktop 模块：补 `desktop/main.go`（`wails.Run`），把 `shell.go` 的 `StartSession/SendChat` 桩代理到 `server/` REST。

### B. 界面计划（HTML 静态原型）
- 在 `prototypes/` 下产出一份「InferGlow 三层桌面 GUI」可交互 HTML 静态原型（复用 openhanako/reasonix 的还原范式）。
- 覆盖聊天主界面 + 会话管理 + 上下文环 + 工具卡片 + 设置面板等核心屏。
- 遵循 `prototypes/原型交互与还原原则.md`：只还原真实能力、演示跳转条用虚线边框+珊瑚色标注、弱交互完善。
- 预览确认后，再考虑是否进入 React 实现（本 spec 不实现 React，仅评估）。

## Impact
- Affected code（基座）: `inferglow/server/session_store.go`、`handlers_session.go`、`handlers_stream.go`、`run_manager.go`、`handlers_observability.go`、`desktop/shell.go`、新增 `desktop/main.go`。
- Affected files（界面）: `prototypes/` 下新增原型目录与 `index.html`。
- No breaking changes；新增端点向后兼容。

## ADDED Requirements

### Requirement: 会话元数据扩展
系统 SHALL 支持会话的分组、置顶、重命名、归档元数据，并提供 `PATCH /v1/sessions/{id}` 更新。

#### Scenario: 分组与置顶
- **WHEN** 客户端 PATCH 会话的分组字段或置顶标志
- **THEN** 会话列表按分组返回，置顶项排在最前

### Requirement: 聊天历史分页
系统 SHALL 提供 `GET /v1/sessions/{id}/messages?before=&limit=` 分页拉取历史消息。

#### Scenario: 长会话加载
- **WHEN** 客户端传入 before 时间戳与 limit
- **THEN** 返回该时间点之前的最多 limit 条消息，可用空结果判断已到顶

### Requirement: 线程级审批
系统 SHALL 复用 `POST /v1/runs/{id}/input` 承载审批决策（允许/拒绝），GUI 点按钮后回填。

#### Scenario: 审批决策回填
- **WHEN** run 处于审批阻塞态，客户端 POST 允许/拒绝
- **THEN** 该 run 恢复执行或终止，SSE 事件反映状态变更

### Requirement: 用量聚合报表
系统 SHALL 暴露用量聚合端点，返回跨会话成本/缓存/Token 统计。

#### Scenario: 报表拉取
- **WHEN** 客户端请求用量聚合
- **THEN** 返回按会话/时间聚合的成本与缓存命中统计

### Requirement: desktop 模块激活
系统 SHALL 提供 `desktop/main.go` 启动 Wails 壳，`StartSession/SendChat` 代理到 server REST。

#### Scenario: 桌面启动连通
- **WHEN** `wails build` 后打开桌面应用并发送消息
- **THEN** 请求转发到 server `/v1/agents/{id}/chat`，返回真实回复

### Requirement: InferGlow GUI HTML 静态原型
原型 SHALL 提供可交互的三层桌面 GUI 预览，覆盖聊天/会话/上下文环/工具卡片/设置。

#### Scenario: 浏览器预览
- **WHEN** 打开 `prototypes/.../index.html`
- **THEN** 聊天可输入、工具卡片可展开、设置标签可切换、演示跳转条带「原型演示」标记

### Requirement: 富输入文件拖拽 dir 标签 chip
Composer SHALL 支持文件/产物拖入，底层自动生成 `dir` 标记，UI 渲染为带删除按钮的标签 chip。

#### Scenario: 拖入文件生成 chip
- **WHEN** 用户拖入文件/产物到 Composer
- **THEN** 底层自动生成 `` `dir: /path/to/file` `` 标记，UI 渲染为带 ✕ 的标签 chip，与普通文本输入视觉区分

#### Scenario: 点击 ✕ 删除 chip
- **WHEN** 用户点击 chip 上的 ✕
- **THEN** chip 从 UI 移除，底层对应的 `` `dir` `` 结构被删除

## MODIFIED Requirements
（无）

## REMOVED Requirements
（无）