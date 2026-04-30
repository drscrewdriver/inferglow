# Inferglow

Go 语言实现的 AI Agent 基础设施框架，对标 [Agently](https://github.com/AgentEra/Agently)（Python）的设计理念。

## 为什么

Go 生态缺乏一个对标 Agently 设计理念的框架：**契约优先、可单测编排、内置沙箱、明确的 Pause/Resume/Persist 能力**。Inferglow 提供一套可组合的基础设施模块，为上层 AI Agent 框架（inferglow）提供支撑。

## 架构概览

```
┌─────────────────────────────────────────────────────────┐
│              agently 主模块（上层业务逻辑）               │
│          Agent 类 · 提示词工程 · 工作流编排               │
└─────────────────────────────────────────────────────────┘
                              │
            ┌─────────────────┼─────────────────┐
            │                 │                 │
            ▼                 ▼                 ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│  session        │ │  action         │ │  sandbox        │
│  对话记忆管理    │ │  Action Runtime │ │  沙箱执行框架    │
└────────┬────────┘ └────────┬────────┘ └────────┬────────┘
         │                   │                   │
         │              ┌────┴────┐               │
         │              │ model   │◄──────────────┘
         │              │ LLM     │
         │              └────┬────┘
         │                   │
         │              ┌────┴────┐
         │              │ schema  │
         │              │ 契约校验 │
         │              └────┬────┘
         │                   │
         │              ┌────┴────┐
         └──────────────│  flow   │
                        │ 编排引擎 │
                        └─────────┘
```

## 模块列表

### Layer 1: model — LLM Provider 统一抽象层

提供统一的 LLM Provider 抽象，屏蔽不同模型供应商（OpenAI、Anthropic、Ollama 等）的 API 差异。

- **模块路径**: `github.com/inferglow/model`
- **依赖**: 无（仅 stdlib + yaml.v3）
- **核心类型**: `ModelRequest`, `ModelResponse`, `StreamChunk`, `ModelRequester`
- **Provider 实现**: OpenAICompatibleProvider, AnthropicCompatibleProvider, OllamaProvider

### Layer 2: schema — Contract-First Schema 引擎

通过 Go 泛型 + 反射实现编译期 + 运行时双重校验，约束 LLM 的输出格式。

- **模块路径**: `github.com/inferglow/schema`
- **依赖**: `github.com/inferglow/model`
- **核心类型**: `OutputSchema`, `FieldDef`, `DataType`, `ContractEngine`
- **核心功能**: 泛型推导、JSON Schema 转换、路径校验、JSON 提取

### Layer 3: flow — TriggerFlow 编排引擎

两层流引擎架构：线性 Flow 引擎（简单管道）和 TriggerFlow 事件驱动引擎（复杂业务编排）。

- **模块路径**: `github.com/inferglow/flow`
- **依赖**: `github.com/inferglow/schema`
- **核心类型**: `Flow`, `Step`, `Operator`, `SignalNet`, `LifecycleMachine`
- **算子类型**: 13 种（chunk、signal_gate、batch_fanout、for_each、match_case 等）

### 独立模块: action — Action Runtime

将 Go 函数注册为可发现、可校验、可执行的动作单元。

- **模块路径**: `github.com/inferglow/action`
- **依赖**: 无（完全独立）
- **核心类型**: `Action`, `ActionExecutor`, `ActionResult`, `ActionRegistry`
- **核心功能**: LocalFunctionExecutor（三种签名自动包装）、ActionSpec 安全规格

### 独立模块: session — 对话记忆管理

对话历史维护、上下文窗口自动裁剪、多模态内容支持、JSON/YAML 持久化。

- **模块路径**: `github.com/inferglow/session`
- **依赖**: 无（完全独立）
- **核心类型**: `Session`, `ChatMessage`, `ContentBlock`, `ResizeHandler`
- **核心功能**: 双消息列表、多策略 resize、持久化

### 独立模块: sandbox — 沙箱执行框架

隔离的代码执行环境，支持多种沙箱后端。

- **模块路径**: `github.com/inferglow/sandbox`
- **依赖**: 无（完全独立）
- **核心类型**: `Provider`, `Handle`, `ExecutionPolicy`
- **后端实现**: Docker、gVisor、本地、TrustedLocal、Seatbelt、WindowsAppContainer
- **CLI 示例**: `sandbox/cmd/sandbox/main.go`（独立可运行）

## 模块依赖关系

```
                    model (Layer 1)
                         │
                    schema (Layer 2)
                         │
                       flow (Layer 3)

                    action (独立)

                    session (独立)

                    sandbox (独立)
```

| 模块 | 依赖 | 被谁依赖 |
|------|------|---------|
| **model** | 无 | schema, session |
| **schema** | model | flow |
| **flow** | schema | 上层业务逻辑 |
| **action** | 无 | 上层业务逻辑 |
| **session** | 无 | 上层业务逻辑 |
| **sandbox** | 无 | 上层业务逻辑 |

## 设计原则

1. **契约优先**: Schema 定义先行，LLM 输出受契约约束
2. **可单测编排**: 每个 Flow Step 是纯 Go 函数，可独立单元测试
3. **模块化**: 各子模块完全独立（action、session、sandbox 无依赖），可单独复用
4. **可扩展**: Provider/Executor/ResizeHandler 均通过接口扩展
5. **Go 适配**: 适配 Go 语言特性（goroutine 替代 async、泛型 + 反射替代 Pydantic）

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

## 开发状态

当前所有模块均已实现 MVP，包含完整的单元测试和集成测试。上层的 agently 主模块（Agent 类）正在开发中。

## License

MIT
