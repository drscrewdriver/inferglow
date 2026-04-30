# action - Action Runtime

**模块路径**: `github.com/inferglow/action`

## 概述

action 模块是 Inferglow 的 Action Runtime，提供将 Go 函数注册为可发现、可校验、可执行的动作单元的能力。该模块**完全独立**，不依赖 inferglow 的任何其他子模块。

## 设计定位

- **被谁依赖**: 上层业务逻辑（agently 主模块的 Agent 类）
- **依赖谁**: 无（仅依赖 stdlib）— 完全独立
- **对标 Python**: `agently/core/operation/Action/Action.py` + `ActionRuntime` + `ActionFlow`
- **独立可用性**: ✅ 完全独立，可被任何模块嵌入

## 核心类型

### Action — 可执行动作单元

```go
type Action struct {
    Name        string            // 动作名称
    Description string            // 动作描述
    Schema      map[string]any    // JSON Schema（输入参数）
    Executor    ActionExecutor    // 执行器
    Tags        []string          // 标签
}
```

### ActionExecutor — 执行器接口

```go
type ActionExecutor interface {
    Execute(ctx context.Context, input map[string]any) (*ActionResult, error)
}
```

### ActionResult — 执行结果

```go
type ActionResult struct {
    OK     bool    // 是否成功
    Status string  // "success" | "error" | "blocked"
    Result any     // 执行结果
    Error  string  // 错误信息
}
```

### ActionRegistry — 并发安全注册表

```go
type ActionRegistry struct {
    // 线程安全的 actions 存储
}

func NewRegistry() *ActionRegistry
func (r *ActionRegistry) Register(a *Action) error
func (r *ActionRegistry) Execute(ctx, name, input) (*ActionResult, error)
func (r *ActionRegistry) List() []string
func (r *ActionRegistry) Get(name) (*Action, error)
```

## LocalFunctionExecutor — 函数包装器

将普通 Go 函数自动包装为 `ActionExecutor`，支持三种函数签名：

```go
// 签名 1: 带 context 和 error 返回
func(ctx context.Context, input InputT) (OutputT, error)

// 签名 2: 无 context，带 error
func(input InputT) (OutputT, error)

// 签名 3: 带 context，无 error
func(ctx context.Context, input InputT) OutputT
```

### 使用示例

```go
// 定义一个函数
func AddNumbers(a int, b int) (int, error) {
    return a + b, nil
}

// 包装为 Action
action, err := New("add", "Add two numbers", AddNumbers)
if err != nil { return err }

// 注册到 Registry
registry := NewRegistry()
registry.Register(action)

// 执行
result, _ := registry.Execute(ctx, "add", map[string]any{
    "a": 10,
    "b": 20,
})
// result.Result = 30
```

### 类型转换机制

`LocalFunctionExecutor` 通过 JSON marshal/unmarshal 实现 loose-to-strict 类型转换：
1. 将 `map[string]any` 输入转为 JSON
2. 反序列化为函数的强类型 InputT 参数
3. 通过 reflect 调用函数
4. Panic 恢复机制捕获运行时错误

### Schema 自动生成

从 struct 的 `json` tag 自动生成 JSON Schema：

```go
type SearchRequest struct {
    Query string `json:"query"`
    Limit int    `json:"limit,omitempty"`
}

// 自动生成的 Schema:
// {
//   "type": "object",
//   "properties": {
//     "query": {"type": "string"},
//     "limit": {"type": "integer"}
//   },
//   "required": ["query"]
// }
```

## Action 规格（ActionSpec）

完整的 Action 规格定义，包含安全属性：

```go
type ActionSpec struct {
    ActionID          string
    Name              string
    Desc              string
    Kwargs            map[string]KwargsDef
    Returns           ReturnType
    Tags              []string
    DefaultPolicy     *ActionPolicy
    SideEffectLevel   SideEffectLevel   // "read" | "write" | "exec"
    ApprovalRequired  bool
    SandboxRequired   bool
    ReplaySafe        bool
    ExposeToModel     bool
}
```

## 核心接口一览

```
Action                → 动作单元
ActionExecutor        → 执行器接口
ActionResult          → 执行结果
ActionRegistry        → 注册表
LocalFunctionExecutor → 函数执行器
ActionSpec            → 完整规格
ActionPolicy          → 执行策略
SideEffectLevel       → 副作用级别
```

## 与 Action Runtime 的对应关系

在 Python Agently 中，Action 是三层插件架构：

```
Action (单一抽象)
  → ActionRuntime (规划与调度)
    → ActionFlow (PLAN → EXECUTE 循环)
```

Go 版的 action 模块实现了最底层的 **Action 注册与执行框架**（对应 Python 的 `Action` + `ActionRegistry`）。上层的 `ActionRuntime` 和 `ActionFlow` 规划调度逻辑将在 agently 主模块中实现。

## 与上层的关系

```
agently 主模块 (Agent 类)
  ├── Agent 通过 ActionExtension 注入 action 能力
  ├── @agent.action_func 装饰器 → 自动包装为 Action
  ├── agent.use_actions() → 批量注册
  ├── Action.async_plan_and_execute() → 规划 + 执行循环
  ├── ActionRegistry.Execute(name, input) → 实际执行
  └── ActionResult → 反馈给 LLM 继续下一轮
```
