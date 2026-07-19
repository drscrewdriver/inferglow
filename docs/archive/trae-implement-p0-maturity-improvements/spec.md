# 实施 P0 成熟度改进 Spec

## Why

成熟度分析报告（`docs/maturity-analysis-inferglow-vs-eino.md`）识别出 InferGlow 在 9 项 P0 能力上与 Eino 存在显著差距：无 CI/CD、无通用流式抽象、无开箱即用 Agent、组件生态不完整、无 Checkpoint 持久化、**无运行中抢占/取消**、**无模型故障转移**。这些差距"严重影响生产可用性"。本 Spec 落地报告中的 9 项 P0 建议，将 InferGlow 从"零件库"推进到"可交付框架"。

## What Changes

### 阶段 1：工程基础设施（快速见效）
- 新增 `.github/workflows/ci.yml`：`go test ./...` + `go vet ./...` + `golangci-lint run`
- 新增 `.golangci.yaml`：启用 `gofmt`、`goimports`、`govet`、`revive`（exported 注释检查）
- 新增 `.licenserc.yaml`：统一 MIT license header
- 为所有 `.go` 文件添加 MIT license header

### 阶段 2：通用流式抽象
- 新增 `model/stream_reader.go`：实现泛型 `StreamReader[T]`
  - 核心方法：`Recv() (T, error)`、`Close()`、`Copy() *StreamReader[T]`
  - 工厂函数：`StreamReaderFromChannel[T](ch)`、`StreamReaderFromArray[T](items)`
  - 工具函数：`MergeStreamReaders[T](readers)`、`ConcatStreamReader[T](r)`
  - 流管道：`Pipe[T](cap int) (*StreamWriter[T], *StreamReader[T])` 支持背压
  - 流转换：`StreamReaderWithConvert[T, U](r, fn)` 函数式转换
  - 流过滤：`ErrNoValue` 哨兵值，转换函数返回该值跳过元素
- 新增 `model/stream_reader_test.go`：覆盖读取、关闭、复制、合并、拼接、管道、转换、过滤

### 阶段 3：组件抽象
- 新增 `components/` 顶层目录（go.mod: `github.com/inferglow/components`）
- 新增 `components/tool/`：
  - `interface.go`：`BaseTool` 接口（`Info(ctx) (*ToolInfo, error)`、`Invoke(ctx, input) (output, error)`）
  - `option.go`：工具选项模式
  - 将现有 `action.ActionSpec` 适配为 `ToolInfo`
- 新增 `components/prompt/`：
  - `interface.go`：`ChatTemplate` 接口（`Format(ctx, vars) ([]ChatMessage, error)`）
  - `chat_template.go`：内置 `StringTemplate` 实现（Go text/template）
  - `option.go`：模板选项

### 阶段 4：开箱即用 Agent
- 新增 `orchestrator/agent/chatmodel_agent.go`：`ChatModelAgent` 结构体
  - 封装 `Session` + `ActionExtension` + `ModelRequester` + `Engine`
  - 提供 `NewChatModelAgent(sess, actionExt, modelReq, opts...) *ChatModelAgent`
  - 提供 `Run(ctx, userMessage) (string, error)` 一行调用
  - 预置合理默认值：maxRounds=10、streamTimeout=5min
- 新增 `orchestrator/agent/chatmodel_agent_test.go`：验证开箱即用

### 阶段 5：自动 Checkpoint 持久化
- 扩展 `flow/persistence.go`：
  - 新增 `AutoCheckpoint` 配置（开启后 Pause 自动保存 Snapshot）
  - 新增 `ResumeFromSnapshot(snapshot *ExecutionSnapshot) *Execution`：从快照恢复执行
  - 新增 `CheckpointStore` 接口（`Save`/`Load`/`Delete`），提供 `FileCheckpointStore` 实现
  - 新增 `Serializer` 接口（`Marshal`/`Unmarshal`），支持自定义序列化（默认 JSON）
  - 新增 `WithCheckPointID(id)` 选项，加载/写入指定 ID 的 Checkpoint
  - 新增 `WithWriteToCheckPointID(id)` 选项，从旧 ID 读、写新 ID（版本化）
  - 新增 `WithForceNewRun()` 选项，忽略 Checkpoint 从头执行
  - 新增 `WithStateModifier(fn)` 选项，在 checkpoint 读写时修改状态
- 新增 `flow/checkpoint_test.go`：覆盖自动保存、恢复执行、崩溃恢复、ID 定位、版本化、强制重跑、状态修改场景

### 阶段 6：运行中抢占与取消
- 新增 `orchestrator/agent/turn_loop.go`：`TurnLoop` 三态状态机
  - 三态：`idle`（空闲）、`planning`（LLM 推理中）、`active`（工具执行中）
  - `Preempt(reason string)`：抢占当前轮次，中断 LLM 或工具执行
  - `Cancel(mode CancelMode, recursive bool)`：取消执行
- 新增 `orchestrator/agent/cancel.go`：`CancelMode` 三种模式
  - `CancelImmediate`：立即取消，不等待安全点
  - `CancelAfterChatModel`：等当前 LLM 调用完成后取消
  - `CancelAfterToolCalls`：等当前工具调用批次完成后取消（保证状态一致）
  - 支持位运算组合（如 `CancelAfterChatModel | CancelAfterToolCalls`）
  - `WithRecursive` 选项：传播取消到子 Agent
  - `CancelHandle.Wait()` 方法：阻塞等待取消完成
  - 超时升级：配置超时后从安全点取消升级为立即取消
- 修改 `orchestrator/agent/engine.go`：在 executeLoop 中集成 TurnLoop 检查点
- 新增 `orchestrator/agent/turn_loop_test.go`、`cancel_test.go`：覆盖三态切换、抢占、三种取消模式、递归传播、超时升级

### 阶段 7：模型故障转移
- 新增 `model/failover.go`：`FailoverModelRequester` 结构体
  - 包装多个 `ModelRequester`，按优先级自动切换
  - 健康检查：记录每个 Provider 的失败次数和冷却时间
  - 故障转移策略：主 Provider 失败后自动切换到备用 Provider
  - 恢复策略：冷却期过后自动恢复主 Provider
- 新增 `model/failover_test.go`：覆盖主故障切换、全部故障、冷却恢复、优先级排序场景

## Impact

- **Affected specs**: 
  - `maturity-analysis-inferglow-vs-eino`（落地 P0 建议）
  - `post-g1-roadmap`（Flow 持久化增强）
- **Affected code**:
  - 新增：`.github/workflows/ci.yml`、`.golangci.yaml`、`.licenserc.yaml`
  - 新增：`model/stream_reader.go`、`model/stream_reader_test.go`
  - 新增：`model/failover.go`、`model/failover_test.go`
  - 新增：`components/go.mod`、`components/tool/`、`components/prompt/`
  - 新增：`orchestrator/agent/chatmodel_agent.go`、`orchestrator/agent/chatmodel_agent_test.go`
  - 新增：`orchestrator/agent/turn_loop.go`、`orchestrator/agent/cancel.go` 及测试
  - 修改：`flow/persistence.go`（新增 AutoCheckpoint、ResumeFromSnapshot、CheckpointStore、Serializer、ID 定位）
  - 修改：`orchestrator/agent/engine.go`（集成 TurnLoop 检查点）
  - 修改：所有 `.go` 文件（添加 license header）
- 无 **BREAKING** 变更：所有新增均为追加，现有 API 保持不变

## ADDED Requirements

### Requirement: CI/CD 流水线
系统 SHALL 提供 GitHub Actions CI 配置，在每次提交和 PR 时自动运行 `go test`、`go vet` 和 `golangci-lint`。

#### Scenario: CI 在 PR 时运行
- **WHEN** 创建 Pull Request
- **THEN** CI 自动触发 `go test ./...` 验证所有模块测试通过
- **AND** 运行 `go vet ./...` 检查静态错误
- **AND** 运行 `golangci-lint run` 检查代码风格

### Requirement: Linting 配置
系统 SHALL 提供 `.golangci.yaml` 配置，启用 `gofmt`、`goimports`、`govet`、`revive` linter，强制 exported 符号有注释。

#### Scenario: lint 检查 exported 符号
- **WHEN** 执行 `golangci-lint run`
- **THEN** 未添加文档注释的 exported 函数/类型/变量被报告为 error

### Requirement: License Header 规范
系统 SHALL 为所有 `.go` 文件统一添加 MIT license header，并通过 `.licenserc.yaml` 配置强制检查。

#### Scenario: 文件包含 license header
- **WHEN** 检查任意 `.go` 文件头部
- **THEN** 包含 MIT license 声明文本

### Requirement: 通用 StreamReader 抽象
系统 SHALL 提供泛型 `StreamReader[T]` 抽象，支持读取、关闭、复制、合并流式数据，解耦 Provider 与消费方。

#### Scenario: 从 channel 创建 StreamReader
- **WHEN** 调用 `StreamReaderFromChannel[T](ch)` 创建 reader
- **THEN** `Recv()` 依次返回 channel 中的元素
- **AND** channel 关闭后 `Recv()` 返回 `io.EOF`
- **AND** `Close()` 关闭 reader 并释放资源

#### Scenario: 复制 StreamReader 支持多消费者
- **WHEN** 调用 `reader.Copy()` 创建副本
- **THEN** 原始 reader 和副本可独立读取相同数据
- **AND** 任一 reader 的 `Recv()` 不影响另一 reader

#### Scenario: 合并多个 StreamReader
- **WHEN** 调用 `MergeStreamReaders[T](readers)` 合并多个 reader
- **THEN** 返回的 reader 按到达顺序产出所有 reader 的元素
- **AND** 所有 reader 读完后 `Recv()` 返回 `io.EOF`

#### Scenario: 流管道支持背压
- **WHEN** 调用 `Pipe[T](cap)` 创建 writer/reader 对
- **THEN** writer 写入数据后 reader 可读取
- **AND** 当缓冲满时 writer 阻塞（背压）
- **AND** reader 关闭后 writer 写入返回错误

#### Scenario: 流转换跳过元素
- **WHEN** 调用 `StreamReaderWithConvert[T, U](r, fn)` 转换流
- **AND** fn 返回 `ErrNoValue`
- **THEN** 该元素被跳过，不产出
- **AND** 继续读取下一个元素

### Requirement: Tool 组件抽象
系统 SHALL 提供 `components/tool/` 模块，定义 `BaseTool` 接口，统一工具的信息描述和调用方式，与现有 `action` 模块适配。

#### Scenario: Tool 接口调用
- **WHEN** 实现 `BaseTool` 接口并调用 `Invoke(ctx, input)`
- **THEN** 返回工具执行结果
- **AND** `Info(ctx)` 返回工具名称、描述、参数 schema

#### Scenario: 从 Action 适配为 Tool
- **WHEN** 将现有 `action.Action` 包装为 `BaseTool`
- **THEN** `ToolInfo` 包含 Action 的 Name、Description、Schema
- **AND** `Invoke` 委托给 Action 的 Executor

### Requirement: Prompt 模板抽象
系统 SHALL 提供 `components/prompt/` 模块，定义 `ChatTemplate` 接口，支持变量渲染生成消息列表。

#### Scenario: 字符串模板渲染
- **WHEN** 创建 `StringTemplate` 并调用 `Format(ctx, vars)`
- **THEN** 模板中的 `{{.variable}}` 被替换为 vars 中对应的值
- **AND** 返回填充后的 `[]ChatMessage`

### Requirement: 开箱即用 ChatModelAgent
系统 SHALL 提供 `ChatModelAgent`，封装 Session + ActionExtension + ModelRequester + Engine，用户一行调用即可运行 Agent。

#### Scenario: 最简调用
- **WHEN** 用户调用 `NewChatModelAgent(sess, actionExt, modelReq).Run(ctx, "你好")`
- **THEN** Agent 自动执行 PLAN → EXECUTE 循环
- **AND** 返回最终响应字符串
- **AND** 使用预置默认值（maxRounds=10、streamTimeout=5min）

#### Scenario: 自定义配置
- **WHEN** 用户传入 `WithMaxRounds(5)`、`WithSystemPrompt("你是助手")`
- **THEN** Agent 使用自定义配置而非默认值

### Requirement: 自动 Checkpoint 持久化
系统 SHALL 在 Flow Pause 时自动保存 ExecutionSnapshot 到 CheckpointStore，支持从快照恢复执行。

#### Scenario: Pause 自动保存
- **WHEN** Flow 开启 `AutoCheckpoint` 并配置 `FileCheckpointStore`
- **AND** 执行 `Pause(reason)`
- **THEN** ExecutionSnapshot 自动保存到指定文件路径
- **AND** 返回的 PausePoint 包含 checkpoint 路径

#### Scenario: 从 Checkpoint 恢复
- **WHEN** 调用 `ResumeFromSnapshot(snapshot)` 传入已保存的快照
- **THEN** 创建新 Execution 从快照记录的暂停步骤继续执行
- **AND** 状态（StepLog、Result、Errors）从快照恢复

#### Scenario: 崩溃后恢复
- **WHEN** 进程崩溃后重启，调用 `FileCheckpointStore.Load(path)` 加载快照
- **AND** 调用 `ResumeFromSnapshot(snapshot)` 恢复执行
- **THEN** Flow 从崩溃前的暂停点继续执行

#### Scenario: Checkpoint ID 定位
- **WHEN** 使用 `WithCheckPointID("run-001")` 配置
- **THEN** `Save` 写入 ID 为 "run-001" 的 Checkpoint
- **AND** `Load("run-001")` 加载对应的 Checkpoint

#### Scenario: 版本化写入
- **WHEN** 使用 `WithWriteToCheckPointID("run-001-v2")` 配置
- **AND** 从旧 ID "run-001" 读取 Checkpoint
- **THEN** 恢复后写入新 ID "run-001-v2"，保留旧版本

#### Scenario: 强制重跑
- **WHEN** 使用 `WithForceNewRun()` 配置
- **THEN** 忽略已存在的 Checkpoint，从头执行
- **AND** 执行结果写入新 Checkpoint

#### Scenario: 状态修改器
- **WHEN** 使用 `WithStateModifier(fn)` 配置
- **THEN** 在 Checkpoint 读写时调用 fn 修改状态
- **AND** 修改后的状态被持久化

### Requirement: 运行中抢占与取消
系统 SHALL 提供 `TurnLoop` 三态状态机和 `CancelMode` 三种取消模式，支持运行中抢占当前轮次和按安全点取消执行。

#### Scenario: 三态状态机切换
- **WHEN** Agent 空闲时状态为 `idle`
- **AND** 开始 LLM 调用时切换到 `planning`
- **AND** 开始工具执行时切换到 `active`
- **THEN** 工具执行完成后回到 `idle`

#### Scenario: 抢占当前轮次
- **WHEN** Agent 处于 `planning` 或 `active` 状态
- **AND** 调用 `Preempt(reason)`
- **THEN** 当前 LLM 调用或工具执行被中断
- **AND** Agent 回到 `idle` 状态等待新指令

#### Scenario: 立即取消
- **WHEN** 调用 `Cancel(CancelImmediate, false)`
- **THEN** 立即终止执行，不等待安全点
- **AND** `CancelHandle.Wait()` 返回后执行已完全停止

#### Scenario: 工具调用后取消
- **WHEN** 调用 `Cancel(CancelAfterToolCalls, false)`
- **AND** Agent 正在执行工具
- **THEN** 等待当前工具调用批次完成
- **AND** 完成后取消执行，保证状态一致

#### Scenario: 递归取消传播
- **WHEN** 调用 `Cancel(mode, true)`（recursive=true）
- **AND** Agent 有子 Agent 在运行
- **THEN** 取消信号传播到所有子 Agent
- **AND** 所有子 Agent 也按相同 mode 取消

#### Scenario: 超时升级
- **WHEN** 使用安全点取消（如 `CancelAfterToolCalls`）
- **AND** 等待超过配置的超时时间
- **THEN** 自动升级为 `CancelImmediate` 立即取消

### Requirement: 模型故障转移
系统 SHALL 提供 `FailoverModelRequester`，包装多个 `ModelRequester`，主 Provider 故障时自动切换到备用 Provider，冷却期后恢复。

#### Scenario: 主故障自动切换
- **WHEN** 主 Provider 请求失败
- **THEN** 自动切换到下一个优先级的备用 Provider
- **AND** 请求在备用 Provider 上重试
- **AND** 主 Provider 进入冷却期

#### Scenario: 全部 Provider 故障
- **WHEN** 所有 Provider 都失败
- **THEN** 返回聚合错误（包含所有 Provider 的失败原因）
- **AND** 错误类型为 `AllProvidersFailedError`

#### Scenario: 冷却期恢复
- **WHEN** 主 Provider 冷却期过后
- **THEN** 下次请求自动恢复使用主 Provider
- **AND** 失败计数器重置

#### Scenario: 优先级排序
- **WHEN** 配置多个 Provider `[A, B, C]`
- **THEN** 正常情况下按 A → B → C 顺序尝试
- **AND** A 故障后按 B → C → A（A 在冷却）顺序尝试

## MODIFIED Requirements

### Requirement: flow/persistence.go
现有 `ExecutionPersistence` 仅支持手动 `SaveJSON`/`SaveYAML`。修改后新增 `AutoCheckpoint` 配置（Pause 时自动保存）、`ResumeFromSnapshot` 方法（从快照恢复）、`CheckpointStore` 接口（抽象存储后端）、`Serializer` 接口（自定义序列化）、`WithCheckPointID`/`WithWriteToCheckPointID`/`WithForceNewRun`/`WithStateModifier` 选项。

### Requirement: orchestrator/agent/engine.go
现有 `executeLoop` 是静态循环（启动后无法干预）。修改后在循环关键点（LLM 调用前、工具执行前、轮次结束时）集成 `TurnLoop` 状态检查，支持运行中抢占和取消。

## REMOVED Requirements

无。本 Spec 全部为新增能力，不破坏现有功能。
