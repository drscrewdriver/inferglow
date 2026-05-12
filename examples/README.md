# Examples

可独立运行的示例程序，演示各个 inferglow 模块的使用方法。

## 运行方式

```bash
# 确保在 examples 目录下
cd examples

# 编译运行
go build -o NUL .\example_action.go && echo "action: OK"
go build -o NUL .\example_flow.go && echo "flow: OK"
go build -o NUL .\example_schema.go && echo "schema: OK"
go build -o NUL .\example_session.go && echo "session: OK"
go build -o NUL .\example_audit.go && echo "audit: OK"
go build -o NUL .\example_model.go && echo "model: OK"
go build -o NUL .\example_orchestrator.go && echo "orchestrator: OK"
go build -o NUL .\example_workspace.go && echo "workspace: OK"

# 直接运行（需要 Go 1.25+）
go run example_action.go
go run example_flow.go
go run example_schema.go
go run example_session.go
go run example_audit.go
go run example_model.go
go run example_orchestrator.go
go run example_workspace.go
```

## 示例列表

| 示例 | 对应模块 | 说明 |
|------|---------|------|
| `example_action.go` | action | 注册函数为 Action、使用 LocalFunctionExecutor、执行 Action |
| `example_flow.go` | flow | 线性 Flow 编排、条件分支、带 Schema 校验的步骤 |
| `example_schema.go` | schema | 泛型推导 OutputSchema、JSON Schema 转换、路径表达式解析 |
| `example_session.go` | session | 消息管理、上下文窗口裁剪、多策略 resize、持久化 |
| `example_audit.go` | audit | AuditChain 追加/签名/验证/查询/导出（JSON/CSV/Text） |
| `example_model.go` | model | 3 Provider 构造、AttemptRunner 重试分类、OutputValidator 校验 |
| `example_orchestrator.go` | orchestrator | Agent + Engine + LoopGuard + AuditChain 组装与 Run |
| `example_workspace.go` | workspace | 安全 IO（路径穿越拦截）+ 文件血缘管理 |

> **sandbox** 模块的示例请使用 `sandbox/cmd/sandbox/main.go`（独立 CLI）

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
```
