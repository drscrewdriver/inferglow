# InferGlow Desktop GUI 架构设计

> 创建时间：2026-07-31 | 状态：草案
> 详细接口定义见：[Desktop_GUI_接口定义_detail.md](./Desktop_GUI_接口定义_detail.md)

---

## 1. 技术选型决策

### 1.1 Desktop 框架：**Wails v2**

| 维度 | Wails v2 | Tauri v2 | Electron |
|------|----------|----------|----------|
| 后端语言 | **Go（原生）** | Rust | Node.js |
| 包体积 | ~10-15 MB | ~5-8 MB | ~150+ MB |
| 集成度 | **✅ 零摩擦内嵌 orchestrator** | 需跨语言 IPC | 需独立 sidecar |
| 团队经验 | **✅ 已有构建配置** | 无 | 无 |

**决策理由**：核心全 Go，Wails 可零 IPC 开销直接调用 orchestrator；团队已有 webkit2gtk 兼容方案。

### 1.2 前端框架：**React 18 + TypeScript + Vite**

**决策理由**：流式 token 渲染需精细 DOM 控制（Concurrent Mode）；Markdown/LaTeX/Mermaid 生态最成熟；Zustand 事件驱动模型与 Agent 事件流天然契合。

### 1.3 通信方式：**Wails Binding + WebSocket 混合**

| 场景 | 方式 | 理由 |
|------|------|------|
| CRUD、配置读写 | Wails Binding（同步 RPC） | 类型安全、低延迟 |
| token/reasoning 流、工具事件 | WebSocket | 高频单向推送 |
| 审批交互 | WebSocket 双向 | server→client 推送 + client→server 回复 |

### 1.4 Server 部署模式：**内嵌优先，兼容独立运行**

- **内嵌模式**（默认）：Wails 进程内直接创建 Agent 实例
- **独立模式**（可选）：`inferglow server` 独立进程，前端通过 WS+REST 连接
- 两种模式共享同一套前端代码，通过 `ConnectionMode` 配置切换

---

## 2. Server 能力差距分析

### 2.1 已具备 ✅

Agent CRUD、Memory CRUD+搜索、Flow CRUD、Run 生命周期（pause/resume/cancel）、Run SSE 事件流、Trigger/Webhook、输入抢占（queue/safe_point/force）、健康检查、OpenAPI

### 2.2 关键缺失 🔴

| 缺失能力 | 级别 | 核心问题 |
|---------|------|---------|
| **Token 级流式** | 🔴 P0 | `streamCallbacks` 未实现 `OnToken`/`OnReasoning`，SSE 实质是"完成后一次性发" |
| **审批 API** | 🔴 P0 | 无路由无 handler，`EventApproval` 事件被丢弃 |
| **Session 完整 API** | 🔴 P0 | `GET /v1/sessions/{id}` 返回硬编码 stub，无列表/历史/transcript |
| **Reasoning 流式** | 🔴 P0 | `EventReasoning` 存在但 server 完全不暴露 |
| **WebSocket** | 🟡 P1 | 零实现，双向通信只能 POST 轮询 |
| **Transcript 持久化** | 🟡 P1 | TUI 在内存维护，server 无存储 |
| **Tools 列表** | 🟡 P1 | `GET /v1/tools` 返回空数组 |

### 2.3 核心断链

`handleStreamRun` 调用 `agent.Run()` 时**未注入 `WithCallbacks(cb)`**，`streamCallbacks` 所有方法都是死代码。修复：注入 callbacks + 新增 `OnToken`/`OnReasoning` 回调。

---

## 3. 系统架构

```
┌────────────────────────────────────────────────────┐
│              Frontend (React + Vite)                │
│  Pages: Chat | Sessions | Memory | Agents | Settings│
│  State: Zustand (chat/session/memory/agent/ui)     │
│  Connection: WailsBinding | WSClient | RESTClient   │
├────────────────────────────────────────────────────┤
│              Go Backend (Wails Runtime)             │
│  ┌──────────┐ ┌──────────┐ ┌────────────────────┐  │
│  │ Bindings │ │  WS Hub  │ │ REST (远程模式可选) │  │
│  └────┬─────┘ └────┬─────┘ └─────────┬──────────┘  │
│       └─────────────┼─────────────────┘             │
│         Application Service Layer                   │
│         (AgentService | SessionService | MemorySvc) │
├────────────────────────────────────────────────────┤
│  orchestrator/agent | session/ | memory/ | context/ │
└────────────────────────────────────────────────────┘
```

### 3.1 Go 模块结构

```
inferglow/desktop/
├── app.go              # Wails App，暴露 Binding 方法
├── bindings.go         # Binding 方法实现
├── ws_hub.go           # WebSocket Hub
├── services/
│   ├── agent_service.go
│   ├── session_service.go
│   ├── memory_service.go
│   └── config_service.go
└── wails.json
```

### 3.2 Frontend 模块结构

```
desktop/frontend/src/
├── connection/          # wails-bridge | ws-client | rest-client
├── stores/              # chat | session | memory | agent | ui (Zustand)
├── components/
│   ├── markdown/        # MarkdownRenderer | StreamingMarkdown
│   ├── message/         # UserMessage | AssistantMessage | ToolCard | ApprovalDialog | SystemNotice | TurnReceipt
│   └── ui/              # Sidebar | TitleBar | StatusBar
├── pages/
│   ├── chat/            # ChatPage | ChatInput | TranscriptView
│   ├── sessions/        # SessionList | SessionDetail
│   ├── memory/          # MemoryBrowser | MemoryGraph | MemoryEditor
│   ├── agents/          # AgentList | AgentEditor | SystemPromptEditor
│   └── settings/        # SettingsPage | ProviderConfig | Keybindings
└── hooks/               # useAgent | useChat | useWebSocket | useApproval
```

---

## 4. Server 补齐路线图

| Phase | 任务 | 周期 |
|-------|------|------|
| **P0 修复断链** | `handleStreamRun` 注入 callbacks；实现 `OnToken`/`OnReasoning` SSE | 1 周 |
| **P0 补齐 API** | Session 完整 API、审批 API、Transcript 持久化、Tools 列表 | 2 周 |
| **P1 WebSocket** | WS Hub、事件推送、审批双向通道、输入注入 | 1-2 周 |
| **P2 高级** | Compression 事件、MemoryBridge 自动化、System Prompt 预览 | 2 周 |

---

## 5. TUI → GUI 功能映射

| TUI 功能 | GUI 对应 | 层 |
|---------|---------|-----|
| 流式 token 渲染 | StreamingMarkdown | 前端 |
| 工具调用卡片 | ToolCard | 前端 |
| 审批弹窗 | ApprovalDialog | 前端 + Binding |
| Reasoning 折叠 | AssistantMessage 折叠按钮 | 前端 |
| 粘贴折叠 / 输入历史 | ChatInput + store | 前端 |
| Turn 取消 | useChat.cancelTurn() | 前端 + Binding |
| Session 管理 | SessionList + SessionDetail | 前端 + Service |
| Memory 浏览 | MemoryBrowser + MemoryGraph | 前端 + Service |

**GUI 独有增强**：多会话 Tab 并行、记忆图谱可视化、拖拽文件上传、Mermaid/LaTeX 渲染、Agent 配置可视化编辑、Flow DAG 编排、Eval 仪表盘

---

## 6. 实施优先级

```
Phase 1 — 基础骨架（2 周）
├── [Go] 修复 streamCallbacks 断链 + OnToken/OnReasoning
├── [Go] desktop/ 模块 + Wails 初始化 + 基础 Binding
├── [FE] Vite+React 脚手架 + ChatPage + 流式 Markdown
└── [FE] 端到端验证：输入 → 流式响应

Phase 2 — 核心交互（2-3 周）
├── [Go] Session API + 审批 API + Transcript 持久化 + WS Hub
├── [FE] SessionList + ToolCard + ApprovalDialog + WS 客户端
└── [FE] 主题系统 + 标题栏

Phase 3 — 高级功能（3 周）
├── [Go] Tools 列表 + MemoryBridge 自动化 + WS 双向通道
├── [FE] MemoryBrowser/Graph + AgentEditor + Settings
└── [FE] 系统托盘 + 快捷键

Phase 4 — 差异化（持续）
├── Flow 可视化编排 | Eval Dashboard | 多 Agent 并行
├── 对话导出 | 远程模式支持
```

---

## 7. 风险与缓解

| 风险 | 缓解 |
|------|------|
| Ubuntu 24.04 webkit2gtk 兼容 | 已有 pc 文件复制方案，CI 固化 |
| 双通道（Binding+WS）复杂度 | 抽象 ConnectionLayer 接口统一上层 |
| 长对话流式渲染性能 | 虚拟滚动 + requestAnimationFrame 节流 |
| 内嵌/远程模式差异 | 共享 ApplicationService 层，前端统一接口 |
