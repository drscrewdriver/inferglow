# Inferglow

Go 语言实现的 AI Agent 基础设施框架，对标 [Agently](https://github.com/AgentEra/Agently)（Python）的设计理念。

[![CI](https://github.com/drscrewdriver/inferglow/actions/workflows/ci.yml/badge.svg)](https://github.com/drscrewdriver/inferglow/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25-blue)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

> **一句话定位**：Go 生态里兼具**契约优先**（L1-L4 约束生成）与**高可靠工程**（SSE 流式、取消语义、并发安全、不可篡改审计）的 Agent 基础设施框架。

## 文档导航

| 文档 | 内容 |
|---|---|
| [构建与测试总览](docs/guides/build-and-test.md) | 全仓库（30 个 go.mod）make 目标、Windows 替代、已知注意事项 |
| [GUI 使用与打包](docs/guides/gui.md) | `/gui` 使用、启动双模式、单二进制打包、SSE 契约、设置面板 |
| [开发工作流](docs/guides/development-workflow.md) | TDD 规范（Red/Green/Refactor）、Go 代码约束 |
| [工具目录](docs/guides/tool-catalog.md) | 内置工具/动作清单 |
| [扩展指南](EXTENDING.md) | 如何扩展/接入新模块 |

## 为什么

Go 生态缺乏一个对标 Agently 设计理念的框架：**契约优先、可单测编排、内置沙箱、明确的 Pause/Resume/Persist 能力**。Inferglow 提供一套可组合的基础设施模块，为下游引用 AI Agent 框架（inferglow）提供支撑（已经初步合并）。
<img width="1107" height="626" alt="image" src="https://github.com/user-attachments/assets/b4b892d0-d20b-49e2-ac50-9c0785f7209f" />


## 架构概览

InferGlow 采用 **23 个独立 Go module** 的细粒度架构，按依赖深度自然形成四层结构。

```mermaid
graph TD
    subgraph foundation["基础层 — 12 个模块，零内部依赖"]
        MODEL["model<br/>~8000 LOC"]
        SCHEMA["schema<br/>~2800 LOC"]
        SESS["session<br/>~1800 LOC"]
        SANDBOX["sandbox<br/>~6300 LOC"]
        CTX["context<br/>~6300 LOC"]
        AUDIT["audit<br/>~1100 LOC"]
        APPROVAL["approval<br/>~700 LOC"]
        RAG["rag<br/>~1500 LOC"]
        RERANK["rerank<br/>~500 LOC"]
        OBS["observability<br/>~700 LOC"]
        WS["workspace<br/>~1200 LOC"]
        RESOURCE["resource<br/>~750 LOC"]
    end

    subgraph mid["中间层 — 5 个模块，依赖基础层"]
        COMP["components<br/>→ model"]
        FLOW["flow<br/>→ schema"]
        ACT["action<br/>→ approval, sandbox"]
        MCPSERVER["mcpserver<br/>→ action"]
        BUILTINS["builtins<br/>→ action"]
    end

    subgraph orch["编排层 — 3 个模块，聚合中间层+基础层"]
        ORCH["orchestrator<br/>→ action,audit,flow,model,observability,session"]
        SECURITY["security<br/>→ orchestrator,session (接口注入)"]
        EVAL["eval<br/>→ action,model,orchestrator,session"]
    end

    subgraph app["应用层 — 3 个模块，面向用户的入口"]
        SERVER["server<br/>~3100 LOC<br/>→ flow, orchestrator"]
        CLI["cli<br/>→ orchestrator,context,builtins"]
        EXAMPLES["examples<br/>→ 多模块"]
    end

    %% 中间层 → 基础层
    COMP --> MODEL
    FLOW --> SCHEMA
    ACT --> APPROVAL
    ACT -.->|"with_sandbox"| SANDBOX
    MCPSERVER --> ACT
    BUILTINS --> ACT

    %% 编排层 → 中间层+基础层
    ORCH --> ACT
    ORCH --> SESS
    ORCH --> MODEL
    ORCH --> AUDIT
    ORCH --> FLOW
    ORCH --> OBS
    SECURITY -.->|"接口注入"| SESS
    SECURITY -.->|"接口注入"| ORCH
    EVAL --> MODEL
    EVAL --> ORCH

    %% 应用层 → 编排层+中间层
    SERVER --> ORCH
    SERVER --> FLOW
    CLI --> ORCH
    CLI --> CTX
    CLI --> BUILTINS
```

## 快速开始

> 下面是一个完整的端到端示例，演示如何用 inferglow 组装一个带工具调用的 Agent。
> 无需真实 LLM API Key，使用 MockLLM 即可运行。

### 0. 前端入口总览

InferGlow 有三个独立的前端方向，定位和使用场景各不相同：

| 入口 | 路径 | 定位 | 运行方式 | 状态 |
|------|------|------|---------|------|
| **Web GUI** | `/web` | 浏览器中运行的 Agent 管理界面（参考 DeepSeek Harness） | Server 无头启动 → 浏览器访问 | ⏳ **新方向，待实现** |
| Desktop GUI | `/gui` | 桌面壳内嵌的 Agent 界面（openhanako 风格） | Wails 桌面窗口内嵌 React GUI | ✅ 已实现（25 任务完成 23） |
| Dashboard | `/dashboard` | 独立可观测性仪表盘（Span 统计） | 浏览器直接访问 | ✅ 已实现（开发调试用） |

---

#### 方向一：Web GUI（浏览器网页版）— 待实现

**目标**：Server 以无头模式启动（监听端口），用户在任意浏览器中打开即用。不依赖桌面环境，不限操作系统。

**参考**：[DeepSeek Harness](https://github.com/deepseek-ai/harness) Web UI

**参考布局特征**（基于 DeepSeek Harness 截图）：

```
┌──────────────────────────────────────────────────────────────┐
│  顶栏：Logo + 导航标签（对话/轨迹/上下文） + Session log     │
├──────────┬────────────────────────────┬──────────────────────┤
│ 侧栏     │  对话区（主区域）           │  右侧详情面板        │
│          │                            │  - 任务管理          │
│ 工作区   │  消息流（Markdown 渲染）    │  - 文件列表          │
│ 会话树   │  产物展示（package.json）   │  - task_plan.md      │
│ 分组折叠 │  工具调用卡片              │                      │
│ 搜索     │                            │                      │
│          ├────────────────────────────┤                      │
│          │  输入区：模型选择 + 冻结    │                      │
│          │  + 发送按钮                │                      │
│          ├────────────────────────────┤                      │
│          │  终端抽屉（PowerShell）     │                      │
├──────────┴────────────────────────────┴──────────────────────┤
│  状态栏：轮次 · 步数 · LLM 耗时 · 工具调用 · Token 统计     │
└──────────────────────────────────────────────────────────────┘
```

**核心设计原则**：

1. **Server 无头**：`inferglow-server` 只监听 HTTP 端口，不启动任何 GUI 进程。前端是独立的 SPA，通过 REST + SSE 与后端通信。
2. **浏览器原生**：不依赖 Electron/Wails/Tauri，任何现代浏览器（Chrome/Edge/Firefox）均可访问。
3. **局域网共享**：Server 监听 `0.0.0.0` 时，同网络内其他设备也可访问。
4. **样式参考 DeepSeek Harness**：深色主题、三栏布局、侧栏工作区/会话树、底部终端抽屉、状态栏统计。

**与 Desktop GUI 的区别**：

| 维度 | Web GUI（网页版） | Desktop GUI（桌面版） |
|------|-------------------|----------------------|
| 运行环境 | 浏览器（任意 OS） | Wails 桌面窗口 |
| 通信方式 | REST + SSE（HTTP） | Wails Go↔JS 绑定（直调） |
| 样式风格 | DeepSeek Harness 风格 | openhanako 风格 |
| 部署方式 | Server 远程部署，浏览器远程访问 | 本地安装桌面应用 |
| 代码位置 | `webui/`（源码）→ `server/webbrowser/`（embed） | `web/`（源码）→ `server/webui/`（embed） |

**目录结构**：

```
inferglow-github/
├── webui/                    # Web GUI 源码（浏览器网页版，DeepSeek Harness 风格）
│   ├── src/
│   │   ├── App.tsx           # 主应用（三栏布局）
│   │   ├── components/       # Sidebar / ChatArea / DetailsPanel / StatusBar
│   │   ├── api/              # REST transport + SSE 解析
│   │   └── styles/           # CSS tokens（深色主题）
│   ├── vite.config.ts        # base: '/web/', outDir: '../server/webbrowser'
│   └── package.json          # inferglow-webui
├── web/                      # Desktop GUI 源码（桌面壳内嵌，openhanako 风格）
│   ├── src/                  # 完整功能（25 任务完成 23）
│   ├── vite.config.ts        # base: '/gui/', outDir: '../server/webui'
│   └── package.json          # inferglow-web
├── server/
│   ├── webbrowser/           # Web GUI embed 产物（webui/build 输出）
│   ├── webui/                # Desktop GUI embed 产物（web/build 输出）
│   ├── handlers_webui.go     # /web/ 路由 handler
│   └── handlers_gui.go       # /gui/ 路由 handler
```

**待完善**：

- Server 端新增 `/v1/settings`、`/v1/themes` API（设置/主题持久化）
- 对齐 DeepSeek Harness 的布局和交互模式（SSE 流式聊天、工具调用卡片等）
- 会话管理、上下文可视化等完整功能

---

#### 方向二：Desktop GUI（桌面壳内嵌）— 已实现

当前 `web/` 目录下的 React 19 + Vite + Zustand 前端，设计初衷是嵌入 Wails 桌面壳（`desktop/`）。

```bash
# 启动方式：Server + 浏览器访问（当前也可用，但定位是桌面 GUI）
cd server
go run ./cmd/inferglow-server -demo-agent
# 浏览器访问 http://localhost:8080/gui/
```

**已实现能力**（25 任务完成 23）：

- 三栏布局（侧栏 | 对话区 | 详情面板）
- 会话管理（置顶/归档/重命名/分组/搜索/拖拽排序）
- 输入流量管理（三级规划队列 + 冻结/恢复 + 后台任务）
- 会话折叠与导航（自动折叠、Canvas Minimap、智能加载历史）
- 上下文可视化（六色堆叠条、趋势图、上下文浏览器）
- @file 功能、沙箱与权限、设置面板（15 tab）、主题系统（20 套）

**待完成**：Task 24（设置面板服务端持久化）、Task 25（主题系统服务端持久化）

开发详见 [docs/guides/gui.md](docs/guides/gui.md)。

---

#### 方向三：Dashboard（可观测性仪表盘）— 已实现

独立的 Span 统计页面，纯 HTML，无认证，5 秒自动刷新。

```bash
# 随 Server 启动即可访问
http://localhost:8080/dashboard
```

### 1. 创建一个最简单的 Agent

```go
package main

import (
	"context"
	"fmt"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/session"
)

// 自定义 MockLLM — 无需真实 API Key 即可演示 Agent 编排逻辑
type mockLLM struct{}

func (m *mockLLM) Name() string { return "mock-llm" }

func (m *mockLLM) GenerateRequestData(ctx context.Context, req *model.ModelRequest) (*model.RequestData, error) {
	return &model.RequestData{Model: "mock", Messages: req.ChatHistory}, nil
}

func (m *mockLLM) RequestModel(ctx context.Context, data *model.RequestData) (<-chan *model.StreamChunk, error) {
	ch := make(chan *model.StreamChunk, 1)
	ch <- &model.StreamChunk{
		Delta:  `{"next_action":"response","final_response":"Hello from inferglow Agent!"}`,
		IsDone: true,
	}
	close(ch)
	return ch, nil
}

func (m *mockLLM) BroadcastResponse(ctx context.Context, stream <-chan *model.StreamChunk) (<-chan *model.ResultEvent, error) {
	return nil, nil
}

func main() {
	ctx := context.Background()

	// 1. 创建 Session（对话记忆）
	sess := session.NewSession("demo", 4000)

	// 2. 创建 ActionExtension（管理可被 LLM 调用的工具）
	actExt := agent.NewActionExtension()

	// 3. 注册一个 Action（将 Go 函数包装为工具）
	greetAction, _ := action.New("greet", "Greet a user",
		func(ctx context.Context, req map[string]any) (string, error) {
			name, _ := req["name"].(string)
			if name == "" {
				name = "friend"
			}
			return fmt.Sprintf("Hello, %s!", name), nil
		})
	actExt.Register(greetAction)

	// 4. 创建 Agent 并运行
	ag := agent.New(sess, actExt, &mockLLM{},
		agent.WithMaxRounds(5),
		agent.WithSystemPrompt("You are a helpful assistant."),
	)

	result, err := ag.Run(ctx, "Hello!", nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("Agent response:", result)
}
```

### 2. 运行方式

```bash
# 确保在 examples 目录下
cd examples

# 默认模式（无沙箱，体积更小）— 推荐从这里开始
go run example_quickstart.go

# 或逐个验证各模块
go run example_action.go      # Action 注册与执行
go run example_flow.go        # Flow 步骤编排
go run example_schema.go      # Schema 校验
go run example_session.go     # 对话记忆管理
go run example_audit.go       # 审计链
go run example_model.go       # LLM Provider 抽象
go run example_orchestrator.go # Agent 端到端编排

# 沙箱模式（需要 with_sandbox build tag）
go run -tags with_sandbox example_sandbox_enabled.go
```

### 3. 学习路径

| 步骤 | 示例 | 学习内容 | 预计时间 |
|------|------|---------|---------|
| 1 | `example_action.go` | 将 Go 函数注册为 Action 并调用 | 5 min |
| 2 | `example_flow.go` | 用 Flow 编排步骤管道 | 5 min |
| 3 | `example_schema.go` | Schema 定义与校验 | 5 min |
| 4 | `example_session.go` | 对话记忆管理与裁剪 | 5 min |
| 5 | `example_audit.go` | 不可篡改审计链 | 5 min |
| 6 | `example_model.go` | LLM Provider 抽象与重试 | 10 min |
| 7 | `example_orchestrator.go` | 组装完整 Agent | 10 min |
| 8 | `example_workspace.go` | 安全文件操作 | 5 min |
| 9 | `example_pluggable.go` | 接口注入安全特性 | 10 min |
| 10 | `example_sandbox_enabled.go` | 沙箱执行（需 build tag） | 10 min |
| 11 | `example_toolgroup.go` | 按组注册/列举/过滤工具（ToolGroup） | 5 min |
| 12 | `example_context.go` | 上下文管理（Mode/Ingest/渲染/transient） | 5 min |

> **推荐路径**：先跑 `example_quickstart.go` 感受全貌，再按顺序学习 1→7→8→9→10；需要工具组织或上下文管理时，可同时查阅 [`docs/guides/`](./docs/guides/README.md) 能力使用指南（工具组织与调度、上下文管理）。

### 4. 编译配置

#### 默认模式（无沙箱，推荐）
```bash
go build ./...
```

#### 沙箱模式（打包完整沙箱后端）
```bash
go build -tags with_sandbox ./...
```

#### 启用安全特性（接口注入，无需特殊编译）
```go
import (
    "github.com/inferglow/security/sessionhook"
    "github.com/inferglow/security/agenthook"
    "github.com/inferglow/security/pii"
    promptinjection "github.com/inferglow/security/prompt_injection"
)

secHook := sessionhook.NewSecurityHook(promptinjection.NewDefaultConfig())
sess := session.NewSessionWithOptions("id", 4000, session.WithSecurityHook(secHook))

outHook := agenthook.NewOutputInjectionHook(promptinjection.NewDefaultConfig())
piiMasker := agenthook.NewPIIMasker(pii.NewMasker(pii.MaskConfig{}))

ag := agent.New(sess, actExt, llm,
    agent.WithOutputSecurityHook(outHook),
    agent.WithPIIMasker(piiMasker),
)
```

> 完整可运行示例见 [`examples/example_pluggable.go`](./examples/example_pluggable.go) 与 [`examples/example_sandbox_enabled.go`](./examples/example_sandbox_enabled.go)。

## 代码库导航

本项目的知识图谱由 [graphify](https://github.com/Graphify-Labs/graphify) 自动生成，可快速查询架构关系。

```bash
# 查看全局架构
graphify query "What is the overall architecture of inferglow?"

# 查看 Agent 循环
graphify query "How does the Agent orchestration loop work?"

# 查看模块依赖
graphify query "What are the dependencies between modules?"

# 查看统计信息
graphify stats
```

生成的图谱数据位于 [`graphify-out/graph.json`](./graphify-out/graph.json)（8017 节点，17577 边）。

### 核心入口速查

| 你的目标 | 起始文件 | 说明 |
|---------|---------|------|
| 快速体验完整 Agent | [`examples/example_quickstart.go`](./examples/example_quickstart.go) | 无需 API Key，3 分钟跑通 |
| 注册自定义工具 | [`action/`](./action/) 模块 | `action.New()` 自动包装函数 |
| 编排多步骤流程 | [`flow/`](./flow/) 模块 | `flow.NewFlow().AddStep().Build()` |
| 启动 REST API 服务 | [`server/cmd/inferglow-server/main.go`](./server/cmd/inferglow-server/main.go) | HTTP 托管 Agent |
| 使用 CLI 终端 | [`cli/cmd/inferglow-cli/main.go`](./cli/cmd/inferglow-cli/main.go) | 交互式 REPL |
| 调试 Agent 循环 | [`orchestrator/agent/engine.go`](./orchestrator/agent/engine.go) → `executeLoop()` | 核心循环 18 步 |
| 添加新 LLM Provider | [`model/`](./model/) 模块 | 实现 `ModelRequester` 接口 |

## 模块列表

23 个独立 Go module，按依赖深度分为四层。总代码量约 62,000 行（不含测试）。

### 基础层 — 12 个模块，零内部依赖

#### model — LLM Provider 统一抽象层 (~8000 LOC)

提供统一的 LLM Provider 抽象，屏蔽不同模型供应商（OpenAI、Anthropic、Ollama 等）的 API 差异。

- **模块路径**: `github.com/inferglow/model`
- **依赖**: 无（仅 stdlib + yaml.v3）
- **核心类型**: `ModelRequest`, `ModelResponse`, `StreamChunk`, `ModelRequester`, `StreamRequester`
- **Provider 实现**: OpenAICompatible, AnthropicCompatible, Ollama, OpenAIResponses（`/responses` 端点）
- **Schema 校验**: `BuildJSONSchemaFromOutput`（L1/L2）、`OutputValidator`（L4 后置校验 + 重试）
- **URL 解析**: `ResolveURL` — `full_url` 完全覆盖 `base_url + default_path`
- **非标字段映射**: `ContentMapping` + `ExtractByPath` — 从非标准 SSE 路径提取 delta/reasoning
- **流式归一化**: `LeadingThinkNormalizer` — 三态状态机分离 `<think>` 推理内容
- **缓存预算**: `UsageInfo.PromptTokensDetails["cached_tokens"]` — Prefix Cache 命中信息回传

#### schema — Contract-First Schema 引擎 (~2800 LOC)

通过 Go 泛型 + 反射实现编译期 + 运行时双重校验，约束 LLM 输出格式。

- **模块路径**: `github.com/inferglow/schema`
- **依赖**: 无（仅 yaml.v3）
- **核心类型**: `OutputSchema`, `FieldDef`, `DataType`, `ContractEngine`
- **核心功能**: 泛型推导、JSON Schema 转换、路径校验、JSON 提取

#### session — 对话记忆管理 (~1800 LOC)

对话历史维护、上下文窗口自动裁剪、多模态内容支持、JSON/YAML 持久化。

- **模块路径**: `github.com/inferglow/session`
- **依赖**: 无（安全特性通过 `MessageHook` 接口注入）
- **核心类型**: `Session`, `ChatMessage`, `ContentBlock`, `ResizeHandler`, `MessageHook`

#### sandbox — 沙箱执行框架 (~6300 LOC)

隔离的代码执行环境，支持多种沙箱后端。

- **模块路径**: `github.com/inferglow/sandbox`
- **依赖**: 无（完全独立）
- **核心类型**: `Provider`, `Handle`, `ExecutionPolicy`
- **后端实现**: Docker、gVisor、本地、TrustedLocal、Seatbelt、E2B、Windows RestrictedToken、Windows AppContainer、Windows Sandbox (WSB)

#### context — 上下文管理引擎 (~6300 LOC)

从 `session/contextmgr` 拆出的独立模块。三区压缩（hot/warm/cold）、Prefix Cache 预算、甜点区自适应、宪法区（Zone 0.5）、任务相关性重组。

- **模块路径**: `github.com/inferglow/context`
- **依赖**: 无（完全独立，通过接口与 session 交互）
- **核心类型**: `HybridManager`, `CacheBudgetUpdater`, `ConstitutionalZone`
- **核心功能**: sweet-spot 自适应阈值、缓存预算钩子（`CacheBudgetUpdater` 最小接口避免循环依赖）、三问重组、衰减预热

#### audit — 链表式审计链 (~1100 LOC)

基于 SHA-256 哈希指针的不可篡改审计日志，支持 HMAC 签名验证。

- **模块路径**: `github.com/inferglow/audit`
- **依赖**: 无
- **核心类型**: `AuditChain`, `AuditEntry`, `AuditHook`

#### approval — HITL 审批 (~700 LOC)

Human-in-the-Loop 审批流程管理。

- **模块路径**: `github.com/inferglow/approval`
- **依赖**: 无
- **核心类型**: `Manager`, `ApprovalRequest`, `AccessPolicy`

#### rag — RAG 管道 (~1500 LOC)

文档加载、文本分割、Embedding 注册、检索管道。

- **模块路径**: `github.com/inferglow/rag`
- **依赖**: 无
- **核心类型**: `Pipeline`, `Loader`（6 种格式）、`Splitter`（3 种策略）、`EmbeddingRegistry`

#### rerank — 重排序 (~500 LOC)

检索结果重排序，支持多种后端。

- **模块路径**: `github.com/inferglow/rerank`
- **依赖**: 无
- **核心类型**: `Reranker`, `Document`
- **后端**: Cohere、LLM-based、Fallback

#### observability — OpenTelemetry 集成 (~700 LOC)

语义化 Span 追踪，6 种 SpanKind 覆盖 Agent/LLM/Tool/Flow 全链路。

- **模块路径**: `github.com/inferglow/observability`
- **依赖**: 无

#### workspace — 工作区文件操作 (~1200 LOC)

沙箱化的文件/目录操作，路径穿越防护、文件大小/数量限制。

- **模块路径**: `github.com/inferglow/workspace`
- **依赖**: 无
- **核心功能**: SafePath 三重防护、ReadOnly 模式、LineageStore 血缘追踪

#### resource — 资源管理 (~750 LOC)

资源提供者抽象与管理。

- **模块路径**: `github.com/inferglow/resource`
- **依赖**: 无
- **核心类型**: `Provider`, `Manager`, `Handle`

### 中间层 — 5 个模块，依赖基础层

#### flow — TriggerFlow + LCEL 编排引擎 (~7400 LOC)

三层流引擎：线性 Flow（简单管道）、TriggerFlow 事件驱动引擎（复杂业务编排）、LCEL Chain（轻量线性管道）。

- **模块路径**: `github.com/inferglow/flow`
- **依赖**: `schema`
- **核心类型**: `Flow`, `Step`, `Operator`, `SignalNet`, `LifecycleMachine`, `Chain`
- **算子类型**: 13 种（chunk、signal_gate、batch_fanout、for_each、match_case 等）
- **LCEL Chain**: `LCEL().Pipe().Build()` 线性管道 + MapChain/BranchChain/ParallelChain 组合器
- **Step.Schema 校验**: 每步执行后 `validateStepOutput` 主动校验

#### action — Action Runtime (~2900 LOC)

将 Go 函数注册为可发现、可校验、可执行的动作单元。

- **模块路径**: `github.com/inferglow/action`
- **依赖**: `approval`, `sandbox`（`sandbox` 通过 `with_sandbox` build tag 可选）
- **核心类型**: `Action`, `ActionExecutor`, `ActionResult`, `ActionRegistry`
- **核心功能**: LocalFunctionExecutor（三种签名自动包装）、SandboxExecutor（需 `with_sandbox`）、ActionSpec 安全规格

#### components — Prompt/Tool 通用接口 (~400 LOC)

- **模块路径**: `github.com/inferglow/components`
- **依赖**: `model`

#### mcpserver — MCP 协议服务 (~850 LOC)

MCP JSON-RPC 2.0 服务端，三种传输全覆盖。

- **模块路径**: `github.com/inferglow/mcpserver`
- **依赖**: `action`
- **核心类型**: `Server`, `Transport`, `JSONRPCRequest`
- **传输**: stdio（标准输入输出）、SSE（GET `/sse` + POST `/messages`）、StreamableHTTP（POST `/mcp`）

#### builtins — 内置 Action/Policy/Tool (~2200 LOC)

预置的常用 Action、安全策略和 Tool 定义。

- **模块路径**: `github.com/inferglow/builtins`
- **依赖**: `action`

### 编排层 — 3 个模块，聚合中间层+基础层

#### orchestrator — Agent 编排层 / 用户入口 (~7700 LOC)

PLAN-EXECUTE 循环引擎。安全特性通过接口注入 / build tag 可选启用。

- **模块路径**: `github.com/inferglow/orchestrator`
- **依赖**: `action`, `audit`, `flow`, `model`, `observability`, `session`（`security` 接口注入；`sandbox` 通过 `with_sandbox` 可选）
- **核心类型**: `Agent`, `Engine`, `ActionDispatcher`, `LoopGuard`, `TurnLoop`, `CancelManager`
- **核心功能**: 并发 Action 执行、function calling、LLM 输出修复管道、死循环检测、三种取消安全点、Agent 回放测试

#### security — 安全基础设施 (~2000 LOC)

PII 脱敏、Prompt 注入检测、令牌桶限流、RBAC 访问控制。通过接口注入模式接入，不注入即零开销。

- **模块路径**: `github.com/inferglow/security`
- **依赖**: `session`、`orchestrator`（仅 `sessionhook`/`agenthook` 子包实现接口契约）
- **子模块**: `pii`（5 种模式）、`prompt_injection`（三级严重度）、`ratelimit`（令牌桶）、`rbac`、`sessionhook`（`MessageHook`）、`agenthook`（`OutputSecurityHook` / `PIIMasker`）

#### eval — 离线评估框架 (~750 LOC)

基于预录响应的 Agent 回放测试框架，支持并行执行与断言校验。

- **模块路径**: `github.com/inferglow/eval`
- **依赖**: `model`, `session`, `action`, `orchestrator`
- **核心类型**: `Suite`, `Case`, `Runner`, `ScriptedProvider`, `Report`
- **核心功能**: ScriptedProvider mock（实现 `model.ModelRequester`）、并行 Case 执行、Contains/NotContains/ToolSequence 断言、Golden Session 适配、Text/JSON 报告

#### examples — 示例代码 (~2800 LOC)

完整可运行的示例程序，覆盖主要使用场景。

- **模块路径**: `github.com/inferglow/examples`
- **依赖**: 多模块

### 应用层 — 3 个模块，面向用户的入口

#### server — REST API 服务 (~3100 LOC)

Agent HTTP 托管服务，提供 RESTful 接口、外部触发器、持久化 Memory、流式工具调用和运行时状态检查。

- **模块路径**: `github.com/inferglow/server`
- **依赖**: `orchestrator`（Agent 执行）, `flow`（声明式 Flow 数据模型 + trigger 子包）
- **核心类型**: `Server`, `Router`, `RunManager`, `MemoryStore`, `trigger.Registry`
- **子包**: `trigger/`（Webhook/Cron/EventBus 三种触发器）
- **架构说明**: server 对 flow 的依赖是**数据模型层**（REST API 需要序列化 `FlowDef`、注册 `stage.Registry`），Agent 执行路径通过 orchestrator 完成
- **V7 新增能力**:
  - 外部触发器：Webhook（HMAC 验签）、Cron（定时）、EventBus（事件驱动）
  - LCEL 声明式链（`flow/lcel.go`）：Chain/Pipe/Invoke/Build + 3 组合器
  - 持久化 Memory：MemoryStore 接口 + InMemoryStore + CRUD API
  - 运行时状态检查：只读 state/steps 查询 API
  - 流式工具调用：AgentCallbacks → SSE 事件流桥接
  - Session 结束自动提升：SessionEndHook 异步调用 LongMemPromoter

#### cli — 终端 REPL 客户端 (~1200 LOC)

交互式终端 Agent 客户端，内置持久记忆注入、上下文压缩、会话管理。GUI 项目可选择将 cli 作为子进程调用，或完全独立实现。

- **模块路径**: `github.com/inferglow/cli`
- **依赖**: `orchestrator`, `action`, `builtins`, `context`, `model`, `session`
- **核心类型**: `RunREPL`, `MemoryBridge`, `CLIConfig`, `CommandFunc`
- **核心功能**: 交互式 REPL、持久记忆注入（ProactiveRecall）、上下文压缩（`/compact`）、宪法区加载、会话恢复

## 模块依赖关系

```mermaid
graph TD
    subgraph foundation["基础层 — 零内部依赖"]
        MODEL["model"]
        SCHEMA["schema"]
        SESS["session"]
        SANDBOX["sandbox"]
        CTX["context"]
        AUDIT["audit"]
        APPROVAL["approval"]
        RAG["rag"]
        RERANK["rerank"]
        OBS["observability"]
        WS["workspace"]
        RESOURCE["resource"]
    end

    subgraph mid["中间层"]
        COMP["components"]
        FLOW["flow"]
        ACT["action"]
        MCPSERVER["mcpserver"]
        BUILTINS["builtins"]

        COMP --> MODEL
        FLOW --> SCHEMA
        ACT --> APPROVAL
        ACT -.->|"with_sandbox"| SANDBOX
        MCPSERVER --> ACT
        BUILTINS --> ACT
    end

    subgraph orch["编排层"]
        ORCH["orchestrator"]
        SECURITY["security"]
        EVAL["eval"]

        ORCH --> ACT
        ORCH --> SESS
        ORCH --> MODEL
        ORCH --> AUDIT
        ORCH --> FLOW
        ORCH --> OBS
        SECURITY -.->|"接口注入"| SESS
        SECURITY -.->|"接口注入"| ORCH
        EVAL --> MODEL
        EVAL --> ORCH
    end

    subgraph app["应用层"]
        SERVER["server"]
        CLI["cli"]
        EXAMPLES["examples"]

        SERVER --> ORCH
        SERVER --> FLOW
        CLI --> ORCH
        CLI --> CTX
        CLI --> BUILTINS
    end
```

| 模块 | 内部依赖 | 被谁依赖 |
|------|---------|----------|
| model | 无 | orchestrator, eval, components |
| schema | 无 | flow |
| flow | schema | orchestrator, server |
| action | approval, sandbox | orchestrator, mcpserver, builtins, eval |
| session | 无 | orchestrator, security, eval |
| audit | 无 | orchestrator |
| sandbox | 无 | action (build tag) |
| context | 无 | — (用户代码直接集成) |
| orchestrator | action, audit, flow, model, observability, session | security, eval, server |
| security | session, orchestrator (接口注入) | 用户代码 |
| eval | model, session, action, orchestrator | 用户代码 |
| server | flow（数据模型）, orchestrator（Agent 执行）, trigger（子包） | 用户代码 |
| cli | orchestrator, action, builtins, context, model, session | 用户代码 / GUI 子进程 |
| mcpserver | action | 用户代码 |
| builtins | action | 用户代码 |
| approval | 无 | action |
| rag | 无 | 用户代码 |
| rerank | 无 | 用户代码 |
| observability | 无 | orchestrator |
| workspace | 无 | 用户代码 |
| resource | 无 | 用户代码 |
| components | model | 用户代码 |

## 设计原则

1. **契约优先**: Schema 定义先行，LLM 输出受 L1-L4 四层校验约束（见下文"Schema 四层校验架构"）
2. **可单测编排**: 每个 Flow Step 是纯 Go 函数，可独立单元测试
3. **模块化**: 各子模块完全独立（action、session、sandbox 无依赖），可单独复用
4. **可扩展**: Provider/Executor/ResizeHandler 均通过接口扩展
5. **Go 适配**: 适配 Go 语言特性（goroutine 替代 async、泛型 + 反射替代 Pydantic）

## Schema 四层校验架构

Inferglow 实现 L1-L4 四层 schema 保障，确保 LLM 输出结构合规：

| 层级 | 机制 | 触发条件 | 合规率 |
|------|------|---------|--------|
| **L1 硬约束** | `response_format: json_schema`（XGrammar in sglang/vllm, GBNF in llama.cpp 等 token 级约束引擎） | `force_json:true` + `Output.Properties` 非空 | ~100% |
| **L2 API 约束** | 同 L1，云端 provider 服务端 structured output | 同上（OpenAI/DeepSeek） | ~99% |
| **L3 兜底 prompt** | system prompt 注入 schema 描述 | provider 不支持 json_schema 时降级 | ~80% |
| **L4 后置校验** | JSON 结构校验 + 字段类型检查 + 重试 | `WithOutputSchema` 配置后始终启用 | 检测层 |

- L1/L2 是**同一个 HTTP 参数**（`response_format`），区别在服务端实现（vLLM XGrammar vs 云端软约束）
- L3 是**降级方案**，仅当 L1/L2 不可用时注入 prompt
- L4 是**检测层**，即使 L1 生效也应跑 L4（防御性编程）
- Flow 的 `Step.Schema` 在每步执行后独立校验（`validateStepOutput`），与 L4 互补

## 与 Python Agently 的对照

| Python Agently | Go Inferglow | 职责 |
|---------------|-------------|------|
| `agently/core/model/` | `github.com/inferglow/model` | LLM Provider 抽象 |
| `agently/types/data/` + `types/plugins/` | `github.com/inferglow/schema` | Schema 定义 + 契约验证 |
| `agently/builtins/blocks/` + `trigger_flow/` | `github.com/inferglow/flow` | 编排引擎 |
| `agently/core/operation/Action/` | `github.com/inferglow/action` | Action 注册与执行 |
| `agently/core/session/` | `github.com/inferglow/session` | 对话记忆 |
| `agently/types/data/policy_approval.go` | `github.com/inferglow/sandbox` | 沙箱执行 |

## Go 语言适配

| Python 特性 | Go 适配方案 |
|------------|------------|
| ContextVar | context.Context + 值传递 |
| Pydantic TypeAdapter | Go 泛型 + 反射 + JSON Schema |
| 装饰器 (@agent.tool_func) | Go func + 显式调用 |
| async/await | goroutine + channel |
| TypedDict | Go struct |
| Protocol (typing) | Go interface |
| asyncio.Event/Lock | Go channel + sync.Mutex |

## 工程鲁棒性（Engineering Robustness）

面向流式、并发与高可靠性场景的工程化保障（这是 Agent 框架能否扛住生产环境的关键）：

| 关注点 | 实现 |
|--------|------|
| **SSE 流式解析** | `model/internal/ssestream` 统一封装四个 Provider 的 SSE 解析：每次读取前轮询 `ctx.Done()`、有界缓冲 channel（cap 64）、EOF/错误归一、`body` 随协程退出关闭，**无 goroutine 泄漏** |
| **取消语义** | `CancelManager` 提供三种取消安全点，区分"停止给客户端"与"后台必须读完"两种语义；所有协程通过 `context` 联动取消 |
| **死循环防护** | `LoopGuard` 检测无限 Agent 循环，避免失控轮转 |
| **并发 Action** | 并发执行 + 输入队列 + `EventSink` 统一事件流，避免共享可变状态竞态 |
| **不可篡改审计** | 链表式 SHA-256 哈希指针 + HMAC 签名，审计链防篡改、可验证 |
| **确定性测试** | `eval` 离线回放（ScriptedProvider mock），并行走 Case + 断言（Contains/ToolSequence），无需真实 API Key 即可复现并发与编排行为 |

> 设计取向：**用"有界缓冲 + 显式取消 + 单一事件流"替代"裸 goroutine + 无界 channel"**，从结构上规避资源泄漏与竞态，而不是靠事后排查。

## 系统分析文档

详细的系统分析文档位于 [`docs/system-analysis/`](./docs/system-analysis/) 目录：

| 文档 | 内容 |
|------|------|
| [01-architecture-overview.md](./docs/system-analysis/01-architecture-overview.md) | 分层架构、模块依赖、全景调用关系图 |
| [02-model-and-schema.md](./docs/system-analysis/02-model-and-schema.md) | LLM Provider 抽象、Schema 引擎、ContractEngine |
| [03-flow.md](./docs/system-analysis/03-flow.md) | Flow/TriggerFlow 双引擎、13 种算子、Pause/Resume |
| [04-action-and-mcp.md](./docs/system-analysis/04-action-and-mcp.md) | Action Runtime、三种 Executor、MCP JSON-RPC 协议 |
| [05-session-sandbox-audit.md](./docs/system-analysis/05-session-sandbox-audit.md) | 双列表 Session、8 种沙箱后端、SHA-256 哈希链 |
| [06-security-observability-workspace.md](./docs/system-analysis/06-security-observability-workspace.md) | PII/注入/限流/RBAC、OTel 6 种 Span、工作区血缘 |
| [07-orchestrator.md](./docs/system-analysis/07-orchestrator.md) | Agent 入口、executeLoop 18 步逐行解析、LoopGuard |
| [08-call-chains.md](./docs/system-analysis/08-call-chains.md) | 13 条端到端调用链 + 全景关系图 + 错误传播表 |

## 开发状态

所有模块均已实现并经过完整测试：

- **23 个 Go module**：全部通过 `go build` 和 `go vet`，Windows 交叉编译 clean
- **orchestrator 编排层**：Agent Loop + function calling + 并发 Action + 死循环检测 + 三种取消安全点
- **Schema 四层校验**：L1 json_schema / L2 API 约束 / L3 prompt 兜底 / L4 后置校验+重试 + Flow step.Schema
- **MCP 协议**：JSON-RPC 2.0 三种传输全覆盖（stdio / SSE / StreamableHTTP）
- **沙箱**：8 种后端（Docker、gVisor、本地、TrustedLocal、Seatbelt、E2B、Windows RestrictedToken/AppContainer/Sandbox）
- **上下文管理**：独立 `context/` 模块，三区压缩 + Prefix Cache 预算 + 甜点区自适应 + 宪法区
- **评估框架**：`eval/` 离线回放测试，ScriptedProvider mock + 并行执行 + 断言校验
- **外部触发器**：Webhook（HMAC 验签）/ Cron（定时）/ EventBus（事件驱动），trigger.Registry 生命周期管理
- **LCEL 声明式链**：Chain/Pipe/Invoke/Build + MapChain/BranchChain/ParallelChain 组合器
- **持久化 Memory**：MemoryStore 接口 + InMemoryStore + CRUD API + SessionEndHook 自动提升
- **运行时状态检查**：只读 state/steps 查询 API，ExecState 快照
- **流式工具调用**：AgentCallbacks → SSE 事件流桥接，step_done 逐步事件
- **CLI 终端客户端**：`cli/` 独立模块，交互式 REPL + 持久记忆注入 + 上下文压缩 + 会话恢复，GUI 可作子进程接入

### V6 架构演进完成事项

| 梯队 | 任务 | 状态 | 说明 |
|------|------|:----:|------|
| **第一梯队** 结构修复 | S1 otel 解耦 | ✅ | `ports.go` SpanStarter 接口，删除 7 处直接 import |
| | S2 contextmgr 独立 module | ✅ | `context/` 从 `session/` 拆出，独立 go.mod |
| | S3 middleware.Handler 统一签名 | ✅ | `orchestrator/middleware/` 统一 `Handler` 类型 |
| **第二梯队** fork 差异化 | S4 team/ 包 | ✅ | Multi-Agent 协调器 |
| | S5 扩展机制文档化 | ✅ | `docs/EXTENDING.md` 7 种机制 |
| | F1 HITL 审批集成 | ✅ | `approval/` 独立模块 |
| | F3 Agent 回放测试 | ✅ | `agent/replay.go` |
| | F4 提示词版本标记 | ✅ | `SessionData.PromptVersion` |
| | F5 成本模型 | ✅ | `CacheBudgetUpdater` 接口 |
| **第三梯队** 远期 | L1 Prefix Cache 预算 | ✅ | `context/hybrid.go` sweet-spot + 缓存预算钩 |
| | L2 Eval Runner | ✅ | `eval/` 独立模块，11 个测试 |
| | L3 MCP 三传输 | ✅ | stdio + SSE + StreamableHTTP |
| | L4 Windows 沙箱 | ✅ | RestrictedToken + AppContainer + WindowsSandbox (WSB) |

## License

MIT
