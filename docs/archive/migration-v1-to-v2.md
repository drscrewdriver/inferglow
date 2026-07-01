# 迁移指南：v1 → v2（可插拔架构）

Inferglow v2 将沙箱执行与安全特性从硬依赖改造为**可选依赖**。本指南帮助 v1 用户完成迁移。

## 一、Breaking Changes 摘要

| 变更项 | v1 行为 | v2 行为 | 影响 |
|-------|--------|--------|------|
| `action` 模块沙箱依赖 | 直接依赖 `sandbox` | 通过 `with_sandbox` build tag 可选 | 默认编译不再引入 `sandbox` |
| `session.SecurityHook` 类型 | 定义在 `session` 包 | 移至 `security/sessionhook` 子包 | 导入路径变更 |
| `session.NewSecurityHook` | 在 `session` 包 | 在 `security/sessionhook` 包 | 导入路径变更 |
| `agent.OutputInjectionHook` 类型 | 定义在 `orchestrator/agent` 包 | 移至 `security/agenthook` 子包 | 导入路径变更 |
| `agent.PIIMasker` 字段类型 | `*pii.Masker` | `PIIMasker` 接口 | `WithPIIMasker` 参数类型变更 |
| `orchestrator/go.mod` | 直接依赖 `security` | 不直接依赖 `security` | 需在用户代码层注入 |
| `session/go.mod` | 直接依赖 `security` | 不直接依赖 `security` | 需在用户代码层注入 |

## 二、迁移步骤

### 步骤 1：编译参数调整

**默认模式（无沙箱）**——无需任何参数，体积更小：

```bash
go build ./...
```

**沙箱模式**——若需要 `SandboxExecutor`，添加 `-tags with_sandbox`：

```bash
go build -tags with_sandbox ./...
```

> 若 v1 代码使用了 `action.NewSandboxExecutor`，迁移后必须用 `-tags with_sandbox` 编译，否则 `SandboxExecutor.Execute` 会返回错误提示需启用该 tag。

### 步骤 2：安全钩子注入方式变更（sessionhook）

`session` 包不再提供 `SecurityHook` 实现，仅保留 `MessageHook` 接口与 `WithSecurityHook` Option。实现移至 `security/sessionhook`。

```go
// 旧方式（v1）
import "github.com/inferglow/session"

hook := session.NewSecurityHook(cfg)
sess := session.NewSessionWithOptions("id", 10, session.WithSecurityHook(hook))

// 新方式（v2）
import (
    "github.com/inferglow/session"
    "github.com/inferglow/security/sessionhook"
)

hook := sessionhook.NewSecurityHook(cfg)
sess := session.NewSessionWithOptions("id", 10, session.WithSecurityHook(hook))
```

`session.WithSecurityHook` 签名不变（接受 `MessageHook` 接口），`sessionhook.SecurityHook` 实现了该接口。

### 步骤 3：输出安全钩子注入方式变更（agenthook）

`orchestrator/agent` 包不再提供 `OutputInjectionHook` 实现，仅保留 `OutputSecurityHook` 接口与 `WithOutputSecurityHook` Option。实现移至 `security/agenthook`。

```go
// 旧方式（v1）
import "github.com/inferglow/orchestrator/agent"

hook := agent.NewOutputInjectionHook(cfg)
ag := agent.New(sess, actExt, llm, agent.WithOutputSecurityHook(hook))

// 新方式（v2）
import (
    "github.com/inferglow/orchestrator/agent"
    "github.com/inferglow/security/agenthook"
)

hook := agenthook.NewOutputInjectionHook(cfg)
ag := agent.New(sess, actExt, llm, agent.WithOutputSecurityHook(hook))
```

### 步骤 4：PIIMasker 接口注入

`Agent.piiMasker` 字段类型从 `*pii.Masker` 改为 `PIIMasker` 接口。`security/agenthook` 提供 `PIIMasker` 适配器包装 `*pii.Masker`。

```go
// 旧方式（v1）
import (
    "github.com/inferglow/orchestrator/agent"
    "github.com/inferglow/security/pii"
)

masker := pii.NewMasker(pii.DefaultConfig())
ag := agent.New(sess, actExt, llm, agent.WithPIIMasker(masker))

// 新方式（v2）
import (
    "github.com/inferglow/orchestrator/agent"
    "github.com/inferglow/security/agenthook"
    "github.com/inferglow/security/pii"
)

masker := pii.NewMasker(pii.MaskConfig{})
ag := agent.New(sess, actExt, llm, agent.WithPIIMasker(agenthook.NewPIIMasker(masker)))
```

> `agent.WithPIIMasker` 现在接受 `agent.PIIMasker` 接口而非 `*pii.Masker`。`agenthook.NewPIIMasker` 返回的 `*agenthook.PIIMasker` 实现了该接口。

### 步骤 5：go.mod 调整

若你的模块直接依赖 `session` 或 `orchestrator`，且需要安全特性，需在**你的模块**的 `go.mod` 中添加 `security` 依赖：

```
require github.com/inferglow/security v0.0.0
replace github.com/inferglow/security => ../security
```

`session` 与 `orchestrator` 自身不再 require `security`。

## 三、完整迁移示例

```go
package main

import (
    "context"

    "github.com/inferglow/action"
    "github.com/inferglow/orchestrator/agent"
    "github.com/inferglow/security/agenthook"
    "github.com/inferglow/security/pii"
    promptinjection "github.com/inferglow/security/prompt_injection"
    "github.com/inferglow/security/sessionhook"
    "github.com/inferglow/session"
)

func main() {
    ctx := context.Background()

    // 1. Session + 输入侧注入检测（sessionhook）
    secHook := sessionhook.NewSecurityHook(promptinjection.NewDefaultConfig())
    sess := session.NewSessionWithOptions("demo", 4000, session.WithSecurityHook(secHook))

    // 2. ActionExtension
    actExt := agent.NewActionExtension()
    greetAction, _ := action.New("greet", "greet", func(ctx context.Context, req map[string]any) (string, error) {
        return "hello", nil
    })
    _ = actExt.Register(greetAction)

    // 3. 输出侧注入检测 + PII 脱敏（agenthook）
    outHook := agenthook.NewOutputInjectionHook(promptinjection.NewDefaultConfig())
    piiMasker := agenthook.NewPIIMasker(pii.NewMasker(pii.MaskConfig{}))

    llm := /* your ModelRequester */
    ag := agent.New(sess, actExt, llm,
        agent.WithMaxRounds(10),
        agent.WithOutputSecurityHook(outHook),
        agent.WithPIIMasker(piiMasker),
    )

    _, _ = ag.Run(ctx, "Hello")
}
```

## 四、FAQ

### Q1：迁移后默认编译报错找不到 `sandbox` 包？

A：v2 默认模式不引入 `sandbox`。若你的代码直接引用 `github.com/inferglow/sandbox`，要么改用 `-tags with_sandbox` 编译，要么移除对 `sandbox` 的直接引用（仅通过 `action.SandboxExecutor` 间接使用时，默认模式下会得到 stub）。

### Q2：`session.NewSecurityHook` 找不到了？

A：已移至 `security/sessionhook.NewSecurityHook`。更新导入路径即可。

### Q3：`agent.WithPIIMasker` 编译报类型不匹配？

A：v2 中 `WithPIIMasker` 接受 `agent.PIIMasker` 接口而非 `*pii.Masker`。用 `agenthook.NewPIIMasker(piiMasker)` 包装后再传入。

### Q4：不注入安全钩子会有性能影响吗？

A：不会。`securityHook` / `outputHook` / `piiMasker` 为 nil 时，`Session.AddMessageChecked` 与 `Agent.Run` 会跳过对应检查，**零开销**。

### Q5：可以只启用部分安全特性吗？

A：可以。三个接口（`MessageHook`、`OutputSecurityHook`、`PIIMasker`）相互独立，按需注入即可。例如只启用 PII 脱敏而不启用注入检测。

### Q6：如何同时启用沙箱和安全特性？

A：编译时加 `-tags with_sandbox`，运行时注入 `sessionhook` / `agenthook`。两者互不影响。

### Q7：v2 是否向后兼容？

A：**不完全兼容**。导入路径变更和 `WithPIIMasker` 参数类型变更是 breaking change，需要按本指南调整代码。但接口语义和行为保持一致，迁移后逻辑无需改变。
