# example_flow - 流程编排示例 / Flow Orchestration Example

## 概述 / Overview

本示例展示了如何使用 `flow` 模块编排多个步骤（Step）组成可执行流程。示例涵盖了三种流程模式：线性顺序执行、带条件分支的执行、以及带 Schema 校验的步骤定义。

This example demonstrates how to use the `flow` module to orchestrate multiple Steps into executable flows. It covers three flow patterns: linear sequential execution, conditional branching execution, and step definition with Schema validation.

## 核心概念 / Core Concepts

- **Step（步骤）**：流程中的最小执行单元，包含名称和执行函数
- **Flow（流程）**：由多个 Step 组成的执行管道，支持链式调用
- **线性流程**：Step 按顺序依次执行，前一个的输出作为后一个的输入
- **条件分支**：根据条件判断结果选择不同的执行路径
- **StepLog（步骤日志）**：记录流程中每个 Step 的执行状态和输出
- **Schema 校验**：Step 可关联输出 Schema，用于结构化数据验证

- **Step**：The smallest execution unit within a flow, consisting of a name and an execution function
- **Flow**：An execution pipeline composed of multiple Steps, supporting chained calls
- **Linear Flow**：Steps execute sequentially, with each step's output feeding into the next
- **Conditional Branch**：Selects different execution paths based on a condition evaluation
- **StepLog**：Records the execution status and output of each Step in the flow
- **Schema Validation**：Steps can be associated with an output Schema for structured data validation

## 前置条件 / Prerequisites

- Go 1.21+
- inferglow 项目依赖已安装（`go mod tidy`）
- 无需 LLM API Key

- Go 1.21+
- inferglow project dependencies installed (`go mod tidy`)
- No LLM API Key required

## 使用示例 / Usage Example

代码流程如下 / The code flow is as follows:

**示例 1：线性流程 / Example 1: Linear Flow**
- 创建三个 Step：`parse`（解析）-> `validate`（验证）-> `format`（格式化）
- 使用 `flow.NewFlow().AddStep(parseStep).To(validateStep).To(formatStep).Build()` 构建流程
- 输入 `"Hello World!"`，依次经过三个步骤处理

**示例 2：条件分支 / Example 2: Conditional Branch**
- 创建 `analyze` Step，根据输入数字的正负返回不同结果
- 使用 `.If(condition, positiveStep, negativeStep)` 实现分支
- 分别用 `42`（正数）和 `-10`（负数）测试两条路径

**示例 3：Schema 校验 / Example 3: Schema Validation**
- 定义 `WeatherResult` 结构体，包含 City、Temp、Humidity 字段
- 创建返回结构化数据的 Step，Schema 字段可用于后续校验

**Example 1: Linear Flow**
- Create three Steps: `parse` -> `validate` -> `format`
- Build the flow using `flow.NewFlow().AddStep(parseStep).To(validateStep).To(formatStep).Build()`
- Input `"Hello World!"`, processed through the three steps sequentially

**Example 2: Conditional Branch**
- Create an `analyze` Step that returns different results based on whether the input number is positive or negative
- Use `.If(condition, positiveStep, negativeStep)` to implement branching
- Test both paths with `42` (positive) and `-10` (negative)

**Example 3: Schema Validation**
- Define a `WeatherResult` struct with City, Temp, and Humidity fields
- Create a Step that returns structured data; the Schema field is available for subsequent validation

## 运行验证 / Running the Example

```
cd examples
go run example_flow.go
```

## 预期输出 / Expected Output

输出将包含以下关键信息 / The output will contain the following key information:

**线性流程输出 / Linear Flow Output:**
- `Status: completed` -- 流程执行完成
- 三个 Step 的日志：`parse` -> `validate` -> `format`，依次输出处理结果

**条件分支输出 / Conditional Branch Output:**
- 输入 `42` 时，执行路径为 `analyze` -> `handle_positive`
- 输入 `-10` 时，执行路径为 `analyze` -> `handle_negative`
- 两条路径的 Status 均为 `completed`

**Schema 校验输出 / Schema Validation Output:**
- `Step with Schema created (Schema field available for validation)` -- Step 携带 Schema 信息

- **Linear Flow**: `Status: completed` with three Step logs in sequence
- **Conditional Branch**: Input `42` follows `analyze` -> `handle_positive`; input `-10` follows `analyze` -> `handle_negative`
- **Schema Validation**: Confirms the Step carries Schema information for structured data validation

该输出表明 flow 模块支持线性编排、条件分支和结构化数据验证三种核心功能，StepLog 提供了完整的执行追踪能力。

This output confirms that the flow module supports three core capabilities: linear orchestration, conditional branching, and structured data validation, with StepLog providing comprehensive execution tracing.