# Tasks — R9

## Phase 0: subagent 实时状态监控接口前置（B0）
- [x] T0.1 builtins/actions：新增 subagent_registry.go — SubagentRegistry
      （Register/MarkDone/MarkError/List(bySession)，内存 map + mutex，可选快照持久化仿 TaskStore）；
      Spawn 记录 {ID, ParentSession, Task, Status, SystemPrompt, StartedAt, EndedAt, Result/Error}（Depth 字段实际未落：executor 拿不到引擎深度，面板不需要，YAGNI 裁掉）。
- [x] T0.2 builtins/actions/sub_agent.go：executor Execute 前后埋点（spawn 开始登记 running，
      结束 mark done/error）；Registry 通过构造配置注入（nil = 不埋点，向后兼容）；
      补/改单测。
- [x] T0.3 orchestrator/agent：RunAgent 顺序路径递增 depth（clone engine 或 ++/defer），
      补「嵌套 spawn 触发 MaxDepth」测试（修现存递归漏洞）。
- [x] T0.4 orchestrator/agent：executeLoop 裸路径安装 flow 上下文
      （ctx 无 flow context 时构造 flowContextImpl 并 WithFlowContext，参照 flow_exec.go:82 构造）；
      补「裸环 spawn_agent 可用」集成测试（echo 模型）。
- [x] T0.5 server/agent_factory.go：注册 spawn_agent + 注入 Registry；
      main.go 在 agent 装配前创建 Registry（R8 TaskStore 接线教训）并 SetSubagentRegistry。
- [x] T0.6 server/handlers_subagent.go：GET /v1/subagents?session=（列表，含 running）；
      router 注册（auth 语义同 /v1/tasks）。
- [x] T0.7 cd builtins/actions && go test；cd orchestrator && go test；cd server && go test 全绿。

## Phase 1: server 端补齐（workspace 重命名）
- [x] T1.1 server/handlers_workspace.go：新增 handleRenameWorkspace —
      PATCH /v1/workspaces/{name} {new_name}：Get(old) 取 root → Open(new, root) → Close(old)；
      404/同名/非法名报错；成功后 PersistWorkspaces。
- [x] T1.2 同函数内联动 sessionStore：workspace==old 的 SessionRecord 全部 UpdateMeta/直改为新名。
- [x] T1.3 server/router.go 注册路由（含 auth-open 既有语义）；server 新增
      handlers_workspace_rename_test.go（rename 成功/404/会话联动）。
- [x] T1.4 cd server && go test ./... 全绿。

## Phase 2: 前端 API 与 store
- [x] T2.1 api/client.ts：forkSession(id)（POST fork，返回新会话）、
      setSessionStatus(id, status)（PATCH status）、workspaceRename(name, newName)。
- [x] T2.2 store.ts：Settings 增 sidebarGroupBy('workspace'|'flat') / sidebarSort('recent'|'manual')，
      随 persistSettings 持久化；updateSetting 支持新键。
- [x] T2.3 bridge/inferglow.ts：forkSession（成功后 refreshSessions + select 新会话）、
      archiveSession(id, archived)、renameWorkspace（成功后 refreshWorkspaces + refreshSessions）。

## Phase 3: 视图选项菜单（元素1+2）
- [x] T3.1 新建 panels/ViewOptionsMenu.tsx（portal+fixed，复用 ConfigPopover 定位骨架）：
      「分组方式 / 排序方式」两个 section，单选打勾，点选即生效并关闭。
- [x] T3.2 Sidebar.tsx：视图选项按钮接 anchor + 菜单；读取 settings.sidebarGroupBy/SidebarSort
      驱动树渲染：flat = 平铺；recent = updatedAt 倒序；manual = 创建序。

## Phase 4: 会话树重构（元素3+5）
- [x] T4.1 Sidebar.tsx：删除「注册的 workspace」独立区块；组行改为
      folder 图标 + chevron + 组名 + rowActions（⋯ 菜单 + ＋）；组行点击 setActiveWorkspace。
- [x] T4.2 会话行：标题 + formatRelTime(updatedAt) + ⋯ 按钮（hover 显示）；
      移除行尾 trash 与双击重命名。
- [x] T4.3 util formatRelTime（src/lib/relTime.ts 或就近）：刚刚/N分钟/N小时/N天。

## Phase 5: 行操作菜单（元素4+5左）
- [x] T5.1 panels/RowActionsMenu.tsx（或复用 ViewOptionsMenu 骨架）：通用行菜单
      （items: {label, danger?}，onPick）。
- [x] T5.2 会话 ⋯ 菜单：归档/取消归档（setSessionStatus）、Fork（forkSession）、
      重命名（行内编辑输入框，回车 PATCH）、删除（confirm → deleteSession）。
- [x] T5.3 workspace ⋯ 菜单：重命名（行内输入 → workspaceRename）、删除（confirm → DELETE）。
- [x] T5.4 归档会话灰显样式（opacity/删除线视 DSH 观感）。

## Phase 6: 第七类工具「任务管理」
- [x] T6.1 PaneEmptyCards.tsx：TAB_TYPES 增 { kind:'subagent', label:'任务管理' }；
      TabIcon 补 subagent 分支图标。
- [x] T6.2 api/client.ts：listSubagents(sessionId)（GET /v1/subagents?session=）。
- [x] T6.3 新建 panels/SubagentPanel.tsx：两节——「子代理」读 listSubagents 轮询
      （状态点 running/done/error + 任务摘要 + 耗时 + 结果/错误）；「运行记录」读
      sessionTrace 持久化 + live 轮询（复用 ConvPanels useTraceData 合并思路，抽公共 util 如可行）；
      行 = 状态点 + agent_id · 起始时间 + 时长 · usage · error 截断；
      排序按 orderJobs 规则（进行中在前）；点击展开 spans 子行。
- [x] T6.4 PanePanels.tsx：'subagent' 分支替换静态 SubagentTree 为 SubagentPanel
      （删除死代码）。

## Phase 7: 待办编辑补全（B3）
- [x] T7.1 TodoPanel：条目 inline 编辑标题（编辑按钮或双击 → input → PATCH title）；
      核对新增/删除/三态循环链路；错误提示保持。

## Phase 8: 构建、实测、收尾
- [x] T8.1 cd webui-dsh && npm run build；cd server && go build ./cmd/inferglow-server；
      重启 server（-auth-open -pty 等按现行旗标）。
- [x] T8.2 browser-use 实测 checklist Must Pass 全项（工作区菜单/会话菜单/视图选项/
      任务管理/待办编辑），中文数据用 Python urllib 构造（坑 G14）。
- [ ] T8.3 全模块 go test；更新 plans 任务勾选；如遇新坑补踩坑记录。
- [ ] T8.4 git commit（消息：R9 侧栏列表组织对齐 + 第七类任务管理）。

## 执行备注
- T0.7 顺带修复既有 flaky：TaskStore.List 按 CreatedAt（unix 秒）排序，同秒并列时
  sort.Slice 不稳定导致顺序随机 → 并列按 ID 决胜（builtins/actions/task_tracker.go）。

## 执行备注（续）
- 实测修正：菜单按钮 setState 函数式更新器里引用 e.currentTarget 在 React 派发结束后为 null，
  弹层渲染到屏幕外 —— 已改为先捕获局部变量（G17）。
- 实测修正：无 workspace 字段的旧会话因 `?? activeWs` 回退同时出现在工作区组与未分组 →
  改为严格 `s.workspace === w.name`。
- spawn_agent 端到端实测通过：模型真调 spawn_agent，Registry running→done（约 3 分钟），
  面板实时行与归因正确；fork/重命名/归档/工作区重命名往返/视图选项持久化全部浏览器验证。
- 观察项（非 R9 回归，待查）：本 vLLM 后端所有 run 恰好 3分0秒 结束（疑似 streamTimeout），
  子 run result 为空、父 run 工具轮后最终回复为空（「(空回复)」）。建议后续核查 streamTimeout
  与 maxRounds 耗尽后的 synthesis 路径。
