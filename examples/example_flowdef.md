# example_flowdef.go — 声明式 Flow 定义

## 中文说明

演示如何使用 `flow/flowdef` 和 `flow/stage` 包以声明式方式定义和加载 Flow，无需硬编码步骤拼接。

### 核心概念
- **FlowDef**：声明式 Flow 定义结构（YAML/JSON 或编程构建）
- **Stage**：可注册的步骤执行单元，通过 `stage.Registry` 管理
- **Adapter**：将 FlowDef 编译为可执行的 `*flow.Flow`
- **DependsOn**：声明步骤依赖关系，Adepter 自动拓扑排序

### 运行方式
```bash
cd examples
go run example_flowdef.go
```

### 示例输出
```
=== Example: Declarative FlowDef ===
FlowDef validated OK
Flow built from FlowDef
Step: greet → Hello, InferGlow!
Step: uppercase → HELLO, INFERGLOW!
```

---

## English Description

Demonstrates declarative Flow definition using `flow/flowdef` and `flow/stage` packages, eliminating hard-coded step wiring.

### Key Concepts
- **FlowDef**: Declarative flow definition structure (YAML/JSON or programmatic)
- **Stage**: Registered step execution unit managed by `stage.Registry`
- **Adapter**: Compiles FlowDef into executable `*flow.Flow`
- **DependsOn**: Declare step dependencies, auto topo-sorted by Adapter

### Run
```bash
cd examples
go run example_flowdef.go
```