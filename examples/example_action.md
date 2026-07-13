# example_action - 动作注册与调用示例 / Action Registration and Invocation

## 概述 / Overview

本示例展示了如何使用 `action` 模块将 Go 函数注册为 LLM 可调用的 Action（工具），并通过 Registry 执行它们。示例涵盖了三种不同函数签名风格的 Action 定义方式，以及注册、执行和错误处理的完整流程。

This example demonstrates how to use the `action` module to wrap Go functions as LLM-callable Actions and execute them via the Registry. It covers three different function signature styles for Action definition, along with the full workflow of registration, execution, and error handling.

## 核心概念 / Core Concepts

- **Action（动作）**：一个可被 LLM 调用的工具，包含名称、描述和 JSON Schema
- **`action.New()`（自动包装）**：从 Go 函数签名自动推导参数 JSON Schema，创建 `LocalFunctionExecutor`
- **Registry（注册表）**：管理多个 Action 的注册、查找和执行
- **函数签名支持**：`func(ctx, InputT) (OutputT, error)` 和 `func(InputT) (OutputT, error)` 两种风格
- **错误处理**：Action 执行失败时返回包含错误信息的 `ActionResult`

- **Action**：A tool callable by the LLM, consisting of a name, description, and JSON Schema
- **`action.New()` (Auto-wrapping)**：Automatically derives the argument JSON Schema from the Go function signature and creates a `LocalFunctionExecutor`
- **Registry**：Manages registration, lookup, and execution of multiple Actions
- **Function Signature Support**：Both `func(ctx, InputT) (OutputT, error)` and `func(InputT) (OutputT, error)` styles
- **Error Handling**：Returns an `ActionResult` containing error information when Action execution fails

## 前置条件 / Prerequisites

- Go 1.21+
- inferglow 项目依赖已安装（`go mod tidy`）
- 无需 LLM API Key（本示例仅测试 Action 模块）

- Go 1.21+
- inferglow project dependencies installed (`go mod tidy`)
- No LLM API Key required (this example tests only the Action module)

## 使用示例 / Usage Example

代码流程如下 / The code flow is as follows:

1. **定义 Go 函数**：定义了三个函数，展示不同的签名风格和参数结构
   - `addNumbers(ctx, AddRequest)` -- 带 `context.Context` 参数的函数
   - `greet(GreetRequest)` -- 仅带输入参数的函数
   - `greetWithTitleFn(GreetTitleRequest)` -- 带可选字段和错误处理的函数
2. **创建 Action**：使用 `action.New("add", "description", addNumbers)` 自动包装每个函数
3. **注册到 Registry**：创建 `action.NewRegistry()`，通过 `registry.Register(a)` 注册所有 Action
4. **执行 Action**：通过 `registry.Execute(ctx, "add", args)` 按名称调用 Action
5. **错误处理**：传入空参数调用 `greet_with_title`，观察错误返回

1. **Define Go Functions**：Three functions are defined to demonstrate different signature styles and parameter structures
   - `addNumbers(ctx, AddRequest)` -- A function with `context.Context` parameter
   - `greet(GreetRequest)` -- A function with only input parameters
   - `greetWithTitleFn(GreetTitleRequest)` -- A function with optional fields and error handling
2. **Create Actions**：Use `action.New("add", "description", addNumbers)` to auto-wrap each function
3. **Register with Registry**：Create `action.NewRegistry()` and register all Actions via `registry.Register(a)`
4. **Execute Actions**：Invoke Actions by name via `registry.Execute(ctx, "add", args)`
5. **Error Handling**：Call `greet_with_title` with empty arguments to observe error returns

## 运行验证 / Running the Example

```
cd examples
go run example_action.go
```

## 预期输出 / Expected Output

输出将包含以下关键信息 / The output will contain the following key information:

- 每个 Action 的名称、描述和自动生成的 Schema 信息
- `Registered 3 actions: [add greet greet_with_title]` -- 注册成功
- `add` 执行结果：`Result OK=true, Status=success, Result=30` -- 10 + 20 = 30
- `greet` 执行结果：`Result OK=true, Status=success, Result=Hello, World!`
- `greet_with_title` 执行结果：`Result OK=true, Status=success, Result=Hello, Mr. Joshua!`
- 错误处理结果：`Result OK=false, Status=error, Error=name is required` -- 参数校验失败

- Each Action's name, description, and auto-generated Schema information
- `Registered 3 actions: [add greet greet_with_title]` -- Registration successful
- `add` execution result: `Result OK=true, Status=success, Result=30` -- 10 + 20 = 30
- `greet` execution result: `Result OK=true, Status=success, Result=Hello, World!`
- `greet_with_title` execution result: `Result OK=true, Status=success, Result=Hello, Mr. Joshua!`
- Error handling result: `Result OK=false, Status=error, Error=name is required` -- Validation failure

该输出表明 Action 的创建、注册、执行和错误处理流程均正常工作，且 `action.New()` 能正确从不同函数签名推导 JSON Schema。

This output confirms that Action creation, registration, execution, and error handling all work correctly, and that `action.New()` can properly derive JSON Schema from different function signatures.