# Examples

可独立运行的示例程序，演示各个 inferglow 模块的使用方法。

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
go build -o NUL .\example_action.go && echo "action: OK"
go build -o NUL .\example_flow.go && echo "flow: OK"
go build -o NUL .\example_schema.go && echo "schema: OK"
go build -o NUL .\example_session.go && echo "session: OK"
go build -o NUL .\example_audit.go && echo "audit: OK"
go build -o NUL .\example_model.go && echo "model: OK"
go build -o NUL .\example_orchestrator.go && echo "orchestrator: OK"
go build -o NUL .\example_workspace.go && echo "workspace: OK"
go build -o NUL .\example_pluggable.go && echo "pluggable: OK"

# 沙箱模式（需要 -tags with_sandbox）
go build -o NUL -tags with_sandbox .\example_sandbox_enabled.go && echo "sandbox_enabled: OK"

# 直接运行（需要 Go 1.25+）
go run example_action.go
go run example_flow.go
go run example_schema.go
go run example_session.go
go run example_audit.go
go run example_model.go
go run example_orchestrator.go
go run example_workspace.go
go run example_pluggable.go
go run -tags with_sandbox example_sandbox_enabled.go
```

## 示例列表

| 示例 | 对应模块 | Build Tag | 说明 |
|------|---------|-----------|------|
| `example_action.go` | action | - | 注册函数为 Action、使用 LocalFunctionExecutor、执行 Action |
| `example_flow.go` | flow | - | 线性 Flow 编排、条件分支、带 Schema 校验的步骤 |
| `example_schema.go` | schema | - | 泛型推导 OutputSchema、JSON Schema 转换、路径表达式解析 |
| `example_session.go` | session | - | 消息管理、上下文窗口裁剪、多策略 resize、持久化 |
| `example_audit.go` | audit | - | AuditChain 追加/签名/验证/查询/导出（JSON/CSV/Text） |
| `example_model.go` | model | - | 3 Provider 构造、AttemptRunner 重试分类、OutputValidator 校验 |
| `example_orchestrator.go` | orchestrator | - | Agent + Engine + LoopGuard + AuditChain 组装与 Run |
| `example_workspace.go` | workspace | - | 安全 IO（路径穿越拦截）+ 文件血缘管理 |
| `example_pluggable.go` | 可插拔架构 | - | 默认模式：零开销路径 + 接口注入安全特性（sessionhook/agenthook） |
| `example_sandbox_enabled.go` | 可插拔架构 | `with_sandbox` | 沙箱模式：SandboxExecutor + 完整功能 Agent |

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
