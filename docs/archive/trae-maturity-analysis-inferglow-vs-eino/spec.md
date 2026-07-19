# Inferglow vs Eino 成熟度对比分析 Spec

## Why

InferGlow 定位为 Go 生态的 AI Agent 基础设施框架（对标 Python Agently），目前声称 6 个子模块均完成 MVP。但与同生态的成熟开源框架 Eino（CloudWeGo/ByteDance 出品，对标 LangChain + Google ADK）相比，InferGlow 在工程基础设施、组件生态、Agent 模式、编排能力、流式处理、可观测性、中断恢复等多个维度可能存在显著差距。用户需要一个系统、量化的成熟度对比分析，明确 InferGlow 在实现上的具体不足，为后续优化提供依据。

## What Changes

- 产出一份结构化的成熟度对比分析报告，覆盖以下 10 个维度：
  1. **工程基础设施**：CI/CD、linting、license header、mock 生成、依赖管理
  2. **测试体系**：测试/源码比、benchmark、bugfix 测试、generic 测试、集成测试
  3. **组件生态**：组件抽象完整度（model/tool/retriever/embedding/indexer/prompt/document）
  4. **Agent 模式**：预置 Agent 模式（ChatModelAgent/DeepAgent/Supervisor/PlanExecute vs 单一 Agent）
  5. **编排能力**：Graph/Chain/DAG/Branch/Parallel/Workflow vs Flow/TriggerFlow/13 算子
  6. **流式处理**：StreamReader 抽象 vs StreamChunk 处理
  7. **可观测性**：Callback aspect 机制 vs OTel 集成
  8. **中断/恢复**：Checkpoint 持久化 vs Pause/Signal
  9. **多 Agent 协作**：Host/Supervisor 模式 vs 无
  10. **类型安全与泛型**：BaseModel[M]/TypedAgent[M] vs 非泛型接口

- 每个维度给出：量化数据、定性分析、InferGlow 的具体不足、改进建议
- 最终汇总为一张成熟度评分矩阵

## Impact

- **Affected specs**: 
  - `post-g1-roadmap`（分析结果可能影响后续路线图优先级）
  - `adapt-non-standard-providers`（组件生态差距分析相关）
- **Affected code**: 无代码变更，纯分析产出
- **Deliverable**: `docs/maturity-analysis-inferglow-vs-eino.md` 分析报告

## ADDED Requirements

### Requirement: 量化数据采集
分析 SHALL 基于客观量化数据，包括：各项目测试文件数、源码文件数、benchmark 文件数、bugfix 测试数、依赖数、Go 版本要求、CI 配置文件数、linting 规则数等可统计指标。

#### Scenario: 量化数据可验证
- **WHEN** 报告中给出 "InferGlow 有 180 个测试文件，Eino 有 136 个测试文件"
- **THEN** 该数据可通过 `find <project> -name "*_test.go" | wc -l` 验证

### Requirement: 十维度对比分析
分析 SHALL 覆盖 10 个成熟度维度，每个维度包含：(1) 量化对比数据；(2) InferGlow 现状描述；(3) Eino 现状描述；(4) 差距分析；(5) InferGlow 改进建议。

#### Scenario: 工程基础设施维度
- **WHEN** 分析工程基础设施维度
- **THEN** 报告指出 InferGlow 缺失 CI/CD（无 .github/workflows）、无 golangci-lint 配置、无 license header、无 mock 生成工具链
- **AND** 对比 Eino 的 .golangci.yaml（含 revive/godoclint/funlen/cyclop 规则）、.licenserc.yaml、go.uber.org/mock、.github/workflows/pr-check.yml

#### Scenario: 组件生态维度
- **WHEN** 分析组件生态维度
- **THEN** 报告指出 InferGlow 仅有 model Provider 抽象，缺失 Retriever/Embedding/Indexer/Prompt/Document 组件抽象
- **AND** 对比 Eino 的 components/ 目录下 7 类组件接口定义

#### Scenario: Agent 模式维度
- **WHEN** 分析 Agent 模式维度
- **THEN** 报告指出 InferGlow 仅有单一 Agent 实现（orchestrator/agent/agent.go）
- **AND** 对比 Eino 的 4 种预置 Agent（ChatModelAgent、DeepAgent、Supervisor、PlanExecute）

### Requirement: 成熟度评分矩阵
分析 SHALL 在报告末尾提供一张成熟度评分矩阵，对每个维度按 1-5 分打分（1=初始，2=基础，3=可用，4=成熟，5=标杆），直观展示两项目的差距。

#### Scenario: 评分矩阵可读
- **WHEN** 查看报告末尾的评分矩阵
- **THEN** 矩阵包含 10 个维度行、InferGlow 列、Eino 列、差距列
- **AND** 每个单元格有 1-5 的分数和一句话说明

### Requirement: 改进建议优先级排序
分析 SHALL 对 InferGlow 的改进建议按优先级排序（P0/P1/P2），P0 为"严重影响生产可用性"的差距，P1 为"影响开发者体验"的差距，P2 为"远期增强"。

#### Scenario: P0 优先级识别
- **WHEN** 查看改进建议
- **THEN** CI/CD 缺失、组件生态不完整、无预置 Agent 模式等差距被标记为 P0
- **AND** 每个建议包含具体可执行的改进路径

## MODIFIED Requirements

无。本 Spec 为纯分析产出，不修改任何现有需求或代码。

## REMOVED Requirements

无。
