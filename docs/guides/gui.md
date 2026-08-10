# InferGlow GUI 使用指南

InferGlow 的桌面 GUI 是一套 React 19 + Vite + Zustand + TypeScript 前端，构建产物
内嵌进 server（`//go:embed webui`），与 REST API 同源交付：浏览器打开 `/gui` 即用。

## 快速开始

```bash
# 1. 启动后端（可选 -demo-agent：内置 echo agent，无需真实模型即可体验聊天）
cd server
go run ./cmd/inferglow-server -demo-agent

# 2. 浏览器访问
#    http://localhost:8080/gui/
```

> 未启动后端时，GUI 自动进入 **Demo 模式**（顶栏显示"未连接"，输入框禁用），
> 仅展示布局；后端就绪后刷新即切回 live 模式。

## 开发工作流

```bash
cd web
npm install
npm run dev        # Vite dev server（:5173），/v1 与 /health 代理到 :8080
npm run lint       # eslint
npm run test       # vitest（SSE 解析器 / zustand stores）
npm run build      # tsc + vite build → ../server/webui（产物入库，Go embed 直接可用）
```

**产物入库约定**：`server/webui/`（vite build 输出）随 commit 提交，保证
`go build` 与 CI 无需 Node 工具链即可编译（与 `dashboard.html` 先例一致）；
`web/node_modules`、`web/dist` 不入库。

## 架构

### 通道架构（REST 主通道 + transport 抽象预留 Wails）

| 通道 | 状态 | 说明 |
|---|---|---|
| REST（`/v1/*`） | ✅ 本版实现 | `web/src/api/transport.ts` 的 `restTransport`：fetch JSON + SSE 流式 |
| Wails 绑定直调（`window.go.*` + `runtime.EventsOn`） | ⏳ 预留 | `detectTransport()` 运行时探测 `window.go?.desktop`；未来桌面壳（OT-10）只需新增一个 transport 实现，组件/stores 零改动 |

组件与 stores 只依赖 `Transport` 接口（`request` / `streamRun`），通道切换对上层透明。

### 目录结构

```
web/src/
├── api/        # types.ts（后端契约）/ transport.ts（通道抽象）/ sse.ts（SSE 解析）
├── stores/     # zustand：sessionStore / chatStore / usageStore
├── settings/   # settingsSchema（15 tab，对齐 settings-spec.md）/ SettingsPanel / serverData
├── theme/      # themes.ts（20 套主题）+ ThemeProvider（CSS 变量注入）
└── App.tsx     # 三层桌面布局：顶栏 + 会话栏 + 聊天主区 + dock + 状态栏 + 终端抽屉
```

## SSE 消费协议（关键契约）

聊天使用 `POST /v1/agents/{id}/stream-run`（SSE，`fetch` + `ReadableStream` 解析，
见 `web/src/api/sse.ts`）。消费方必须遵守两个**服务端契约怪癖**：

1. **`run_end.tool_name` 承载完整回复正文**——助手最终回复文本放在
   `run_end` 事件的 `tool_name` 字段中（server/handlers_stream.go 的
   `cb.emit("run_end", resp, ...)`），而非 `data` 字段。
2. **`run_start` 可能触发两次**（goroutine 手动 emit + `OnRunStart` 回调），
   前端需幂等（`chatStore` 已用 `runSeen` 去重）。

其他事件：`llm_start/llm_end`（思考帧与 tokens）、`tool_start/tool_end`（工具卡片
四态 run/ok/err）、`done`（收尾）。stream-run 事件通道有界（cap=32）且非阻塞，
高频工具事件可能被丢弃——前端以 `run_end` 为准做最终同步。

**降级**：stream-run 不可用（agent 未接线、404/5xx）时，`chatStore` 自动回退
`POST /v1/agents/{id}/chat` 同步端点。

## 设置面板（15 tab）

单一事实来源是 `prototypes/inferglow-gui/settings-spec.md`（4 分组 15 标签）。
数据源分两类：

- **服务端**（REST）：`credentials`→`/v1/credentials`；`schedules`→`/v1/schedules`
  （含 start/stop）；`skills-tools`→`/v1/skill-hub` + `/v1/tools`；
  `connectors`→`/v1/mcp-hub`；`providers` 用量→`/v1/usage/report`；
  `security` 审计链→`/v1/audit/verify`。
- **本地**（localStorage，key `inferglow.settings.v1`）：外观/界面/快捷键/权限/
  推理/实验等纯 UI 偏好；主题卡片点击即时切换 20 套主题。

## 新增端点（本 GUI 版引入）

| 端点 | 说明 |
|---|---|
| `PATCH /v1/sessions/{id}` | 会话元数据（title/group/pinned/status），指针语义：缺省字段不动、空值显式清空 |
| `GET /v1/sessions/{id}/messages?before=&limit=` | 聊天历史分页（最新在前；`next_before` 游标；空结果=到顶） |
| `GET /v1/usage/report?from=&to=&model=` | 用量聚合报表（复用 session.ReportGenerator，缺省本月窗口） |
| `GET /gui` / `GET /gui/{path...}` | GUI 静态资源（embed，中间件链外免鉴权，与 /dashboard 同级） |

聊天/流式端点新增可选请求字段 `session_id`：携带时消息落库到会话历史
（`msgStore` 未配置时行为不变，响应字节兼容）。

## 手测清单

1. `go run ./cmd/inferglow-server -demo-agent` → 打开 `/gui`
2. 新建会话 → 左侧出现新会话
3. 发送消息 → SSE 流式上屏（`demo echo: ...`）→ 工具卡（如有）状态流转
4. 切换会话 → 历史消息回读（来自 `GET /v1/sessions/{id}/messages`）
5. 长会话向上滚动 → 游标分页加载更早消息
6. 右键会话 → 置顶/归档/删除（`PATCH/DELETE /v1/sessions/{id}`）
7. 设置 → 外观：点击主题卡片即时换肤，刷新后保留（localStorage）
8. 设置 → 凭证/调度/技能/连接器：服务端列表真实渲染
9. 设置 → 提供方：用量报表数字（`/v1/usage/report`）
