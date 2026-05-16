# InferGlow vs Eino 成熟度对比分析报告

> 生成时间：2026-07-22
> 分析对象：InferGlow（本地项目）vs Eino（CloudWeGo 开源框架）
> 数据来源：两项目源码静态分析

---

## 一、执行摘要

InferGlow 是一个 Go 语言实现的 AI Agent 基础设施框架，定位为对标 Python Agently 的底层零件库。Eino 是 CloudWeGo（字节跳动）出品的成熟 LLM 应用开发框架，对标 LangChain + Google ADK，已具备完整的生产级能力。

**核心结论**：

- **功能层面**：InferGlow 有 **9 项差异化优势**（增量 JSON 解析、TriggerFlow 事件驱动、LoopGuard、StreamTimeout、安全模块、沙箱、审计链、契约引擎、工具标签），但缺失 **15 项关键功能**（运行中抢占/取消、模型故障转移、Checkpoint 持久化、流式工具、多模态消息、8 个中间件、4 种预置 Agent、多 Agent 协作、图状态管理、通用流式抽象、回调切面、泛型类型安全等）。
- **工程层面**：InferGlow 在测试体系上略有优势（180 测试 + 7 benchmark + 27 bugfix 回归测试），但完全缺失 CI/CD、linting、license、mock 工具链。
- **定位**：InferGlow 目前是"零件齐备但缺少组装线和出厂质检"的状态，Eino 已经是"可交付的生产线"。

---

## 二、量化数据总览

| 指标 | InferGlow | Eino | 说明 |
|------|-----------|------|------|
| 测试文件数 | 180 | 136 | InferGlow 测试数量更多 |
| 源码文件数 | 159 | 202 | Eino 源码量更大 |
| 测试/源码比 | 1.13 | 0.67 | InferGlow 测试覆盖密度更高 |
| Benchmark 文件 | 7 | 0 | InferGlow 有性能基线，Eino 无 |
| Bugfix 测试文件 | 27 | 0 | InferGlow 有大量回归测试，暗示迭代中修复了较多 bug |
| Generic 测试文件 | 0 | 4 | Eino 有泛型测试，InferGlow 未使用泛型 |
| Mock 文件数 | 1（手写） | 6（自动生成） | Eino 使用 go.uber.org/mock 工具链 |
| 示例文件数 | 10 | 外部仓库 | InferGlow 内置示例，Eino 有独立 eino-examples 仓库 |
| CI/CD 配置 | 无 | 3 个 workflow | Eino 有 pr-check.yml、tests.yml、tag-notification.yml |
| Linting 配置 | 无 | .golangci.yaml | Eino 有 revive/godoclint/funlen/cyclop 规则 |
| License Header | 无 | .licenserc.yaml | Eino 统一 Apache 2.0 头 |
| Go 版本要求 | 1.25.0 | 1.18 | Eino 兼容性更广 |
| 外部依赖数 | 1（yaml.v3） | 12+ | Eino 使用 sonic/uuid/testify 等 |
| 预置 Agent 模式 | 1 | 4 | Eino 有 ChatModelAgent/DeepAgent/Supervisor/PlanExecute |
| 组件抽象类型 | 1（model） | 7 | Eino 有 model/tool/retriever/embedding/indexer/prompt/document |
| 中间件数量 | 0 | 8 | Eino 有 agentsmd/dynamictool/filesystem/patchtoolcalls/plantask/reduction/skill/summarization |
| 编排源码文件 | 16（flow） | 35（compose） | Eino 编排能力更丰富 |
| Schema 源码文件 | 10 | 14 | Eino schema 更完善（含多模态、流式、文档） |

**数据验证命令**：
```bash
# 测试文件数
find <project> -name "*_test.go" -type f | wc -l
# 源码文件数
find <project> -name "*.go" -not -name "*_test.go" -type f | wc -l
# Benchmark 文件
find <project> -name "*_bench_test.go" -type f | wc -l
# Bugfix 测试
find <project> -name "*bugfix_test.go" -type f | wc -l
```

---

## 三、十维度对比分析

### 维度 1：工程基础设施

**InferGlow 现状**：
- 无 `.github/workflows/` 目录，完全没有 CI/CD 配置
- 无 `.golangci.yaml` 或任何 linting 配置
- 无 `.licenserc.yaml`，源码文件无 license header
- 无 `CONTRIBUTING.md`、`CODE_OF_CONDUCT.md`
- 无 mock 生成工具链（仅 1 个手写 mock 文件 `sandbox/docker_mock_test.go`）
- Go 版本要求 1.25.0（过于前沿，限制用户群）
- 依赖极简（仅 `yaml.v3`），避免供应链风险但也缺少成熟工具

**Eino 现状**：
- `.github/workflows/` 下有 3 个 CI 配置：`pr-check.yml`（PR 检查）、`tests.yml`（测试）、`tag-notification.yml`（标签通知）
- `.golangci.yaml` 配置详尽：启用 `revive`（代码规范）、`godoclint`（文档检查）、`funlen`（函数长度 ≤200 行）、`cyclop`（圈复杂度 ≤40）
- `.licenserc.yaml` 统一 Apache 2.0 license header
- 完整的 `CONTRIBUTING.md`、`CODE_OF_CONDUCT.md`、`PULL_REQUEST_TEMPLATE.md`
- 使用 `go.uber.org/mock` 自动生成 6 个 mock 文件（Agent、ChatModel、Retriever、Embedding、Indexer、Document）
- Go 1.18 起步，兼容性广泛
- 合理使用成熟依赖：`sonic`（高性能 JSON）、`testify`（断言）、`google/uuid`、`gonja`（模板）

**差距分析**：InferGlow 在工程基础设施上几乎为空白。缺少 CI/CD 意味着每次提交无法自动验证；缺少 linting 意味着代码风格和质量无强制约束；缺少 license header 在开源场景下有法律风险；无 mock 生成工具链导致测试可维护性差。

**改进建议**：
1. 新增 `.github/workflows/ci.yml`，至少包含 `go test ./...` 和 `golangci-lint run`
2. 新增 `.golangci.yaml`，启用 `revive`、`gofmt`、`go vet` 基础规则
3. 统一添加 license header（MIT 或 Apache 2.0）
4. 引入 `go.uber.org/mock` 或 `matryer/moq` 生成 mock，替代手写
5. 将 Go 版本要求降至 1.21（泛型稳定版本），扩大兼容性

---

### 维度 2：测试体系

**InferGlow 现状**：
- 180 个测试文件 / 159 个源码文件，测试/源码比 1.13（高于 Eino）
- 7 个 benchmark 文件（model、flow、audit、schema、session、orchestrator 模块）
- 27 个 bugfix 测试文件，表明开发过程中修复了大量 bug 并添加了回归测试
- 测试类型丰富：单元测试、集成测试、benchmark、bugfix 回归测试
- 但缺少 generic 测试（未使用泛型）、缺少自动生成的 mock

**Eino 现状**：
- 136 个测试文件 / 202 个源码文件，测试/源码比 0.67
- 0 个 benchmark 文件（无性能基线）
- 4 个 generic 测试文件（`*_generic_test.go`），验证泛型接口的正确性
- 6 个自动生成的 mock 文件，支持依赖注入测试
- 测试覆盖类型：单元测试、集成测试、generic 测试、mock 驱动测试
- 有 `checkpoint_compat_resume_test.go` 等跨版本兼容性测试

**差距分析**：InferGlow 在测试数量和类型多样性上反而优于 Eino（更多测试、有 benchmark），但 27 个 bugfix 测试暗示代码质量在迭代中出现了较多问题。Eino 虽然测试文件少，但有 mock 工具链支撑，测试可维护性更高。InferGlow 的优势是建立了性能基线，这是 Eino 缺失的。

**改进建议**：
1. 引入 mock 生成工具链，提升测试可维护性
2. 将 bugfix 测试中的通用模式提炼为常规测试，减少"打补丁"式测试
3. 增加 generic 测试（如果引入泛型后）
4. 保持 benchmark 优势，逐步建立性能回归 CI 门控

---

### 维度 3：组件生态

**InferGlow 现状**：
- 仅有 `model` 组件抽象（`ModelRequester` 接口、Provider 抽象）
- 无 Retriever、Embedding、Indexer、Prompt、Document 组件抽象
- 无法支持 RAG（检索增强生成）场景
- 无法支持向量数据库集成
- 无 Prompt 模板抽象

**Eino 现状**：
- `components/` 目录下有 7 类组件抽象：
  - `model/`：`BaseModel[M]`、`ChatModel`、`ToolCallingChatModel`、`AgenticModel`
  - `tool/`：`BaseTool`、`InvokableTool`、`StreamableTool` + `utils/`（InvokableFunc、StreamableFunc）
  - `retriever/`：`Retriever` 接口（向量检索）
  - `embedding/`：`Embedder` 接口（文本向量化）
  - `indexer/`：`Indexer` 接口（索引管理）
  - `prompt/`：`ChatTemplate` 接口（模板渲染）
  - `document/`：`Parser`、`Transformer` 接口（文档处理）
- 每类组件都有 `callback_extra.go`（回调扩展）、`option.go`（选项模式）
- 外部仓库 `eino-ext` 提供 OpenAI、Ollama、Elasticsearch 等具体实现

**差距分析**：这是 InferGlow 最大的短板之一。缺少 6 类组件抽象意味着无法支持 RAG、向量检索、文档处理等核心 LLM 应用场景。Eino 的组件生态是生产级 LLM 框架的基础设施，InferGlow 仅有 model 层，相当于只有"嘴巴"没有"眼睛和耳朵"。

**改进建议**：
1. **P0**：新增 `components/tool/` 抽象，统一工具接口（当前 action 模块与 model 模块的 ToolDefinition 割裂）
2. **P0**：新增 `components/prompt/` 抽象，支持模板渲染（当前 systemPrompt 是纯字符串）
3. **P1**：新增 `components/retriever/` 和 `components/embedding/`，支持 RAG 场景
4. **P1**：新增 `components/document/`，支持文档解析
5. **P2**：新增 `components/indexer/`，支持向量数据库集成

---

### 维度 4：Agent 模式

**InferGlow 现状**：
- 仅有 1 种 Agent 实现（`orchestrator/agent/agent.go`）
- Agent 通过 `Engine.executeLoop` 实现 PLAN → EXECUTE 循环
- 支持 `WithMaxRounds`、`WithSystemPrompt`、`WithStreamTimeout`、`WithPIIMasker` 等选项
- 有 `LoopGuard` 防止无限循环、`RateLimitHook` 限流、`SecurityHook` 安全检查
- 无预置 Agent 模式，用户需自行组装

**Eino 现状**：
- 4 种预置 Agent 模式：
  1. **ChatModelAgent**（`adk/chatmodel.go`）：基础 ReAct Agent，自动处理工具调用循环
  2. **DeepAgent**（`adk/prebuilt/deep/`）：深度任务编排，支持子 Agent 委派、shell 执行、文件系统操作、TODO 管理
  3. **Supervisor**（`adk/prebuilt/supervisor/`）：监督者模式，决定何时委派任务给子 Agent、何时直接处理
  4. **PlanExecute**（`adk/prebuilt/planexecute/`）：结构化计划执行，比 DeepAgent 控制更强但灵活性更低
- 所有 Agent 都支持中断/恢复、流式输出、回调注入
- 有 `AgentTool` 可将 Agent 包装为工具供其他 Agent 调用
- 有 `TransferToAgentAction` 支持 Agent 间转移（标注为 NOT RECOMMENDED）

**差距分析**：InferGlow 的单一 Agent 模式只能处理简单的"用户输入 → LLM → 工具调用 → 结果"场景。Eino 的 4 种预置模式覆盖了从简单对话到复杂多步推理、多 Agent 协作的全谱系场景。DeepAgent 的 TODO 管理、shell 执行、文件系统操作是生产级 Agent 的刚需。

**改进建议**：
1. **P0**：实现 `ChatModelAgent` 等价物——一个开箱即用的 ReAct Agent，用户无需手动组装 Engine
2. **P1**：实现 `DeepAgent` 等价物——支持子 Agent 委派、TODO 管理、文件系统操作
3. **P2**：实现 `Supervisor` 和 `PlanExecute` 模式
4. 将现有 `Agent` 重构为 `BasicAgent`，作为其他模式的基础

---

### 维度 5：编排能力

**InferGlow 现状**：
- `flow/` 模块有 16 个源码文件
- 两层流引擎：线性 Flow（简单管道）+ TriggerFlow（事件驱动）
- 13 种算子：chunk、signal_gate、batch_fanout、for_each、match_case 等
- 支持 Pause/Resume、Subflow、GoroutinePool
- 有生命周期管理（LifecycleMachine）、信号网络（SignalNet）
- 有持久化能力（`persistence.go`）

**Eino 现状**：
- `compose/` 模块有 35 个源码文件（是 InferGlow 的 2 倍）
- 编排原语更丰富：
  - `Graph`：通用图（支持 Pregel 和 DAG 两种运行模式）
  - `Chain`：顺序管道
  - `Branch`：条件分支
  - `ChainParallel`：并行执行
  - `DAG`：有向无环图
  - `Workflow`：工作流
  - `ToolNode`：工具节点
  - `AgenticToolsNode`：Agent 工具节点
- 支持状态管理（`state.go`）、字段映射（`field_mapping.go`）、值合并（`values_merge.go`）
- 支持流式拼接（`stream_concat.go`）、流式读取（`stream_reader.go`）
- 支持 Checkpoint 持久化（`checkpoint.go`）、中断恢复（`resume.go`、`interrupt.go`）
- 支持图管理器（`graph_manager.go`）动态修改图
- 支持内省（`introspect.go`）

**差距分析**：InferGlow 的 Flow 引擎在设计上不逊色（TriggerFlow 的事件驱动 + 13 算子是亮点），但 Eino 的 compose 模块在功能广度上更胜一筹（Graph 的 Pregel/DAG 双模式、状态管理、字段映射、动态图修改）。InferGlow 的优势是 Pause/Signal 机制，但缺少 Checkpoint 持久化。

**改进建议**：
1. **P1**：为 Flow 引擎增加 Checkpoint 持久化能力（当前 Pause 是内存态）
2. **P1**：增加状态管理（State）和字段映射能力
3. **P2**：支持图的动态修改（运行时增删节点）
4. 保持 TriggerFlow 的事件驱动优势，这是 Eino 没有的差异化能力

---

### 维度 6：流式处理

**InferGlow 现状**：
- `model/streaming_json.go` 处理 SSE 流式 JSON
- `StreamChunk` 结构包含 Delta、Reasoning、ToolCalls、Usage 字段
- 流式处理与 Provider 耦合（每个 Provider 自己解析 SSE）
- 无通用 StreamReader 抽象
- 无流式拼接、流式复制、流式合并能力

**Eino 现状**：
- `schema/stream.go`（1024 行）提供完整的 `StreamReader[T]` 泛型抽象
- 核心方法：`Recv()`（读取）、`Close()`（关闭）、`Copy()`（复制）
- 高级功能：`MergeStreamReaders`（合并多流）、`StreamReaderFromArray`（数组转流）、`StreamReaderFromArrayWithChunkSize`（分块）
- `compose/stream_concat.go`：流式拼接
- `compose/stream_reader.go`：图组件间的流式路由
- 流式处理贯穿整个编排链路：组件只需实现自己的流式范式，框架自动处理拼接、装箱、合并、复制

**差距分析**：InferGlow 的流式处理是"点对点"的（Provider 直接产 StreamChunk），缺少通用抽象。Eino 的 `StreamReader[T]` 是生产级流式处理的标杆，支持多消费者、流复制、流合并等复杂场景。这在多 Agent 协作、并行编排中是刚需。

**改进建议**：
1. **P0**：实现通用 `StreamReader[T]` 泛型抽象，解耦 Provider 与消费方
2. **P1**：实现流复制（`Copy`）和流合并（`Merge`）能力
3. **P1**：让流式处理贯穿 flow 编排链路，组件间自动传递流

---

### 维度 7：可观测性

**InferGlow 现状**：
- `observability/otel/` 模块已实现 OpenTelemetry 集成
- 有 4 类 Span：`AgentSpan`、`LLMInteractionSpan`、`ToolCallSpan`、`FlowExecutionSpan`
- 有导出器配置（`exporters.go`，支持 Jaeger/OTLP）
- 有语义属性（model_name、prompt_tokens 等）
- 但缺少回调切面机制（无法在固定切点注入逻辑）

**Eino 现状**：
- `callbacks/` 模块提供完整的切面机制：
  - 5 个切点：`OnStart`、`OnEnd`、`OnError`、`OnStartWithStreamInput`、`OnEndWithStreamOutput`
  - `HandlerBuilder` 支持按需注册切点函数（无需实现全部接口）
  - `AspectInject` 提供切面注入工具
  - 支持 `TimingChecker` 优化性能（避免无回调时的开销）
- 切面机制贯穿组件、图、Agent 全链路
- 外部 `eino-ext` 提供 OTel、Langfuse 等具体回调实现

**差距分析**：InferGlow 有 OTel 集成（这是优势），但缺少通用的回调切面机制。Eino 的 callbacks 是"框架级"的可观测性，用户可以在任何切点注入日志、指标、追踪逻辑。InferGlow 的 OTel 是"硬编码"的，灵活性低。

**改进建议**：
1. **P1**：实现通用回调切面机制（OnStart/OnEnd/OnError 等切点）
2. **P1**：提供 `HandlerBuilder` 等价的便捷构建工具
3. **P2**：将 OTel 集成重构为回调切面的一个实现，而非硬编码
4. 保持 OTel 语义属性的标准化优势

---

### 维度 8：中断/恢复

**InferGlow 现状**：
- `flow/pause.go`：基于 channel 的 Pause/Resume 机制
- `flow/signal.go`：信号网络（SignalNet）用于外部中断
- 有 `pause_bugfix_test.go`、`pause_regression_test.go`、`signal_bugfix_test.go` 等回归测试
- **无持久化**：Pause 状态仅在内存中，进程崩溃后无法恢复
- 无 Checkpoint 机制

**Eino 现状**：
- `compose/checkpoint.go`（300+ 行）：完整的 Checkpoint 持久化机制
  - 支持图状态、输入、流数据的序列化
  - 支持中断点（Interrupt ID）定位
  - 支持跨版本迁移（`checkpoint_migrate_test.go`）
- `compose/resume.go`：从 Checkpoint 恢复执行
  - 检索中断状态、准备恢复上下文、定位执行点
- `compose/interrupt.go`：中断管理
- `adk/interrupt.go`：Agent 级中断，支持 human-in-the-loop
- 有跨版本兼容性测试（`checkpoint_compat_resume_test.go`，验证 v0.7.37 到 v0.8.4 的 Checkpoint 兼容性）

**差距分析**：InferGlow 的 Pause/Resume 是内存态的，无法用于生产环境的长时间运行任务。Eino 的 Checkpoint 机制支持持久化、跨版本兼容、human-in-the-loop，是生产级 Agent 框架的必备能力。

**改进建议**：
1. **P0**：为 Flow 引擎实现 Checkpoint 持久化（序列化图状态到文件/数据库）
2. **P0**：实现从 Checkpoint 恢复执行的能力
3. **P1**：支持 Agent 级中断（human-in-the-loop 场景）
4. **P2**：建立跨版本 Checkpoint 兼容性测试

---

### 维度 9：多 Agent 协作

**InferGlow 现状**：
- **完全无多 Agent 协作能力**
- 单一 Agent 无法委派任务给其他 Agent
- 无 Host/Supervisor 模式
- 无 Agent 间通信机制

**Eino 现状**：
- `adk/prebuilt/supervisor/`：Supervisor 模式，监督者决定任务委派
- `adk/prebuilt/deep/`：DeepAgent 支持子 Agent 委派
- `flow/agent/multiagent/host/`：Host 模式多 Agent 编排
- `adk/agent_tool.go`：可将 Agent 包装为工具供其他 Agent 调用
- `TransferToAgentAction`：Agent 间转移（标注 NOT RECOMMENDED，推荐用 AgentTool 替代）
- `deterministic_transfer.go`：确定性转移（不依赖 LLM 决策）

**差距分析**：多 Agent 协作是复杂 Agent 应用的核心能力（如"研究 Agent + 编码 Agent + 审查 Agent"协作）。InferGlow 完全缺失这一能力，限制了其在复杂业务场景的适用性。

**改进建议**：
1. **P1**：实现 `AgentTool`，允许将 Agent 包装为工具
2. **P1**：实现 Supervisor 模式，支持任务委派
3. **P2**：实现 Host 模式多 Agent 编排
4. **P2**：支持确定性转移（不依赖 LLM 的硬编码委派）

---

### 维度 10：类型安全与泛型

**InferGlow 现状**：
- **未使用 Go 泛型**
- `ModelRequester` 接口是非泛型的
- 消息类型是固定结构体（`ChatMessage`、`ContentBlock`）
- 无类型参数化的组件接口
- Schema 模块使用 `interface{}` 和反射

**Eino 现状**：
- 大量使用 Go 泛型：
  - `BaseModel[M messageType]`：泛型模型接口（M 约束为 `*schema.Message | *schema.AgenticMessage`）
  - `TypedAgent[M MessageType]`：泛型 Agent 接口
  - `TypedAgentEvent[M]`、`TypedAgentInput[M]`、`TypedAgentOutput[M]`：泛型事件类型
  - `StreamReader[T]`：泛型流读取器
  - `TypedConfig[M]`：泛型配置（如 DeepAgent 配置）
- `internal/generic/`：泛型工具函数
- 类型约束通过 `messageType` interface 实现 sealed type 效果
- 有 `*_generic_test.go` 验证泛型接口正确性

**差距分析**：InferGlow 未使用泛型，导致类型安全性低、代码重复高。Eino 的泛型设计使得 Message 和 AgenticMessage 可以共用同一套接口，同时保持类型安全。这是现代 Go 框架的基本要求。

**改进建议**：
1. **P1**：引入泛型重构 `ModelRequester` 为 `ModelRequester[M]`
2. **P1**：实现 `StreamReader[T]` 泛型抽象
3. **P2**：将 Agent 接口泛型化为 `TypedAgent[M]`
4. 使用类型约束实现 sealed type 效果，防止外部扩展

---

## 四、功能实现深度对比

> 以下从功能点级别（而非"有/无"）对比两项目的实现差异。

### 4.1 流式处理功能

| 功能点 | InferGlow | Eino | 差距 |
|--------|-----------|------|------|
| 增量 JSON 解析 | ✅ `StreamingJSONParser` 支持 token 边界、不完整 JSON、事件驱动（8 种事件类型） | ❌ 无对应实现 | InferGlow 优势 |
| 通用流抽象 | ❌ 无，`StreamChunk` 是固定结构体 | ✅ `StreamReader[T]` 泛型抽象，支持任意类型 | Eino 优势 |
| 流管道（Pipe） | ❌ 无 | ✅ `Pipe[T](cap)` 创建读写双端，支持背压（带缓冲） | Eino 优势 |
| 流过滤 | ❌ 无 | ✅ `ErrNoValue` 哨兵值，`StreamReaderWithConvert` 跳过元素 | Eino 优势 |
| 流转换 | ❌ 无 | ✅ `StreamReaderWithConvert` 函数式转换 | Eino 优势 |
| 流复制 | ❌ 无（单消费者） | ✅ `Copy()` 支持多消费者 | Eino 优势 |
| 流合并 | ❌ 无 | ✅ `MergeStreamReaders`、`MergeNamedStreamReaders` | Eino 优势 |
| 数组转流 | ❌ 无 | ✅ `StreamReaderFromArray`、`StreamReaderFromArrayWithChunkSize` | Eino 优势 |
| 消息拼接 | ❌ 无 | ✅ `ConcatMessages`、`ConcatAgenticMessages` | Eino 优势 |
| SourceEOF 追踪 | ❌ 无 | ✅ `SourceEOF` 错误类型标识来源流 | Eino 优势 |
| 流在编排中传递 | ❌ Provider 直产 StreamChunk，不贯穿 flow | ✅ 组件间自动拼接、装箱、合并、复制 | Eino 优势 |

**核心差距**：InferGlow 的 `StreamingJSONParser` 是一个**独特亮点**（Eino 没有），但通用流式抽象完全缺失。Eino 的 `StreamReader[T]` 支持 10+ 种流操作，是生产级流式处理的标杆。InferGlow 的流是"点对点"的，无法支持多消费者、流复制、流合并等复杂场景。

---

### 4.2 Agent 循环控制功能

| 功能点 | InferGlow | Eino | 差距 |
|--------|-----------|------|------|
| PLAN → EXECUTE 循环 | ✅ `executeLoop` 基础循环 | ✅ ChatModelAgent 内部 ReAct 循环 | 持平 |
| 最大轮数控制 | ✅ `WithMaxRounds`（默认 10） | ✅ `MaxIteration` 配置 | 持平 |
| 流超时 | ✅ `WithStreamTimeout`（默认 5 分钟） | ❌ 无显式流超时 | InferGlow 优势 |
| 循环终止保护 | ✅ `LoopGuard` 检测重复决策 | ❌ 无对应 | InferGlow 优势 |
| 运行中抢占（Preempt） | ❌ 无 | ✅ `TurnLoop` 的 `preemptTurnPhase` 三态状态机（idle/planning/active） | Eino 优势 |
| 运行中取消（Cancel） | ❌ 无（仅靠 ctx.Done） | ✅ `CancelMode` 三种模式：`CancelImmediate`/`CancelAfterChatModel`/`CancelAfterToolCalls`，支持位运算组合 | Eino 优势 |
| 取消递归传播 | ❌ 无 | ✅ `WithRecursive` 传播到子 Agent | Eino 优势 |
| 取消等待 | ❌ 无 | ✅ `CancelHandle.Wait()` 阻塞等待取消结果 | Eino 优势 |
| 取消超时升级 | ❌ 无 | ✅ 超时后从安全点取消升级为立即取消 | Eino 优势 |
| 工具调用后钩子 | ❌ 无 | ✅ `WithAfterToolCallsHook` 在下一轮 LLM 前注入逻辑 | Eino 优势 |
| 历史修改 | ❌ 无 | ✅ `WithHistoryModifier` resume 时修改历史 | Eino 优势 |

**核心差距**：InferGlow 的循环控制是"静态"的（启动后无法干预），Eino 支持**运行中抢占和取消**，这是生产级 Agent 的刚需。例如：用户在 Agent 思考时说"停下，换个方向"，Eino 可以抢占当前轮次，InferGlow 只能等当前轮结束。Eino 的 `CancelAfterToolCalls`（工具执行完再取消）保证了状态一致性。

---

### 4.3 中断/恢复功能

| 功能点 | InferGlow | Eino | 差距 |
|--------|-----------|------|------|
| Pause 记录状态 | ✅ `PausePoint`（StepName、Input、Timestamp） | ✅ Checkpoint（图状态、输入、流数据） | Eino 更全面 |
| Resume 从下一步恢复 | ✅ `Flow.Resume(pp, resumeInput)` | ✅ `compose.Resume(info)` | 持平 |
| 持久化存储 | ❌ 仅内存态 | ✅ `CheckPointStore` 接口，支持任意后端 | Eino 优势 |
| 序列化器 | ❌ 无 | ✅ `Serializer` 接口（Marshal/Unmarshal），可自定义 | Eino 优势 |
| Checkpoint ID 定位 | ❌ 无 | ✅ `WithCheckPointID` 加载/写入指定 ID | Eino 优势 |
| 写入不同 ID | ❌ 无 | ✅ `WithWriteToCheckPointID` 从旧 ID 读、写新 ID（版本化） | Eino 优势 |
| 强制重跑 | ❌ 无 | ✅ `WithForceNewRun` 忽略 Checkpoint 从头跑 | Eino 优势 |
| 状态修改器 | ❌ 无 | ✅ `WithStateModifier` 在 checkpoint 读写时修改状态 | Eino 优势 |
| 跨版本迁移 | ❌ 无 | ✅ `checkpoint_migrate_test.go` + v0.7.37→v0.8.4 兼容测试 | Eino 优势 |
| Agent 级中断（HITL） | ❌ 无 | ✅ `adk/interrupt.go` + `ResumeInfo` + `CompositeInterrupt` | Eino 优势 |
| 流数据持久化 | ❌ 无 | ✅ Checkpoint 支持流数据的序列化与恢复 | Eino 优势 |
| Context 取消处理 | ✅ Resume 时检查 `ctx.Done()` | ✅ 完整的 ctx 取消处理 | 持平 |

**核心差距**：InferGlow 的 Pause/Resume 是**内存态的**，进程崩溃即丢失。Eino 的 Checkpoint 机制支持持久化、版本化、状态修改、强制重跑、HITL，是生产级长时间运行任务的必备能力。

---

### 4.4 工具调用功能

| 功能点 | InferGlow | Eino | 差距 |
|--------|-----------|------|------|
| 工具元数据 | ✅ `Action.Name`/`Description`/`Schema`（map[string]any） | ✅ `ToolInfo`（Name/Desc/Params/ToolID） | 持平 |
| 工具执行 | ✅ `ActionExecutor.Execute(ctx, map[string]any)` | ✅ `InvokableTool.InvokableRun(ctx, json string)` | 接口差异 |
| 流式工具 | ❌ 无 | ✅ `StreamableTool.StreamableRun` 返回 `StreamReader[string]` | Eino 优势 |
| 多模态工具结果 | ❌ 无 | ✅ `EnhancedInvokableTool` 返回 `ToolResult`（文本/图片/音频/视频/文件） | Eino 优势 |
| 流式多模态工具 | ❌ 无 | ✅ `EnhancedStreamableTool` 流式返回 `ToolResult` | Eino 优势 |
| 结构化工具参数 | ❌ `map[string]any` 松散 | ✅ `ToolArgument` 结构化参数 | Eino 优势 |
| 函数式工具包装 | ✅ `LocalFunctionExecutor` 三种签名自动包装 | ✅ `utils.InferTool`/`utils.NewTool` + `InvokableFunc`/`StreamableFunc` | 持平 |
| 工具标签 | ✅ `Tags []string` | ❌ 无 | InferGlow 优势 |
| 工具元数据 | ✅ `Metadata map[string]any` | ❌ 无 | InferGlow 优势 |
| 工具选项模式 | ❌ 无 | ✅ `tool.Option` 选项模式 | Eino 优势 |

**核心差距**：InferGlow 的工具是"同步阻塞"的，返回 `map[string]any`。Eino 支持**流式工具**和**多模态工具结果**（图片/音频/视频/文件），这是多模态 Agent 场景的刚需。

---

### 4.5 中间件功能

| 功能点 | InferGlow | Eino | 差距 |
|--------|-----------|------|------|
| 中间件系统 | ❌ 完全无 | ✅ 8 个中间件 | Eino 优势 |
| 动态工具发现 | ❌ 无 | ✅ `dynamictool/toolsearch` 运行时搜索工具 | Eino 优势 |
| 文件系统操作 | ❌ 无 | ✅ `filesystem` 提供 read/write/edit/glob/grep 工具 | Eino 优势 |
| 工具调用补丁 | ❌ 无 | ✅ `patchtoolcalls` 修改/覆盖工具调用行为 | Eino 优势 |
| 计划任务管理 | ❌ 无 | ✅ `plantask` TODO 列表 + 持久化 + 历史 | Eino 优势 |
| 工具结果精简 | ❌ 无 | ✅ `reduction` 大结果压缩/摘要 | Eino 优势 |
| 技能管理 | ❌ 无 | ✅ `skill` 技能作为工具注册 | Eino 优势 |
| 对话摘要 | ❌ 无 | ✅ `summarization` 长对话自动摘要 | Eino 优势 |
| Agent 元数据 | ❌ 无 | ✅ `agentsmd` Agent 描述增强 | Eino 优势 |
| 大工具结果处理 | ❌ 无 | ✅ `reduction/internal` 清理/压缩大结果 | Eino 优势 |

**核心差距**：InferGlow 完全没有中间件系统。Eino 的 8 个中间件覆盖了**动态工具发现、文件系统操作、工具结果精简、计划任务、技能管理、对话摘要**等生产级场景。这些中间件是 DeepAgent 等 Agent 模式的基础设施。

---

### 4.6 Agent 高级功能

| 功能点 | InferGlow | Eino | 差距 |
|--------|-----------|------|------|
| 失败自动重试 | ❌ 无（依赖 `model/attempt.go` 的 AttemptRunner） | ✅ `retry_chatmodel.go` 包装 ChatModel，指数退避 + 随机抖动 | Eino 优势 |
| 模型故障转移 | ❌ 无 | ✅ `failover_chatmodel.go` 多模型自动切换 | Eino 优势 |
| 重试错误分类 | ✅ `attempt.go` 区分可重试/不可重试错误 | ✅ `RetryExhaustedError` 包装最后错误 | 持平 |
| 确定性转移 | ❌ 无 | ✅ `deterministic_transfer.go` 不依赖 LLM 的硬编码委派 | Eino 优势 |
| 流式输出 | ✅ `streaming.go` Agent 流式输出 | ✅ `MessageStream` 流式事件 | 持平 |
| 取消监控 | ❌ 仅靠 ctx | ✅ `cancel.go` 三种 CancelMode + 递归传播 | Eino 优势 |
| 子 Agent 委派 | ❌ 无 | ✅ `AgentTool` 包装 Agent 为工具 + `SubAgents` 配置 | Eino 优势 |
| Agent 转移 | ❌ 无 | ✅ `TransferToAgentAction`（标注 NOT RECOMMENDED） | Eino 优势 |
| 审计集成 | ✅ `AuditHook` + `NewEngineWithAudit` | ❌ 核心无（外部 eino-ext） | InferGlow 优势 |
| 循环保护 | ✅ `LoopGuard` 检测重复决策 | ❌ 无 | InferGlow 优势 |
| PII 脱敏 | ✅ `WithPIIMasker` 输入/输出双向 | ❌ 核心无 | InferGlow 优势 |
| 提示注入检测 | ✅ `SecurityHook` | ❌ 核心无 | InferGlow 优势 |
| 速率限制 | ✅ `RateLimitHook` | ❌ 核心无 | InferGlow 优势 |

**核心差距**：InferGlow 在**安全侧**（审计/PII/注入/限流）有显著优势，但缺少**可靠性侧**（重试/故障转移/确定性转移）。Eino 的 `retry_chatmodel.go` + `failover_chatmodel.go` 是生产级 Agent 的容错标配。

---

### 4.7 Schema 功能

| 功能点 | InferGlow | Eino | 差距 |
|--------|-----------|------|------|
| 输出 Schema 约束 | ✅ `OutputSchema` + `FieldDef` + `DataType` | ❌ 无独立 Schema 引擎 | InferGlow 优势 |
| 契约引擎 | ✅ `ContractEngine` 运行时校验 | ❌ 无 | InferGlow 优势 |
| JSON Schema 转换 | ✅ `jsonschema.go` Go 类型 → JSON Schema | ✅ 通过 `eino-contrib/jsonschema` | 持平 |
| 路径校验 | ✅ `path.go` | ❌ 无 | InferGlow 优势 |
| Blueprint 序列化 | ✅ `blueprint.go` Flow 可序列化 | ❌ 无 | InferGlow 优势 |
| 类型适配器 | ✅ `type_adapter.go` | ❌ 无 | InferGlow 优势 |
| 消息多模态 | ❌ `ChatMessage` 仅文本 | ✅ `Message` + `ContentBlock`（文本/图片/音频/视频/文件） | Eino 优势 |
| AgenticMessage | ❌ 无 | ✅ `AgenticMessage` + `AgenticRoleType` | Eino 优势 |
| 消息拼接 | ❌ 无 | ✅ `ConcatMessages`/`ConcatAgenticMessages` | Eino 优势 |
| 消息解析器 | ❌ 无 | ✅ `message_parser.go` | Eino 优势 |
| ToolInfo | ❌ `ToolDefinition` 简单 | ✅ `ToolInfo` + `ToolID` + 多模态支持 | Eino 优势 |
| Document 抽象 | ❌ 无 | ✅ `document.go` + `Parser`/`Transformer` | Eino 优势 |
| 流式 Schema | ❌ 无 | ✅ `stream.go` `StreamReader[T]` | Eino 优势 |
| 版本注册 | ❌ 无 | ✅ `schema.RegisterName[T]` 支持序列化版本 | Eino 优势 |

**核心差距**：两项目侧重点不同。InferGlow 的 Schema 是**输出契约约束**（LLM 输出格式校验），Eino 的 Schema 是**消息与数据模型**（多模态/流式/文档）。InferGlow 在输出约束上更成熟，但**完全缺少多模态支持**。

---

### 4.8 编排功能

| 功能点 | InferGlow | Eino | 差距 |
|--------|-----------|------|------|
| 线性 Flow | ✅ `Flow` 简单管道 | ✅ `Chain` 顺序执行 | 持平 |
| 事件驱动 Flow | ✅ `TriggerFlow` + `SignalNet` + 13 算子 | ❌ 无对应（Graph 是数据驱动） | InferGlow 优势 |
| 通用图 | ❌ 无 | ✅ `Graph` 支持 Pregel + DAG 双模式 | Eino 优势 |
| 条件分支 | ✅ `OpMatchRoute`/`OpMatchCase` | ✅ `Branch` | 持平 |
| 并行执行 | ✅ `OpBatchFanout`/`OpForEachSplit` | ✅ `ChainParallel`/`DAG` | 持平 |
| 子流程 | ✅ `OpSubFlow` | ✅ 嵌套 Graph | 持平 |
| 信号门控 | ✅ `OpSignalGate` | ❌ 无对应 | InferGlow 优势 |
| 批量收集 | ✅ `OpBatchCollect`/`OpMatchCollect` | ✅ `values_merge.go` | 持平 |
| 干预点 | ✅ `OpIntervention` | ✅ `interrupt.go` | 持平 |
| 结果汇聚 | ✅ `OpResultSink` | ✅ `END` 节点 | 持平 |
| 状态管理 | ❌ 无 | ✅ `state.go` 图级状态 | Eino 优势 |
| 字段映射 | ❌ 无 | ✅ `field_mapping.go` 节点间字段映射 | Eino 优势 |
| 值合并 | ❌ 无 | ✅ `values_merge.go` 多输入合并策略 | Eino 优势 |
| 动态图修改 | ❌ 无 | ✅ `graph_manager.go` 运行时增删节点 | Eino 优势 |
| 图内省 | ❌ 无 | ✅ `introspect.go` | Eino 优势 |
| Lambda 节点 | ❌ 无 | ✅ `types_lambda.go` 任意函数作为节点 | Eino 优势 |
| Checkpoint | ❌ 无 | ✅ `checkpoint.go` 图级持久化 | Eino 优势 |
| Workflow 模式 | ❌ 无 | ✅ `workflow.go` 高级工作流 | Eino 优势 |
| 组件到图节点 | ❌ 无 | ✅ `component_to_graph_node.go` | Eino 优势 |
| 工具节点 | ❌ 无 | ✅ `tool_node.go`/`agentic_tools_node.go` | Eino 优势 |
| Goroutine 池 | ✅ `goroutine_pool.go` | ❌ 无（依赖 Pregel 调度） | InferGlow 优势 |
| 生命周期管理 | ✅ `lifecycle.go` | ✅ Graph 内置 | 持平 |
| Blueprint 序列化 | ✅ `triggerflow_blueprint.go` | ❌ 无 | InferGlow 优势 |

**核心差距**：InferGlow 的 TriggerFlow **事件驱动 + 13 算子**是差异化亮点（Eino 的 Graph 是数据驱动）。但 Eino 在**状态管理、字段映射、动态图修改、Lambda 节点、工具节点、组件集成**上更全面。InferGlow 的 Flow 是独立的编排引擎，Eino 的 compose 是**与组件生态深度集成**的编排引擎。

---

## 五、成熟度评分矩阵

| 维度 | InferGlow | Eino | 差距 | 说明 |
|------|-----------|------|------|------|
| 1. 工程基础设施 | 1 | 5 | -4 | InferGlow 无 CI/linting/license/mock 工具链；Eino 全套齐备 |
| 2. 测试体系 | 4 | 3 | +1 | InferGlow 测试更多且有 benchmark；Eino 有 mock 工具链但无 bench |
| 3. 组件生态 | 1 | 5 | -4 | InferGlow 仅 model；Eino 有 7 类组件 + 外部 eino-ext |
| 4. Agent 模式 | 2 | 5 | -3 | InferGlow 单一 Agent；Eino 有 4 种预置模式 |
| 5. 编排能力 | 3 | 5 | -2 | InferGlow 有 TriggerFlow 亮点；Eino 功能更广（35 vs 16 源码文件） |
| 6. 流式处理 | 2 | 5 | -3 | InferGlow 无通用抽象；Eino 有 StreamReader[T] 全链路流式 |
| 7. 可观测性 | 3 | 4 | -1 | InferGlow 有 OTel；Eino 有通用回调切面 + OTel（外部） |
| 8. 中断/恢复 | 2 | 5 | -3 | InferGlow 仅内存态 Pause；Eino 有 Checkpoint 持久化 + 跨版本兼容 |
| 9. 多 Agent 协作 | 1 | 5 | -4 | InferGlow 完全缺失；Eino 有 Supervisor/Host/AgentTool |
| 10. 类型安全 | 1 | 5 | -4 | InferGlow 未用泛型；Eino 大量泛型 + sealed type |
| **总分** | **20** | **47** | **-27** | 满分 50 |
| **平均分** | **2.0** | **4.7** | **-2.7** | — |

**差距最大的 Top 3 维度**：
1. **工程基础设施**（-4）：无 CI/CD、无 linting、无 license、无 mock 工具链
2. **组件生态**（-4）：仅 model 抽象，缺 6 类核心组件
3. **多 Agent 协作**（-4）：完全缺失，无任何多 Agent 模式

**InferGlow 唯一优势维度**：
- **测试体系**（+1）：测试文件更多、有 benchmark 基线、有 bugfix 回归测试

---

## 六、InferGlow 具体不足清单（15 项，含功能层）

| # | 不足项 | 对应 Eino 成熟实践 | 优先级 |
|---|--------|-------------------|--------|
| 1 | 无 CI/CD 配置 | `.github/workflows/` 下 3 个 CI 配置 | P0 |
| 2 | 无 linting 配置 | `.golangci.yaml` 含 revive/godoclint/funlen/cyclop | P0 |
| 3 | 组件生态不完整（仅 model） | 7 类组件抽象（model/tool/retriever/embedding/indexer/prompt/document） | P0 |
| 4 | 无预置 Agent 模式 | 4 种预置（ChatModelAgent/DeepAgent/Supervisor/PlanExecute） | P0 |
| 5 | 无 Checkpoint 持久化 | 完整 Checkpoint 机制 + 跨版本兼容测试 | P0 |
| 6 | 无通用流式抽象 | `StreamReader[T]` 泛型 + 流复制/合并/拼接 | P0 |
| 7 | 无运行中抢占/取消 | `TurnLoop` Preempt + `CancelMode` 三模式 + 递归传播 | P0 |
| 8 | 无模型故障转移 | `failover_chatmodel.go` 多模型自动切换 | P0 |
| 9 | 无多 Agent 协作 | Supervisor/Host/AgentTool 三种模式 | P1 |
| 10 | 未使用泛型 | `BaseModel[M]`/`TypedAgent[M]`/`StreamReader[T]` | P1 |
| 11 | 无回调切面机制 | 5 切点（OnStart/OnEnd/OnError/...）+ HandlerBuilder | P1 |
| 12 | 无流式工具/多模态工具 | `StreamableTool` + `EnhancedInvokableTool`（图片/音频/视频） | P1 |
| 13 | 无消息多模态 | `Message` + `ContentBlock`（文本/图片/音频/视频/文件） | P1 |
| 14 | 无中间件系统 | 8 个中间件（agentsmd/dynamictool/filesystem/...） | P2 |
| 15 | 无图状态管理/字段映射 | `state.go` + `field_mapping.go` + `values_merge.go` | P2 |

---

## 七、改进建议优先级排序

### P0：严重影响生产可用性（必须立即处理）

| # | 建议 | 具体路径 | 预期效果 |
|---|------|---------|---------|
| 1 | 建立 CI/CD | 新增 `.github/workflows/ci.yml`，包含 `go test`、`golangci-lint`、`go vet` | 每次提交自动验证，防止回归 |
| 2 | 引入 linting | 新增 `.golangci.yaml`，启用 revive/gofmt/govet | 统一代码风格，提升可读性 |
| 3 | 统一 license | 新增 `.licenserc.yaml`，所有文件添加 MIT header | 规避开源法律风险 |
| 4 | 实现通用流式抽象 | 新增 `model/stream_reader.go`，实现 `StreamReader[T]` 泛型 + Pipe/Copy/Merge/Convert | 解耦 Provider 与消费方，支持多消费者、流过滤、流转换 |
| 5 | 实现 Checkpoint 持久化 | 为 Flow 引擎增加 `CheckPointStore` 接口 + 序列化器 + ID 定位 | 支持长时间运行任务的崩溃恢复、版本化、状态修改 |
| 6 | 补齐组件抽象 | 新增 `components/tool/`、`components/prompt/` | 统一工具接口（含流式/多模态），支持模板渲染 |
| 7 | 实现开箱即用 Agent | 新增 `ChatModelAgent` 等价物，封装 Engine 组件 | 用户无需手动组装即可使用 |
| 8 | 实现运行中抢占/取消 | 为 Agent 增加 `TurnLoop` + `CancelMode`（Immediate/AfterChatModel/AfterToolCalls） | 支持运行中干预、安全点取消、递归传播 |
| 9 | 实现模型故障转移 | 新增 `failover_chatmodel.go` 等价物，多模型自动切换 | 提升生产环境可用性 |

### P1：影响开发者体验（近期处理）

| # | 建议 | 具体路径 | 预期效果 |
|---|------|---------|---------|
| 10 | 引入 mock 工具链 | 使用 `go.uber.org/mock`，为核心接口生成 mock | 提升测试可维护性 |
| 11 | 引入泛型 | 将 `ModelRequester` 重构为 `ModelRequester[M]` | 提升类型安全性 |
| 12 | 实现回调切面 | 新增 `callbacks/` 模块，支持 5 个切点 + HandlerBuilder | 支持灵活的可观测性注入 |
| 13 | 实现多 Agent 协作 | 新增 `AgentTool` 和 `Supervisor` 模式 | 支持复杂业务场景 |
| 14 | 补齐 RAG 组件 | 新增 `components/retriever/`、`components/embedding/` | 支持 RAG 场景 |
| 15 | 为 Flow 增加 Checkpoint | 扩展 `flow/persistence.go`，支持中断点定位 | 支持长时间运行任务恢复 |
| 16 | 实现流式工具/多模态工具 | 新增 `StreamableTool` + `EnhancedInvokableTool`（图片/音频/视频） | 支持多模态 Agent 场景 |
| 17 | 实现消息多模态 | 扩展 `ChatMessage` 为 `Message` + `ContentBlock` | 支持多模态对话 |

### P2：远期增强（规划处理）

| # | 建议 | 具体路径 | 预期效果 |
|---|------|---------|---------|
| 18 | 实现中间件系统 | 新增 `middlewares/` 目录，支持插件式扩展 | 支持文件系统、技能、摘要等扩展 |
| 19 | 实现 DeepAgent 等价物 | 新增支持子 Agent 委派、TODO 管理、shell 执行 | 支持复杂多步推理 |
| 20 | 支持动态图修改 | 为 Flow 引擎增加运行时增删节点能力 + 状态管理 + 字段映射 | 支持自适应编排 |
| 21 | 建立跨版本兼容测试 | 为 Checkpoint 建立版本迁移测试 | 保证升级兼容性 |
| 22 | 引入外部组件实现仓库 | 新建 `inferglow-ext` 仓库，提供 OpenAI/Ollama 等实现 | 分离核心与实现 |

---

## 八、总结

### InferGlow 的定位与现状

InferGlow 目前是一个**"零件齐备但缺少组装线和出厂质检"**的基础设施库：
- **零件层**（model/schema/flow/action/session/sandbox）完成度较高（80-100%）
- **组装线**（Agent 模式、多 Agent 协作、中间件系统）严重缺失
- **出厂质检**（CI/CD、linting、license、mock 工具链）完全空白

### Eino 的成熟度标杆

Eino 已经是一个**"可交付的生产线"**：
- 完整的组件生态（7 类抽象 + 外部 eino-ext）
- 4 种预置 Agent 模式覆盖全谱系场景
- 生产级工程基础设施（CI/linting/license/mock）
- 通用流式抽象（StreamReader[T]）贯穿全链路
- Checkpoint 持久化支持长时间运行任务
- 多 Agent 协作能力（Supervisor/Host/AgentTool）
- 泛型驱动的类型安全（BaseModel[M]/TypedAgent[M]）

### InferGlow 的差异化优势（功能层）

InferGlow 在功能层面有 **7 个独特亮点** 是 Eino 核心仓库没有的：

1. **增量 JSON 解析器**：`StreamingJSONParser` 支持 token 边界、不完整 JSON、8 种事件类型（object_start/end、array_start/end、key、value、done、error）—— Eino 无此能力
2. **TriggerFlow 事件驱动编排**：13 种算子 + SignalNet 信号网络 + InterventionPoint 干预点 —— Eino 的 Graph 是数据驱动，无信号门控
3. **循环保护（LoopGuard）**：检测重复决策防止无限循环 —— Eino 无对应
4. **流超时（StreamTimeout）**：单流超时控制（默认 5 分钟）—— Eino 无显式流超时
5. **安全模块**：`security/pii`（脱敏）+ `prompt_injection`（注入检测）+ `ratelimit`（限流）+ `rbac`（访问控制）—— Eino 核心无
6. **沙箱框架**：`sandbox/` 支持 Docker/gVisor/Seatbelt/WindowsAppContainer/Landlock —— Eino 核心无
7. **审计链**：`audit/` 完整审计链路（Append/Sign/VerifyChain/Export/Query）—— Eino 核心无
8. **输出契约引擎**：`schema/ContractEngine` 运行时校验 + Blueprint 序列化 —— Eino 无独立 Schema 引擎
9. **工具标签与元数据**：`Action.Tags` + `Action.Metadata` —— Eino 无

### Eino 的功能优势（InferGlow 缺失）

Eino 在功能层面有 **15 个 InferGlow 完全缺失的能力**：

1. **运行中抢占（Preempt）** + **取消（Cancel）**：三态状态机 + 三种 CancelMode + 递归传播
2. **模型故障转移** + **自动重试**：`failover_chatmodel.go` + `retry_chatmodel.go`
3. **Checkpoint 持久化**：`CheckPointStore` + 序列化器 + ID 定位 + 跨版本迁移
4. **流式工具** + **多模态工具**：`StreamableTool` + `EnhancedInvokableTool`
5. **消息多模态**：`ContentBlock`（文本/图片/音频/视频/文件）
6. **8 个中间件**：动态工具发现、文件系统、计划任务、工具结果精简、技能管理、对话摘要等
7. **4 种预置 Agent**：ChatModelAgent、DeepAgent、Supervisor、PlanExecute
8. **多 Agent 协作**：AgentTool、Host、确定性转移
9. **图状态管理** + **字段映射** + **值合并**
10. **动态图修改** + **图内省**
11. **Lambda 节点** + **工具节点** + **组件到图节点**
12. **通用流式抽象**：`StreamReader[T]` + Pipe/Copy/Merge/Convert/Filter
13. **回调切面**：5 切点 + HandlerBuilder + AspectInject
14. **AgenticMessage** + **消息拼接**
15. **泛型类型安全**：`BaseModel[M]`/`TypedAgent[M]`/`StreamReader[T]`

### 建议的行动路径

1. **立即行动（P0）**：补齐工程基础设施 + 通用流式抽象 + Checkpoint 持久化 + 运行中抢占/取消 + 模型故障转移
2. **近期行动（P1）**：补齐组件生态 + 预置 Agent 模式 + 多 Agent 协作 + 流式工具 + 消息多模态 + 泛型重构
3. **远期规划（P2）**：中间件系统 + DeepAgent + 动态图 + 外部组件仓库

InferGlow 应该**保持 9 项差异化优势**（JSON 解析/TriggerFlow/LoopGuard/安全/沙箱/审计/契约引擎），同时**快速补齐 15 项功能短板**，才能从"零件库"进化为"生产级框架"。
