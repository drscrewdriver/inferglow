# example_schema - 结构化输出 Schema 示例 / Structured Output Schema Example

## 概述 / Overview

本示例展示了如何使用 `schema` 模块从 Go 结构体类型推导结构化输出定义，并生成 JSON Schema。示例涵盖了泛型方法推导、非泛型方法推导、JSON Schema 转换、路径表达式解析和手动构建 Schema 五种使用方式。

This example demonstrates how to use the `schema` module to derive structured output definitions from Go struct types and generate JSON Schema. It covers five usage patterns: generic derivation, non-generic derivation, JSON Schema conversion, path expression parsing, and manual Schema construction.

## 核心概念 / Core Concepts

- **DefineOutput[T]（泛型推导）**：通过 Go 泛型从结构体类型自动推导 OutputSchema，包含字段名、类型、是否必需和描述信息
- **DefineOutputFromType（非泛型推导）**：在运行时根据 reflect 信息推导 Schema（适用于泛型不可用的场景）
- **GenerateJSONSchema（JSON Schema 转换）**：将内部的 OutputSchema 转换为标准的 JSON Schema 格式
- **ParsePath（路径表达式解析）**：解析如 `items[*].id` 的路径表达式，用于嵌套字段的定位
- **手动构建 Schema**：直接通过 `OutputSchema` 和 `FieldDef` 结构体构建自定义 Schema

- **DefineOutput[T] (Generic Derivation)**：Automatically derives OutputSchema from a struct type via Go generics, including field names, types, required status, and descriptions
- **DefineOutputFromType (Non-generic Derivation)**：Derives Schema at runtime using reflect information (useful when generics are not available)
- **GenerateJSONSchema (JSON Schema Conversion)**：Converts the internal OutputSchema to standard JSON Schema format
- **ParsePath (Path Expression Parsing)**：Parses path expressions like `items[*].id` for locating nested fields
- **Manual Schema Construction**：Builds custom Schemas directly using `OutputSchema` and `FieldDef` structs

## 前置条件 / Prerequisites

- Go 1.21+
- inferglow 项目依赖已安装（`go mod tidy`）
- 无需 LLM API Key

- Go 1.21+
- inferglow project dependencies installed (`go mod tidy`)
- No LLM API Key required

## 使用示例 / Usage Example

代码流程如下 / The code flow is as follows:

**示例 1：泛型方法推导 / Example 1: Generic Derivation**
- 定义 `WeatherResponse` 结构体，包含 City、Country、Temp、Humidity 和嵌套的 Forecasts 数组
- 调用 `schema.DefineOutput[WeatherResponse]()` 自动推导 Schema
- 输出每个字段的类型、必需性和描述信息，以及嵌套结构体的子字段

**示例 2：非泛型方法 / Example 2: Non-generic Method**
- 定义 `AnalysisResult` 结构体，包含 Summary、Issues、Rating 和嵌套的 Sections 数组
- 使用 `schema.DefineOutput[AnalysisResult]()` 推导 Schema

**示例 3：JSON Schema 转换 / Example 3: JSON Schema Conversion**
- 调用 `schema.GenerateJSONSchema(weatherSchema)` 将内部 Schema 转换为标准 JSON Schema
- 输出 JSON Schema 的 type、required 和 properties 信息

**示例 4：路径表达式解析 / Example 4: Path Expression Parsing**
- 解析 `city`、`user.name`、`items[*].id`、`issues[*].severity` 等路径表达式
- 展示如何将路径字符串解析为分段路径组件

**示例 5：手动构建 Schema / Example 5: Manual Schema Construction**
- 直接创建 `OutputSchema` 实例，手动定义 `message` 和 `code` 两个字段

**Example 1: Generic Derivation**
- Define a `WeatherResponse` struct with City, Country, Temp, Humidity, and a nested Forecasts array
- Call `schema.DefineOutput[WeatherResponse]()` to automatically derive the Schema
- Output each field's type, required status, and description, plus nested struct sub-fields

**Example 2: Non-generic Method**
- Define an `AnalysisResult` struct with Summary, Issues, Rating, and a nested Sections array
- Derive the Schema using `schema.DefineOutput[AnalysisResult]()`

**Example 3: JSON Schema Conversion**
- Call `schema.GenerateJSONSchema(weatherSchema)` to convert the internal Schema to standard JSON Schema format
- Output the JSON Schema's type, required, and properties information

**Example 4: Path Expression Parsing**
- Parse path expressions like `city`, `user.name`, `items[*].id`, `issues[*].severity`
- Demonstrate how to decompose path strings into segmented path components

**Example 5: Manual Schema Construction**
- Directly create an `OutputSchema` instance and manually define `message` and `code` fields

## 运行验证 / Running the Example

```
cd examples
go run example_schema.go
```

## 预期输出 / Expected Output

输出将包含以下关键信息 / The output will contain the following key information:

- `WeatherResponse` Schema 的每个字段信息（类型、必需性、描述），包括嵌套的 Forecasts 子字段
- `AnalysisResult` Schema 的每个字段信息
- 转换后的 JSON Schema 结构，包含 `type: object`、`required` 列表和 `properties` 详情
- 路径表达式解析结果，如 `items[*].id` -> `[items, *, id]`
- 手动构建的 Schema 信息，包含 `message`（string，必需）和 `code`（int，可选）

- Each field of the `WeatherResponse` Schema (type, required, description), including nested Forecasts sub-fields
- Each field of the `AnalysisResult` Schema
- The converted JSON Schema structure with `type: object`, `required` list, and `properties` details
- Path expression parsing results, e.g., `items[*].id` -> `[items, *, id]`
- Manually constructed Schema information with `message` (string, required) and `code` (int, optional)

该输出表明 schema 模块能够正确从 Go 结构体推导结构化输出定义，并支持 JSON Schema 转换、路径解析和手动构建等多种使用方式。

This output confirms that the schema module can correctly derive structured output definitions from Go structs, and supports multiple usage patterns including JSON Schema conversion, path parsing, and manual construction.