# Examples

可独立运行的示例程序，演示各个 inferglow 模块的使用方法。每个示例均配有中英双语描述文档。

## 示例列表 / Example List

| 示例 / Example | 描述文档 / Docs | 模块 / Module | 说明 / Description |
|:--------------:|:---------------:|:------------:|:-----------------:|
| `example_quickstart.go` | [📄](example_quickstart.md) | 综合 | 最小完整 Agent / Minimal Agent |
| `example_action.go` | [📄](example_action.md) | action | Action 注册与执行 |
| `example_flow.go` | [📄](example_flow.md) | flow | Flow 步骤编排 |
| `example_schema.go` | [📄](example_schema.md) | schema | Schema 校验 |
| `example_session.go` | [📄](example_session.md) | session | 对话记忆管理 |
| `example_audit.go` | [📄](example_audit.md) | audit | 审计链 |
| `example_model.go` | [📄](example_model.md) | model | LLM Provider 抽象 |
| `example_orchestrator.go` | [📄](example_orchestrator.md) | orchestrator | Agent 端到端编排 |
| `example_workspace.go` | [📄](example_workspace.md) | workspace | 安全文件操作 |
| `example_pluggable.go` | [📄](example_pluggable.md) | 可插拔安全 | 安全特性注入 |
| `example_sandbox_enabled.go` | [📄](example_sandbox_enabled.md) | 沙箱 | 沙箱执行 (需 `with_sandbox`) |
| `example_server_comprehensive.go` | [📄](example_server_comprehensive.md) | 综合 (server) | 完整系统能力串联 |

## 编译模式

Inferglow v2 支持两种编译模式，部分示例需要指定 build tag：

```bash
# 默认模式（无沙箱）：所有 example_*.go 均以 //go:build ignore 标记，可单独 go run
go run example_pluggable.go

# 沙箱模式：example_sandbox_enabled.go 需要 -tags with_sandbox
go run -tags with_sandbox example_sandbox_enabled.go
```

## 运行方式

```bash
# 确保在 examples 目录下
cd examples

# 编译运行（默认模式）
go build -o NUL .\example_quickstart.go && echo "quickstart: OK"
go build -o NUL .\example_action.go && echo "action: OK"
go build -o NUL .\example_flow.go && echo "flow: OK"
go build -o NUL .\example_schema.go && echo "schema: OK"
go build -o NUL .\example_session.go && echo "session: OK"
go build -o NUL .\example_audit.go && echo "audit: OK"
go build -o NUL .\example_model.go && echo "model: OK"
go build -o NUL .\example_orchestrator.go && echo "orchestrator: OK"
go build -o NUL .\example_workspace.go && echo "workspace: OK"
go build -o NUL .\example_pluggable.go && echo "pluggable: OK"
go build -o NUL .\example_server_comprehensive.go && echo "server_comprehensive: OK"

# 沙箱模式（需要 -tags with_sandbox）
go build -o NUL -tags with_sandbox .\example_sandbox_enabled.go && echo "sandbox_enabled: OK"

# 直接运行（需要 Go 1.25+）
go run example_quickstart.go
go run example_action.go
go run example_flow.go
go run example_schema.go
go run example_session.go
go run example_audit.go
go run example_model.go
go run example_orchestrator.go
go run example_workspace.go
go run example_pluggable.go
go run example_server_comprehensive.go
go run -tags with_sandbox example_sandbox_enabled.go
```

## 学习路径

建议按以下顺序学习，从基础概念逐步深入到完整 Agent 编排：

```
入门 → 核心模块 → 高级特性 → 生产化
```

| 阶段 | 步骤 | 示例 | 学习内容 | 预计时间 |
|------|:----:|------|---------|:--------:|
| **入门** | 0 | `example_quickstart.go` | 第一个 Agent：Session + Action + MockLLM 端到端 | 3 min |
| **核心模块** | 1 | `example_action.go` | 将 Go 函数注册为 Action 并调用 | 5 min |
| | 2 | `example_flow.go` | 用 Flow 编排步骤管道 | 5 min |
| | 3 | `example_schema.go` | Schema 定义与校验 | 5 min |
| | 4 | `example_session.go` | 对话记忆管理与裁剪 | 5 min |
| | 5 | `example_audit.go` | 不可篡改审计链 | 5 min |
| | 6 | `example_model.go` | LLM Provider 抽象与重试 | 10 min |
| | 7 | `example_orchestrator.go` | 组装完整 Agent（含 LoopGuard + Audit） | 10 min |
| | 8 | `example_workspace.go` | 安全文件操作 | 5 min |
| **高级特性** | 9 | `example_pluggable.go` | 接口注入安全特性（PII/注入检测） | 10 min |
| | 10 | `example_sandbox_enabled.go` | 沙箱执行（需 `with_sandbox` build tag） | 10 min |
| **综合** | 11 | `example_server_comprehensive.go` | 完整系统能力串联（server/模型/审计/沙箱） | 15 min |

> **推荐路径**：先跑 `example_quickstart.go` 感受全貌，再按顺序学习 1→7→8→9→10。

> **sandbox** 模块的独立 CLI 示例请使用 `sandbox/cmd/sandbox/main.go`

## 可插拔架构说明

Inferglow v2 将沙箱与安全特性改造为可选依赖：

- **`example_pluggable.go`**（默认模式）：演示不引入 sandbox 依赖时如何使用 Agent，以及通过接口注入（`security/sessionhook`、`security/agenthook`）启用 PII 脱敏与注入检测。无需特殊 build tag。
- **`example_sandbox_enabled.go`**（沙箱模式）：演示 `with_sandbox` tag 下真实 `SandboxExecutor` 的使用，通过 `sandbox.Manager` 调用后端 Provider 执行隔离命令。必须用 `-tags with_sandbox` 编译。

两种模式互不冲突：沙箱通过 build tag 控制，安全特性通过接口注入控制，可独立或组合启用。

## 依赖

所有示例通过 go.mod 中的 replace 指令引用本地 inferglow 子模块：

```
replace github.com/inferglow/model => ./../model
replace github.com/inferglow/schema => ./../schema
replace github.com/inferglow/flow => ./../flow
replace github.com/inferglow/action => ./../action
replace github.com/inferglow/session => ./../session
replace github.com/inferglow/audit => ./../audit
replace github.com/inferglow/orchestrator => ./../orchestrator
replace github.com/inferglow/workspace => ./../workspace
replace github.com/inferglow/security => ./../security
replace github.com/inferglow/sandbox => ./../sandbox
```
