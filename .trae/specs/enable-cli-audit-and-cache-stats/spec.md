# CLI 审计链路与缓存复盘基础设施 Spec

## Why

- **CLI 任务不可审计**：`buildAgent()` 使用 `agent.New()` → `NoOpHook`，所有审计条目被静默丢弃，CLI 模式下请求/决策/工具执行无迹可查
- **前缀缓存率不可复盘**：`UsageInfo.PromptTokensDetails["cached_tokens"]` 虽能从 Provider 获取，但未持久化、未聚合，无法生成 session/时间维度的缓存效率报告
- **运营盲区**：无 token 用量统计、成本核算、缓存率趋势分析能力，影响成本优化决策

## What Changes

### 1. CLI 审计链路打通（P0）
- `CLIConfig` 增加 `Audit` 嵌套配置块（`Enabled bool` + `StoragePath string` + `SignatureKey string`），**单一开关**，无 `FeatureFlags.AuditChain` 冗余
- `buildAgent()` 根据 `cfg.Audit.Enabled` 决定是否创建 AuditChain 并使用 `NewEngineWithAudit()`
- `EnsureDataDirs()` 增加 `audit/` 目录
- **非破坏性**：默认 `Enabled=false`，不修改现有配置的用户行为不变

### 2. 审计条目元数据增强（P0）
- `engine.go` 的 `auditHook.Append()` 调用中注入 `UsageInfo`（input_tokens, output_tokens, cached_tokens, reasoning_tokens）
- 增加 `Metadata` 键：`model`、`provider`、`input_tokens`、`output_tokens`、`cached_tokens`、`reasoning_tokens`
- 工具执行审计条目同样增加 token 用量（通过 flow context 传递）

### 3. Session 级缓存/用量聚合（P1）
- 新增 `SessionUsageStats` 类型：聚合 input_tokens, output_tokens, cached_tokens, total_cost
- 新增 `UsageRecorder` 组件：监听每轮 LLM 调用的 UsageInfo，实时累加到 session 级统计
- 持久化到 `sessions/{uuid}.usage.jsonl`，按轮次追加

### 4. CLI 复盘命令（P1）
- `/audit query [--source] [--action] [--from] [--to]` — 查询审计条目
- `/audit stats` — 审计条目统计摘要（总条目数、按 source/action 分布）
- `/cost` — 当前 session 的 token 用量与成本明细
- `/cache-stats` — 当前 session 的缓存命中率、节省成本

### 5. 缓存率复盘报告（P2）
- 新增 `CacheReport` 生成器：按时间范围聚合缓存率，支持模型粒度下钻
- 输出格式：表格（CLI 内） + JSON（可导出）
- 指标：总 prompt tokens、cached tokens、缓存命中率、节省成本

## Impact

- **Affected packages**: `cli`, `orchestrator/agent`, `audit`, `model`, `session`
- **Affected files**:
  - `cli/config.go` — CLIConfig 增加 AuditConfig
  - `cli/agent_factory.go` — 改为 NewEngineWithAudit
  - `cli/runtime.go` — 传递审计链
  - `cli/commands.go` — 新增复盘命令
  - `orchestrator/agent/engine.go` — 审计条目注入 UsageInfo
  - `orchestrator/agent/flow_context_impl.go` — 工具审计条目注入 UsageInfo
  - `model/result.go` — 可能增加 UsageInfo 传播接口
  - `session/` — 新增 UsageRecorder / SessionUsageStats
- **No breaking changes**: 所有新增功能默认关闭，零影响现有用户

## Requirements

### Requirement: CLI 审计链路
The system SHALL enable CLI audit chain when configured.

#### Scenario: Audit enabled in CLI config
- **WHEN** user sets `audit.enabled = true` in config.json
- **THEN** CLI REPL/OneShot 模式下所有 agent 决策和工具执行均写入 audit-YYYYMMDD.jsonl
- **AND** audit 条目包含完整的 source, action, input, output, metadata, timestamp

#### Scenario: Audit disabled (default)
- **WHEN** user does not configure audit
- **THEN** CLI 行为不变，零额外开销

### Requirement: Audit 条目 Token 用量
The system SHALL record token usage in each audit entry.

#### Scenario: LLM decision with usage
- **WHEN** engine 执行一轮 LLM 调用
- **THEN** audit entry 的 metadata 包含 model, provider, input_tokens, output_tokens, cached_tokens, reasoning_tokens

### Requirement: Session 用量聚合
The system SHALL aggregate token usage and cost at session level.

#### Scenario: Session running
- **WHEN** agent 在 session 中执行多轮
- **THEN** UsageRecorder 累加每轮用量
- **AND** 用户可在 session 结束时查询总用量和成本

### Requirement: CLI 复盘命令
The system SHALL provide CLI commands for audit query and usage review.

#### Scenario: Audit query
- **WHEN** user runs `/audit query --source agent --action decision`
- **THEN** 返回匹配的审计条目列表

#### Scenario: Cost overview
- **WHEN** user runs `/cost`
- **THEN** 显示当前 session 的 input/output/cached tokens 总数和估算成本

#### Scenario: Cache stats
- **WHEN** user runs `/cache-stats`
- **THEN** 显示缓存命中率、节省成本、按模型维度的缓存效率

### Requirement: 缓存率复盘报告
The system SHALL generate cache efficiency reports.

#### Scenario: Report generation
- **WHEN** user requests cache report
- **THEN** 生成按时间/模型维度的缓存命中率趋势
- **AND** 输出 CLI 表格和可导出的 JSON