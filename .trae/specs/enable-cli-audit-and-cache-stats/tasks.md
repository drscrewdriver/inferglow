# Tasks

## Task 1: CLI 审计配置与链路打通（P0）
修改 CLI 配置和启动链路，使 CLI 模式支持审计链。

- [x] SubTask 1.1: `CLIConfig` 增加 `Audit` 嵌套配置块（`Enabled bool`, `StoragePath string`, `SignatureKey string`），**单一开关**，不新增 `FeatureFlags` 字段避免冗余
- [x] SubTask 1.2: `EnsureDataDirs()` 增加 `audit/` 目录创建
- [x] SubTask 1.3: `buildAgent()` 增加重载——当 `cfg.Audit.Enabled` 为 true 时，创建 `audit.AuditChain` 实例并传入 `NewEngineWithAudit()`；否则保持现有 `agent.New()` 路径
- [x] SubTask 1.4: `AgentRuntime` 持有 `AuditChain` 引用，`Close()` 时确保审计链正确关闭
- [x] SubTask 1.5: 新增单元测试验证：audit enabled 时审计条目写入 jsonl、audit disabled 时零额外调用

## Task 2: 审计条目元数据注入 Token 用量（P0）
在 engine 和 action 审计条目中注入 UsageInfo 和模型信息。

- [x] SubTask 2.1: `engine.go` 的 `executeLoop` 中，获取 LLM 响应中的 `UsageInfo`，注入到 `auditEntry.Metadata`（model, provider, input_tokens, output_tokens, cached_tokens, reasoning_tokens）
- [x] SubTask 2.2: `flow_context_impl.go` 的 `AuditAppend` 方法增加 `UsageInfo` 参数，使其能传递 token 用量到 action 审计条目
- [x] SubTask 2.3: 新增单元测试验证：audit entry 的 metadata 包含正确的 token 用量字段

## Task 3: Session 级用量聚合（P1）
创建 `SessionUsageStats` 和 `UsageRecorder`，支持 session 级别的 token 用量和成本聚合。

- [x] SubTask 3.1: 在 `session/` 包下新增 `usage_stats.go`：定义 `SessionUsageStats` 结构体（input_tokens, output_tokens, cached_tokens, reasoning_tokens, total_cost, 每次调用的明细记录）
- [x] SubTask 3.2: 新增 `UsageRecorder` 组件：提供 `Record(usage UsageInfo, model, provider string)` 方法，累加统计并追加到 `sessions/{uuid}.usage.jsonl`
- [x] SubTask 3.3: `UsageRecorder` 支持 `Summary()` 方法返回当前 session 的聚合摘要
- [x] SubTask 3.4: 新增单元测试验证：多轮调用后 Summary 数据正确、持久化可恢复

## Task 4: CLI 复盘命令（P1）
在 CLI 中新增审计查询和用量统计命令。

- [x] SubTask 4.1: `commands.go` 注册 `/audit` 子命令（`query`, `stats`），实现审计条目查询和统计
- [x] SubTask 4.2: 注册 `/cost` 命令，调用 `UsageRecorder.Summary()` 显示当前 session 用量
- [x] SubTask 4.3: 注册 `/cache-stats` 命令，计算并显示缓存命中率、节省成本
- [x] SubTask 4.4: 新增集成测试验证：命令执行后输出格式正确、数据可读

## Task 5: 缓存率复盘报告（P2）
生成跨 session 的缓存效率复盘报告。

- [x] SubTask 5.1: 在 `session/` 包下新增 `cache_report.go`：定义 `CacheReport` 结构体和 `ReportGenerator`，按时间/模型维度聚合缓存率
- [x] SubTask 5.2: 实现 CLI 表格输出和 JSON 导出格式
- [x] SubTask 5.3: 注册 `/cache-report` 命令，支持 `--from` `--to` `--model` 过滤参数
- [x] SubTask 5.4: 新增单元测试验证：报告数据聚合正确、边界情况（空数据、单 session）处理正常

# Task Dependencies

- [Task 2] 依赖于 [Task 1]（审计链路需要先打通才能写入含用量的条目）
- [Task 3] 不依赖 [Task 1/2]（可独立实现）
- [Task 4] 依赖于 [Task 1] 和 [Task 3]（复盘命令需要审计数据和用量数据）
- [Task 5] 依赖于 [Task 3]（报告基于用量聚合数据）

# 并行策略

- 第一轮并行：Task 1（CLI 审计链路） + Task 3（Session 用量聚合）——无依赖关系，可同时进行
- 第二轮并行：Task 2（审计条目注入用量）——依赖 Task 1 完成后进行
- 第三轮：Task 4（CLI 复盘命令）——依赖 Task 1 和 Task 3 完成后进行
- 第四轮：Task 5（缓存率报告）——依赖 Task 3 完成后进行