# Tasks

- [x] Task 1: 采集量化数据
  - [x] SubTask 1.1: 统计两项目的测试文件数、源码文件数、benchmark 文件数、bugfix 测试数、generic 测试数
  - [x] SubTask 1.2: 检查两项目的工程配置文件（.golangci.yaml、.licenserc.yaml、.github/workflows/、CONTRIBUTING.md、CODE_OF_CONDUCT.md）
  - [x] SubTask 1.3: 对比两项目的 go.mod 依赖数、Go 版本要求、replace 指令使用
  - [x] SubTask 1.4: 统计两项目的 mock 文件数、examples 数、文档文件数

- [x] Task 2: 撰写十维度对比分析
  - [x] SubTask 2.1: 工程基础设施对比（CI/CD、linting、license、mock 工具链）
  - [x] SubTask 2.2: 测试体系对比（测试/源码比、测试类型分布、benchmark 覆盖）
  - [x] SubTask 2.3: 组件生态对比（7 类组件抽象 vs 仅 model Provider）
  - [x] SubTask 2.4: Agent 模式对比（4 种预置 Agent vs 单一 Agent）
  - [x] SubTask 2.5: 编排能力对比（Graph/Chain/DAG/Branch/Parallel/Workflow vs Flow/TriggerFlow/13 算子）
  - [x] SubTask 2.6: 流式处理对比（StreamReader 抽象 vs StreamChunk）
  - [x] SubTask 2.7: 可观测性对比（Callback aspect vs OTel）
  - [x] SubTask 2.8: 中断/恢复对比（Checkpoint 持久化 vs Pause/Signal）
  - [x] SubTask 2.9: 多 Agent 协作对比（Host/Supervisor vs 无）
  - [x] SubTask 2.10: 类型安全与泛型对比（BaseModel[M]/TypedAgent[M] vs 非泛型）

- [x] Task 3: 生成成熟度评分矩阵
  - [x] SubTask 3.1: 为每个维度按 1-5 分打分
  - [x] SubTask 3.2: 计算总分和平均分
  - [x] SubTask 3.3: 标注差距最大的 Top 3 维度

- [x] Task 4: 整理改进建议优先级排序
  - [x] SubTask 4.1: 识别 P0 级差距（严重影响生产可用性）
  - [x] SubTask 4.2: 识别 P1 级差距（影响开发者体验）
  - [x] SubTask 4.3: 识别 P2 级差距（远期增强）
  - [x] SubTask 4.4: 为每个建议给出具体改进路径

- [x] Task 5: 输出最终分析报告
  - [x] SubTask 5.1: 将所有分析内容整合为 `docs/maturity-analysis-inferglow-vs-eino.md`
  - [x] SubTask 5.2: 确保报告结构清晰、数据可验证、建议可执行

# Task Dependencies
- [Task 2] 依赖 [Task 1]（需要量化数据支撑分析）
- [Task 3] 依赖 [Task 2]（需要维度分析完成才能打分）
- [Task 4] 依赖 [Task 2]（需要差距分析才能排序建议）
- [Task 5] 依赖 [Task 2]、[Task 3]、[Task 4]（整合所有内容）
