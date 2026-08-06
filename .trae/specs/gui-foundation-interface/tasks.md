# Tasks

## 基座计划（后端实现）

- [x] Task 1: 会话元数据扩展
  - [x] 1.1 在 `server/session_store.go` 增加 metadata 字段（group/pinned/archive/name）
  - [x] 1.2 新增 `PATCH /v1/sessions/{id}` 更新这些字段
  - [x] 1.3 会话列表按分组返回、置顶项排前
  - [x] 1.4 补充 `session_store_test.go` 单元测试
- [x] Task 2: 聊天历史分页
  - [x] 2.1 新增 `GET /v1/sessions/{id}/messages?before=&limit=`
  - [x] 2.2 返回 before 时间点之前最多 limit 条消息，空结果表示到顶
  - [x] 2.3 补充分页测试
- [x] Task 3: 线程级审批
  - [x] 3.1 确认 `POST /v1/runs/{id}/input` 可承载审批决策（允许/拒绝）
  - [x] 3.2 若缺字段则扩展 input payload 区分审批决策
  - [x] 3.3 审批后 run 恢复/终止，SSE 事件反映状态变更
  - [x] 3.4 补充审批回填测试
- [x] Task 4: 用量聚合报表
  - [x] 4.1 复用 `session.LoadUsage` + `ReportGenerator` 暴露聚合端点
  - [x] 4.2 返回按会话/时间聚合的成本与缓存命中统计
  - [x] 4.3 补充聚合测试
- [x] Task 5: desktop 模块激活
  - [x] 5.1 新增 `desktop/main.go`（`wails.Run`）
  - [x] 5.2 把 `shell.go` 的 `StartSession/SendChat` 桩代理到 server REST
  - [x] 5.3 `wails build` 通过，桌面能连通 `/v1/agents/{id}/chat`

## 界面计划（HTML 静态原型）

- [x] Task 6: InferGlow GUI 静态原型
  - [x] 6.1 在 `prototypes/` 规划原型目录结构（复用 openhanako/reasonix 还原范式）
  - [x] 6.2 产出可交互 `index.html`：聊天主界面（输入/流式回显/工具卡片）
  - [x] 6.3 产出会话管理 + 上下文环（SVG）+ 设置面板（标签联动）
  - [x] 6.4 遵循 `原型交互与还原原则.md`：演示跳转条虚线边框+珊瑚色标注、弱交互完善
  - [x] 6.5 浏览器验证：聊天可输入、工具卡片可展开、设置标签切换正常
- [ ] Task 7: 富输入文件拖拽 dir 标签 chip
  - [ ] 7.1 Composer 容器从纯 textarea 升级为富输入容器，支持拖入感知
  - [ ] 7.2 拖入文件/产物时自动生成 `` `dir: /path/to/file` `` 底层标记
  - [ ] 7.3 UI 渲染为带 ✕ 的标签 chip，与普通文本输入视觉区分
  - [ ] 7.4 点击 ✕ 删除 chip 并清理底层 `` `dir` `` 结构

# Task Dependencies
- [x] [Task 2] depends on [Task 1]（历史消息需带会话元数据）
- [x] [Task 3] depends on [Task 1]（审批状态与会话/run 关联）— 可并行
- [x] [Task 4] 独立，可并行
- [x] [Task 5] 独立于 1–4，可并行
- [x] [Task 6]（界面）独立于基座，可随时并行；预览确认后再评估 React 实现