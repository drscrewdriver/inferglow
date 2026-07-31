# 工具组织与调度使用指南

> 说明如何把 Go 函数做成可被 Agent 调用的「工具」（`Action`），对其进行**注册、分组、过滤、调度**。
> 对应示例：`examples/example_toolgroup.go`（按组注册/列举/过滤）。

## 1. 核心概念

| 概念 | 类型 | 作用 |
|------|------|------|
| `Action` | 结构体 | 一个可被 LLM 调用的工具单元（名称 + 描述 + JSON Schema + 执行器 + Tags） |
| `ActionRegistry` | 注册表 | 扁平注册/查询/执行 `Action` 的热路径 |
| `ToolGroup` | 结构体 | 命名的工具组声明（由 `Tags` 选择成员） |
| `GroupRegistry` | 派生视图 | 在 `ActionRegistry` 之上把工具按标签组织成组，**不复制数据** |
| `ToolFilter` | 过滤器 | 请求期（per-request）动态过滤工具 |
| `ActionDispatcher` | 调度器 | 并发执行一批 `ActionCall` |

> **架构说明**：`GroupRegistry` 是 `ActionRegistry` 之上的一层**派生视图**，复用现有 `Tags` 元数据，不改动 `ActionRegistry` 的任何语义。`ActionRegistry` 的 `Register/Get/List/Execute` 热路径完全不变——这是遵守 [EXTENDING.md](../../docs/EXTENDING.md) 的「Registry（#6）机制能力增强」而非新增第 8 种扩展机制。

## 2. 扁平注册：`Action` + `ActionRegistry`

这是最基础的方式：把普通 Go 函数包装成 `Action`，逐个注册。

```go
import "github.com/inferglow/action"

// 1. 定义普通 Go 函数
type AddRequest struct {
	A int `json:"a"`
	B int `json:"b"`
}

func addNumbers(ctx context.Context, req AddRequest) (int, error) {
	return req.A + req.B, nil
}

// 2. 用 action.New 自动包装（推导 Schema + 创建 LocalFunctionExecutor）
addAction, err := action.New("add", "Add two numbers together", addNumbers)
if err != nil {
	return err
}

// 3. 注册到 Registry
registry := action.NewRegistry()
_ = registry.Register(addAction)

// 4. 执行
result, _ := registry.Execute(ctx, "add", map[string]any{"a": 10, "b": 20})
fmt.Printf("OK=%v Status=%s Result=%+v\n", result.OK, result.Status, result.Result)
```

`ActionRegistry` 关键方法：

- `Register(a *Action) error` — 注册（拒绝空名 / nil 执行器 / 重名）
- `Get(name) (*Action, error)` / `GetAction(name) *Action` — 查询
- `List() []string` — 列出全部名称（排序）
- `Execute(ctx, name, input) (*ActionResult, error)` — 执行
- `Tag(names, tags)` — 给动作追加标签
- `ListActionNames(tags []string) []string` — 返回包含**全部**给定标签的动作名（排序）

## 3. 按组注册：`ToolGroup` + `GroupRegistry`

> 工具多了以后，扁平管理不够直观。用 `ToolGroup` 把标签语义化地组织成「只读组」「plan 组」等。

### 3.1 约定：`group:<name>` 保留标签

`GroupRegistry` 通过**保留标签约定** `group:<name>` 派生组成员。一个 `Action` 的 `Tags` 包含某 `ToolGroup` 的 `Tags` 中**全部**标签，即属于该组。

```go
// 给动作打上组标签
registry.Tag([]string{"ls", "stat"}, []string{"group:readonly", "readonly"})
registry.Tag([]string{"rm", "mv"}, []string{"group:write", "write"})
```

### 3.2 创建并注册组

```go
gr := action.NewGroupRegistry(registry)

_ = gr.Register(&action.ToolGroup{
	Name:        "readonly",
	Description: "只读工具组",
	Tags:        []string{"group:readonly"},
	Policy:      &action.GroupPolicy{ReadOnly: true, MaxLevel: action.SideEffectRead},
})

_ = gr.Register(&action.ToolGroup{
	Name:        "plan",
	Description: "plan 模式可用工具",
	Tags:        []string{"group:readonly"},
	// Policy 可留 nil（可选）
})
```

### 3.3 按组列举与校验

```go
names, err := gr.ListActionNames("readonly") // 组内动作名（排序）
if err != nil {
	return err
}

exists := gr.HasAction("readonly", "ls")       // true
registered := gr.List()                        // 所有组名
readonlyGroup, _ := gr.Get("readonly")         // 取组定义
if gr.Unregister("plan") {                     // 注销组
	fmt.Println("plan group removed")
}
```

`GroupRegistry` 关键方法：`Register` / `Get` / `List` / `Unregister` / `ListActionNames` / `HasAction`。全部并发安全（内部 RWMutex）。

> `GroupPolicy`（`ReadOnly` / `MaxLevel`）目前是**组级权限语义的预留**，将用于后续把组级只读约束接入请求期 `ToolFilter`。

## 4. 请求期过滤：`ToolFilter`

`ToolFilter` 在**请求期**按「允许 / 禁止 / 最大副作用级别」过滤工具，与注册期静态元数据解耦。

```go
// 预置 Profile
readonly := action.ReadOnlyProfile()  // MaxLevel = SideEffectRead（只读工具）
balanced := action.BalancedProfile()  // MaxLevel = SideEffectWrite（读+写，禁网络/执行）
permissive := action.PermissiveProfile() // 允许全部（等价 nil filter）
custom := action.CustomProfile([]string{"ls", "cat"}, []string{"rm"}) // 显式白/黑名单

// 自定义过滤器
filter := &action.ToolFilter{
	Allowed:   []string{"ls", "cat", "rm"},
	Forbidden: []string{"rm"},
	MaxLevel:  action.SideEffectRead,
}

// 过滤优先序：Forbidden > Allowed > MaxLevel
ok := filter.IsAllowed("ls", spec) // true
```

## 5. 调度：`ActionDispatcher`

`ActionDispatcher` 并发执行一批 `ActionCall`，并可选挂接审计钩子。

```go
import "github.com/inferglow/orchestrator/actionruntime"

d := actionruntime.NewActionDispatcher(registry)
// 或带审计：
// d := actionruntime.NewActionDispatcherWithAudit(registry, &audit.NoOpHook{})

results := d.Execute(ctx, []actionruntime.ActionCall{
	{ActionID: "1", ToolName: "add", Kwargs: map[string]any{"a": 1, "b": 2}},
	{ActionID: "2", ToolName: "greet", Kwargs: map[string]any{"name": "World"}},
})
// 结果按传入顺序返回；支持 ExecuteInterruptible 带抢占信号
```

## 6. 在编排层使用：`ActionExtension`

`ActionExtension` 是 orchestrator 对 `ActionRegistry` + `GroupRegistry` 的简化封装，提供按组能力。

```go
ext := extension.NewActionExtension()
_ = ext.Register(a)                    // 扁平注册
_ = ext.RegisterGroup(&action.ToolGroup{Name: "readonly", Tags: []string{"group:readonly"}})

all := ext.ListActions()                      // 全部工具定义（向后兼容）
readonlyTools, err := ext.ListActionsByGroup("readonly") // 按组
filtered, err := ext.ListActionsFiltered("readonly", action.ReadOnlyProfile(), specs) // 组 + 请求期过滤
_ = ext.Execute(ctx, "add", map[string]any{"a": 1, "b": 2})
```

- `ListActions()` 保持向后兼容，返回全部工具定义。
- `ListActionsByGroup(group)` 只返回该组成员。
- `ListActionsFiltered(group, filter, specs)` 组合「组成员」+「`ToolFilter`」——先取组内成员，再逐个用 `filter.IsAllowed` 判断，结果严格落在组内。

## 7. 组合模式：plan 模式

「只读组 + `ReadOnlyProfile`」是典型组合：限制工具**集合**到只读组，同时限制**副作用级别**到 read。

```go
if mode == "plan" {
	// 只暴露只读组，且仅允许 read 级别副作用
	tools, err := ext.ListActionsFiltered("readonly", action.ReadOnlyProfile(), specs)
	// 把 tools 注入 prompt，Agent 只能看到/调用这些只读工具
}
```

## 8. 完整可运行示例

请直接运行 `examples/example_toolgroup.go`：

```bash
cd examples
go run example_toolgroup.go
```

## 9. 关键文件

| 文件 | 内容 |
|------|------|
| `action/action.go` | `Action` 结构体、`New`、`NewRegistry` |
| `action/registry.go` | `ActionRegistry` 的注册/查询/标签/过滤 |
| `action/group.go` | `ToolGroup` / `GroupPolicy` / `GroupRegistry` |
| `action/tool_filter.go` | `ToolFilter` 与预置 Profile |
| `action/spec.go` | `ActionSpec` / `SideEffectLevel` |
| `orchestrator/actionruntime/dispatcher.go` | `ActionDispatcher` |
| `orchestrator/agent/internal/extension/action.go` | `ActionExtension`（按组能力） |