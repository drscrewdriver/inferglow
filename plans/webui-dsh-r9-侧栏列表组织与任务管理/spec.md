# Spec: R9 — 侧栏列表组织对齐 DSH + 第七类工具「任务管理」

## 背景（用户指正的两处偏差）

1. **侧栏归类方式不对**：R8 做成了「workspace 管理区 + 会话分组区」两段式；DSH 的真实形态是
   **一棵按 workspace 归组的会话树**（组行 = workspace 行，带 ⋯ 操作菜单与 ＋ 新建会话，
   组下直接是该 workspace 的会话行）。需要按 DSH 的五个元素对齐：
   - 元素1：视图选项按钮（列表展示模式切换入口）
   - 元素2：视图选项菜单（分组方式：按工作区/单列表；排序方式：手动排序/最近更新）
   - 元素3：按 workspace 归组的会话树（folder 图标 + chevron + 组标题 + 行操作）
   - 元素4：会话行 ⋯ 按钮 → 菜单管理会话的**归档、fork、重命名**（及删除）
   - 元素5：workspace 行右侧两个按钮：**⋯（重命名/删除工作区菜单）** 与 **＋（在该 workspace 新建会话）**
2. **任务管理被错误替换成待办**：底部面板「新建标签页」菜单原有 6 类
   （文件/源代码管理/待办/侧边对话/终端/浏览器），用户要**新增第 7 类「任务管理（子 agent 管理）」**，
   而不是把原「任务管理」改名成「待办」。
   - **待办**（tasks）：只是对接 server 待办列表（/v1/tasks）的缓存与维护，保留现状并补全编辑能力。
     DSH 中待办显示在输入框上部——**不学习该布局**，待办继续放在面板里。
   - **任务管理**（subagent）：具体功能参考 butter-side-bar 的 Subagent 页
     （目录行：状态点 + 标签 + 副文本；后台任务节：进行中在前、时长计时、终止确认、输出查看）。

## 需求

### A. 侧栏对齐（webui-dsh/src/app/layout/Sidebar.tsx 重构）
- A1 视图选项按钮点击打开 portal 菜单（元素1+2）：两组单选——
  分组方式 `按工作区`（默认）/`单列表`；排序方式 `最近更新`（默认）/`手动排序`。
  选择持久化到 store.settings（localStorage）。
- A2 会话树重构（元素3+5）：移除独立「注册的 workspace」区块；树 = 每 workspace 一个组
  （folder 图标 + chevron + 组名 + ⋯ + ＋），组下是该 workspace 的会话行
  （标题 + 相对时间 + ⋯）；无 workspace 字段的旧记录进「未分组」组。
  点击组行 = setActiveWorkspace。
- A3 会话行 ⋯ 菜单（元素4）：归档 / Fork / 重命名 / 删除。行内不再用双击+prompt。
  - 归档：PATCH status=archived；已归档会话灰显，菜单变「取消归档」。
  - Fork：POST /v1/sessions/{id}/fork，成功后刷新列表并选中新会话。
  - 重命名：菜单触发行内编辑输入框，回车确认（PATCH title，store.renameSession 已有）。
  - 删除：确认后 DELETE（沿用现有 deleteSession）。
- A4 workspace ⋯ 菜单（元素5左）：重命名工作区 / 删除工作区。
  - server 新增 PATCH /v1/workspaces/{name} {new_name}：
    Get(old) 取 root → Open(new, root) → Close(old)；同步把 sessionStore 中
    workspace==old 的记录改写为新名；持久化快照。
  - 删除工作区：DELETE /v1/workspaces/{name}（已有）+ confirm。
- A5 ＋按钮（元素5右）：行为沿用 R8（setActiveWorkspace + createSession(w.name)）。

### B0. subagent 实时状态监控接口前置（2026-09-07 追加，用户指令）
考察结论：底层有 spawn_agent/RunAgent 但只装在 flow 编排路径；CLI 注册了但裸聊天环缺
flow 上下文（实跑报错）；server 未注册；执行为同步内联、无生命周期句柄；server 已有
/v1/runs、/v1/jobs(+SSE) 监控面但只覆盖命名 flow 运行。前置工作：
- B0.1 SubagentRegistry（builtins/actions，仿 TaskStore 共享单例）：spawn 埋点登记
  {ID, ParentSession, Task, Status, Depth, StartedAt, EndedAt, Result/Error}。
- B0.2 执行链打通：executeLoop 裸路径安装 flow 上下文（orchestrator 一处改）；
  server/agent_factory 注册 spawn_agent；main.go 在 agent 装配前接线 Registry。
- B0.3 深度修复：RunAgent 顺序路径递增 depth + 递归截断测试（现存漏洞）。
- B0.4 查询面：GET /v1/subagents?session=；SSE 二期，面板 v1 轮询。

### B. 第七类工具「任务管理」（子 agent 管理）
- B1 TAB_TYPES 增 `{ kind: 'subagent', label: '任务管理' }`（第 7 项）；TabIcon 补 subagent 图标；
  DEFAULT_BOTTOM_TABS 不默认加入，通过 ＋ 菜单打开；待办 tab 保留（label 待办）。
- B2 SubagentPanel 真实实现（替换 PanePanels 里的静态 SubagentTree），**两节显示**：
  「子代理」节读 GET /v1/subagents?session=（B0 前置落地的真数据：状态点 running/done/error、
  任务描述、耗时、结果/错误摘要）；「运行记录」节 = 当前会话 run 摘要
  （live 轮询 + ListTraces 持久化 trace，复用 ConvPanels.useTraceData 的合并逻辑）：
  - 每 run 一行：状态点（运行中=蓝ongoing / 出错=红error / 完成=绿done）+ 主标签（agent_id · 起始时间）
    + 副文本（时长 · token 用量 · 错误信息截断）。
  - 排序参考 butter-side-bar orderJobs：进行中在前（按开始序），已结束的按结束时间倒序。
  - 点击行展开该 run 的 spans 子行（llm/tool，名称 + 耗时 + 错误标红）。
  - 空态：「当前会话暂无运行记录，发送消息后这里显示每次 agent run」。
- B3 待办面板编辑补全（增/删/改/标完成核对）：TodoPanel 现有 新增/删除/状态循环/刷新，
  **补 inline 编辑标题**（双击或编辑按钮 → 输入框 → PATCH title，server 已支持）；
  核对标注完成（cycle 三态）与删除链路可用。

## 技术方案

- 菜单弹层全部走 ConfigPopover 已验证的 portal+fixed 形态（坑 G6）；视图选项菜单是
  「分组标题 + 多组单选」形态，在 ConfigPopover 基础上新增分组渲染（`groups?: {label, items}[]`），
  或新建轻量 SidebarMenu 组件——实现时二选一，优先扩展 ConfigPopover。
- 相对时间：util 函数 formatRelTime(ms)（<1min 刚刚 / <1h N分钟 / <24h N小时 / N天）。
- 视图偏好：Settings 增 `sidebarGroupBy: 'workspace' | 'flat'`、`sidebarSort: 'recent' | 'manual'`，
  走现有 persistSettings。单列表 = 平铺全部会话（手动排序 = 按创建序，最近更新 = updatedAt 倒序）；
  手动排序 v1 不做拖拽（后端无 order 字段，YAGNI）。
- 会话/工作区菜单状态：Sidebar 本地 state（anchor 元素 + 目标对象），菜单组件无业务逻辑。
- api/client.ts 增：forkSession、patchSessionStatus（archive/unarchive）、workspaceRename。
- server：handlers_workspace.go 增 handleRenameWorkspace + 路由 PATCH /v1/workspaces/{name}。

## 决策记录

| 选项 | 选择 | 理由 |
|------|------|------|
| subagent 监控 v1 | Registry + 同步执行的埋点 + 轮询查询 | spawn_agent 仍是同步内联（改异步执行是编排层大工程）；「运行中」行靠面板轮询可见，够 v1 |
| flow 上下文安装位置 | engine.executeLoop 裸路径统一安装 | 一处改动 CLI/server 同时受益；对比在 server 装需要 orchestrator 暴露构造接口，侵入面更大 |
| 任务管理数据源 | 子代理行（真数据）+ run 摘要行（兜底）双节 | Registry 只在模型实际 spawn 后有数据；run 摘要保证面板任何时候都有内容 |
| 任务管理数据源（v1 原案，已被双节方案取代） | —— | 见上：B0 前置后改为「子代理行 + run 摘要行」双节 |
| 待办布局 | 保留底部面板，不动 composer | 用户明确「没打算让你学习 dsh 输入框上部的布局」 |
| 归档会话可见性 | 灰显留在原组 + 菜单可取消归档 | 不引入新的过滤开关（YAGNI），但归档不能等于消失 |
| 手动排序 | v1 = 创建顺序，无拖拽 | 后端 SessionRecord 无排序字段 |
| 重命名交互 | 菜单触发行内输入框 | 替换双击+prompt（DSH 形态），避免 prompt 阻塞 UI |
| workspace 重命名连带 | 同步改写会话记录的 workspace 字段 | 否则改名的 workspace 下会话全部掉进未分组 |

## 约束
- 前端改动必须 `vite build` + `go build` + 重启 server 才生效（坑 G5）。
- 弹层不得用普通 absolute 定位（坑 G6），一律 portal+fixed。
- 测试脚本发中文用 Python urllib（坑 G14），不用 Git Bash curl。
- go test 须 cd 进各模块目录。
- 本轮只动 webui-dsh 前端 + server 两处小端点，不碰 orchestrator/cli。
