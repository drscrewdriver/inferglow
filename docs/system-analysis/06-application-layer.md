# 06 · 应用层详解

应用层是 Inferglow 架构的最上层，由 3 个模块组成，总代码量约 7,000 LOC。它们依赖编排层和下层模块，为用户提供直接交互的入口——REST API 服务、终端 REPL 客户端以及示例代码。

```
应用层（3 模块）
├── server      — REST API 服务    (~3100 LOC)
├── cli         — 终端 REPL 客户端 (~1200 LOC)
└── examples    — 示例代码         (~2800 LOC)
```

---

## 一、server — REST API 服务

**模块路径：** `github.com/inferglow/server`

**依赖：** `flow`（数据模型）、`orchestrator`（Agent 执行）

**代码量：** ~3100 LOC

### 核心类型

```go
type Server struct {
    cfg        Config
    mux        *http.ServeMux
    httpServer *http.Server
    agentStore AgentStore
    tenantMgr  *TenantManager
    // ...
}
```

`Server` 结构体直接使用标准库 `net/http` 的 `ServeMux` 进行路由分发，无需第三方 HTTP 框架。`agentStore` 管理 Agent 实例的生命周期，`tenantMgr` 提供多租户隔离。

### API 路由

| 路径 | 方法 | 说明 |
|------|------|------|
| `/v1/chat/completions` | POST | 聊天补全 |
| `/v1/flows` | POST | 创建 Flow |
| `/v1/flows/:id` | GET | 查询 Flow |
| `/v1/flows/:id/execute` | POST | 执行 Flow |
| `/v1/flows/:id/pause` | POST | 暂停 Flow |
| `/v1/flows/:id/resume` | POST | 恢复 Flow |
| `/v1/flows/:id/state` | GET | 运行时状态 |
| `/v1/flows/:id/steps` | GET | 步骤状态 |
| `/v1/memory` | GET/POST | 持久化 Memory CRUD |
| `/v1/memory/:id` | GET/DELETE | Memory 操作 |
| `/v1/triggers` | GET/POST | 触发器管理 |
| `/v1/triggers/:id` | DELETE | 删除触发器 |
| `/v1/health` | GET | 健康检查 |
| `/v1/openapi.json` | GET | OpenAPI 3.0 规范 |

### API Handler 模式

server 模块的 handler 遵循标准 Go HTTP handler 模式，通过依赖注入获取 orchestrator 和 flow 的实例：

```go
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
    var req ChatCompletionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request", http.StatusBadRequest)
        return
    }

    agent, err := s.agentStore.Get(req.AgentID)
    if err != nil {
        http.Error(w, "agent not found", http.StatusNotFound)
        return
    }

    result, err := agent.Run(r.Context(), req.Message)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(ChatCompletionResponse{
        Choices: []Choice{{Message: result}},
    })
}

func (s *Server) handleFlowExecute(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id")
    flow, err := s.agentStore.GetFlow(id)
    if err != nil {
        http.Error(w, "flow not found", http.StatusNotFound)
        return
    }

    var req FlowExecuteRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid request", http.StatusBadRequest)
        return
    }

    result, err := flow.Execute(r.Context(), req.Input)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(FlowExecuteResponse{Result: result})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(map[string]string{
        "status": "ok",
    })
}
```

### 外部触发器

server 模块支持三种外部触发器，用于自动化 Flow 执行：

| 触发器 | 说明 | 配置方式 |
|--------|------|---------|
| `WebhookTrigger` | Webhook 回调触发 | HMAC 验签确保请求来源可信 |
| `CronTrigger` | 定时触发 | 标准 Cron 表达式 |
| `EventTrigger` | 事件驱动 | 基于 EventBus 的消息分发 |

### 流式工具调用

server 模块通过 SSE（Server-Sent Events）支持流式输出，工具调用事件以标准格式推送：

```go
type ToolStreamEvent struct {
    Type    string          // "step_done" | "tool_call" | "tool_result"
    Data    json.RawMessage
    // ...
}
```

- `step_done`：Flow 步骤执行完成
- `tool_call`：Agent 发起工具调用
- `tool_result`：工具执行结果返回

### 多租户与隔离

`TenantManager` 负责多租户场景下的资源隔离，通过 tenant ID 区分不同用户的 Agent 实例、Flow 定义和 Memory 存储。`AgentStore` 维护每个租户的 Agent 实例池，支持按需创建和销毁。

---

## 二、cli — 终端 REPL 客户端

**模块路径：** `github.com/inferglow/cli`

**依赖：** `orchestrator`、`action`、`builtins`、`context`、`model`、`session`

**代码量：** ~1200 LOC

### 核心类型

```go
type CLIConfig struct {
    Model      string
    MaxRounds  int
    // ...
}

type MemoryBridge struct {
    hybridManager *context.HybridManager
    store         *Store
    // ...
}
```

`CLIConfig` 控制 REPL 会话的基本行为，包括使用的模型和最大迭代轮数。`MemoryBridge` 桥接 `context.HybridManager` 与持久化存储，实现跨会话的记忆保留。

### 功能列表

| 功能 | 说明 |
|------|------|
| 交互式 REPL | 多轮对话，支持连续上下文 |
| 持久记忆注入 | `MemoryBridge` 在会话启动时自动注入历史记忆 |
| 上下文压缩 | `/compact` 命令触发三区压缩 |
| 宪法区加载 | 不可变系统提示保护，确保 Agent 行为边界 |
| 会话恢复 | 从文件系统恢复之前的会话状态 |
| 内置命令 | `/help`、`/memory`、`/compact`、`/quit` |

### REPL 主循环示例

```go
func runREPL(cfg CLIConfig) error {
    reader := bufio.NewReader(os.Stdin)
    agent := buildAgent(cfg)

    for {
        fmt.Print("> ")
        input, _ := reader.ReadString('\n')
        input = strings.TrimSpace(input)

        switch {
        case input == "/quit":
            return nil
        case input == "/help":
            printHelp()
            continue
        case input == "/compact":
            agent.CompactContext()
            fmt.Println("Context compressed.")
            continue
        case input == "/memory":
            printMemory(agent)
            continue
        }

        result, err := agent.Run(context.Background(), input)
        if err != nil {
            fmt.Fprintf(os.Stderr, "Error: %v\n", err)
            continue
        }
        fmt.Println(result)
    }
}
```

### 持久记忆注入

`MemoryBridge` 在 REPL 启动时从持久化存储加载历史记忆，注入到 `context.HybridManager` 的温区（Warm Zone）中。每次对话结束后，新产生的记忆被异步写回存储。其工作流程如下：

1. **启动加载**：从 `Store` 读取序列化的 `ChatMessage` 列表
2. **注入温区**：将历史消息注入 `HybridManager.WarmZone`
3. **运行时同步**：每轮对话结束后，将增量更新写回 `Store`
4. **/compact 触发**：调用 `HybridManager` 的三区压缩，将冷区消息摘要归档

### 宪法区加载

宪法区（Zone 0.5）是 `context.HybridManager` 提供的不可变系统提示区域。CLI 在初始化时通过 `context.WithConstitutionalZone()` 注入系统级约束，这些约束不会被上下文压缩机制裁剪，确保 Agent 始终遵循预设的行为边界。

---

## 三、examples — 示例代码

**模块路径：** `github.com/inferglow/examples`

**依赖：** 多模块

**代码量：** ~2800 LOC

### 示例覆盖范围

examples 模块提供了多层面的使用示例，覆盖 Inferglow 框架的主要能力：

| 示例类别 | 说明 |
|---------|------|
| 基础 Agent | 创建 Agent、配置模型、发起对话 |
| 工具调用 | 注册自定义 Action、执行工具链 |
| Flow 编排 | 定义线性 Flow、使用 LCEL 链式调用 |
| 多轮对话 | Session 管理、上下文压缩 |
| 沙箱执行 | 配置沙箱策略、执行安全代码 |
| MCP 服务 | 启动 MCP Server、注册工具 |
| 多 Agent 协作 | Team 模式、消息总线通信 |

### 典型示例结构

```go
// 创建一个简单的 Agent 示例
func ExampleBasicAgent() {
    // 1. 创建 Session
    sess := session.NewSessionWithOptions("example", 4000)

    // 2. 创建 Action 扩展
    actExt := action.NewActionExtension()
    actExt.Register(action.NewAction("calculator", "执行算术计算", nil, calculatorFunc))

    // 3. 创建 Model Requester
    provider := model.NewOpenAICompatibleProvider("https://api.openai.com/v1", os.Getenv("OPENAI_API_KEY"))

    // 4. 创建 Agent
    ag := agent.New(sess, actExt, provider,
        agent.WithMaxRounds(5),
        agent.WithSystemPrompt("你是一个有帮助的助手。"),
    )

    // 5. 运行
    result, err := ag.Run(context.Background(), "Hello!")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(result)
}
```

### 示例在开发中的作用

- **集成测试**：examples 模块中的可运行示例同时也是集成测试用例，验证各模块间的协作正确性
- **文档补充**：示例代码是对架构文档的活文档补充，提供可直接运行的代码参考
- **快速原型**：新功能开发时，examples 中对应的示例可作为快速原型起点

---

## 应用层依赖关系总结

```
应用层依赖关系图：

server  →  orchestrator  →  action / audit / flow / model / observability / session
        →  flow

cli     →  orchestrator  →  action / audit / flow / model / observability / session
        →  action / builtins / context / model / session

examples → 多模块（涵盖基础层、中间层、编排层）
```

应用层位于依赖链的最顶端，不提供被其他模块引用的接口，是系统面向用户的最终入口。其中 server 和 cli 分别面向 API 调用者和终端用户，examples 则作为开发者的学习起点。