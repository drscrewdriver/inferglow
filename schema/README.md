# schema - Contract-First Schema 引擎

**模块路径**: `github.com/inferglow/schema`

## 概述

schema 模块是 inferglow 的契约优先 Schema 引擎，提供生产级结构化输出能力。通过 Go 泛型 + 反射实现编译期 + 运行时双重校验，约束 LLM 的输出格式。

## 设计定位

- **被谁依赖**: `flow` 模块（Step 的 Schema 字段）
- **依赖谁**: 无（仅 yaml.v3；schema 模块完全独立，不依赖 model）
- **对标 Python**: `agently/types/data/` 下的 Output/EnsureKeys + `agently/types/plugins/` 下的 Schema 抽象
- **独立可用性**: 不依赖 inferglow 的其他模块（action/session/sandbox/model），完全独立

## 核心类型

### OutputSchema - 输出契约

```go
type OutputSchema struct {
    Format    OutputFormat       // 输出格式（json/markdown/text/xml/yaml 等）
    EnsureAll bool               // 是否强制所有字段
    Fields    map[string]*FieldDef
}

type FieldDef struct {
    Type           DataType
    Description    string
    Ensure         EnsurePolicy     // presence / not_null
    Required       bool
    RequiredFields []string
    Children       map[string]*FieldDef   // 嵌套 struct
    ItemDef        *FieldDef              // 列表元素
}
```

### DataType - 字段类型

```go
const (
    TypeString    DataType = "str"
    TypeInt       DataType = "int"
    TypeFloat     DataType = "float"
    TypeBool      DataType = "bool"
    TypeDict      DataType = "dict"
    TypeList      DataType = "list"
    TypeModel     DataType = "model"
    TypeOptional  DataType = "optional"
)
```

## 核心功能

### 1. Schema 推导 — 泛型方法

从 Go struct 通过 reflect 自动推导 `OutputSchema`：

```go
type WeatherResponse struct {
    City    string `json:"city" description:"城市名称"`
    Temp    float64 `json:"temp"`
    Humidity int    `json:"humidity,omitempty"`  // omitempty = 非必填
}

schema := schema.DefineOutput[WeatherResponse]()
// 自动生成:
// Fields: {
//   "city": {Type: str, Required: true},
//   "temp": {Type: float, Required: true},
//   "humidity": {Type: int, Required: false},
// }
```

### 2. JSON Schema 转换

```go
// OutputSchema → JSON Schema
jsonSchema := GenerateJSONSchema(schema)

// JSON Schema → OutputSchema（反向）
outputSchema := OutputSchemaFromJSONSchema(jsonSchema)
```

### 3. 验证引擎 — ContractEngine

核心验证引擎，支持路径校验和自动重试：

```go
engine := ContractEngine{Schema: mySchema}
result, err := engine.ValidateWithRetry(ctx, llmOutput, maxRetries, retryFn)
```

支持通配符路径（如 `key_issues[*]`）校验。

### 4. JSON 提取

从 LLM 的任意文本中提取符合 Schema 的 JSON：

```go
// 三级策略：直接解析 → 候选提取 → Schema 评分选择
extracted := ExtractJSON(llmText, schemas)

// 修复常见 JSON 错误（全角标点、缺引号等）
fixed := RepairJSONFragment(text)
```

### 5. Blueprint 序列化

TriggerFlow 流程定义的可序列化表示：

```go
// TriggerFlowDefinition — 流程定义
type TriggerFlowDefinition struct {
    Version   string
    Name      string
    Operators []*OperatorDef
    Signals   []*SignalDef
}

// 支持 JSON/YAML/Mermaid 双向转换
serializer := DefinitionSerializer{}
yamlStr, _ := serializer.ToYAML(definition)
definition2, _ := serializer.FromYAML(yamlStr)
```

### 6. 路径工具

解析 dot-path 表达式：

```go
// 解析 "user.name"、"items[*].id" 这类路径
paths := ParsePath("items[*].name")
// → ["items", "name"]
```

## 核心接口一览

```
OutputSchema          → 输出契约
FieldDef              → 字段定义
DataType              → 字段类型枚举
EnsurePolicy          → 存在性策略
ContractEngine        → 验证引擎
DefinitionSerializer  → Blueprint 序列化
TriggerFlowDefinition → 流程定义（可序列化）
```

## 与 model 的关系

schema 模块**不依赖** model 模块（`go.mod` 中无 model 的 require，历史遗留的 replace 指令已删除）。schema 包独立定义了 `OutputSchema`、`FieldDef` 层级结构、`DataType` 类型系统、`EnsurePolicy` 校验策略、以及完整的验证引擎。model 包中的 `OutputSchema` 是独立定义的同名类型，两者无编译期依赖关系。

## 与上层的关系

```
agently 主模块 (Agent 类)
  ├── 用户定义 Go struct
  ├── Schema.DefineOutput[T]() → 自动推导 OutputSchema
  ├── ContractEngine.Validate() → 验证 LLM 输出
  └── LLM ModelRequest.Output = schema (约束输出格式)
```
