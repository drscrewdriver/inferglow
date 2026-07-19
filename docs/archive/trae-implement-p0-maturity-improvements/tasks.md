# Tasks

## 阶段 1：工程基础设施（快速见效，无依赖）

- [x] Task 1: 建立 CI/CD 流水线
  - [x] SubTask 1.1: 新增 `.github/workflows/ci.yml`，配置 `go test ./...`、`go vet ./...`、`golangci-lint run`，触发条件为 push 和 pull_request
  - [x] SubTask 1.2: 新增 `.golangci.yaml`，启用 gofmt、goimports、govet、revive（exported 注释检查），排除 `_test.go` 文件
  - [x] SubTask 1.3: 新增 `.licenserc.yaml`，配置 MIT license header 检查规则

- [x] Task 2: 统一 License Header
  - [x] SubTask 2.1: 为 inferglow 根目录及所有子模块的 `.go` 文件添加 MIT license header
  - [x] SubTask 2.2: 验证 `golangci-lint run` 和 license 检查通过

## 阶段 2：通用流式抽象（无依赖，可与阶段 1 并行）

- [x] Task 3: 实现 StreamReader[T] 泛型抽象
  - [x] SubTask 3.1: 新增 `model/stream_reader.go`，定义 `StreamReader[T]` 结构体和 `Recv()`/`Close()`/`Copy()` 方法
  - [x] SubTask 3.2: 实现 `StreamReaderFromChannel[T](ch <-chan T)` 工厂函数
  - [x] SubTask 3.3: 实现 `StreamReaderFromArray[T](items []T)` 工厂函数
  - [x] SubTask 3.4: 实现 `MergeStreamReaders[T](readers []*StreamReader[T])` 合并函数
  - [x] SubTask 3.5: 实现 `ConcatStreamReader[T](r *StreamReader[T])` 拼接函数
  - [x] SubTask 3.6: 实现 `Pipe[T](cap int) (*StreamWriter[T], *StreamReader[T])` 流管道（支持背压）
  - [x] SubTask 3.7: 实现 `StreamReaderWithConvert[T, U](r, fn)` 流转换 + `ErrNoValue` 哨兵值跳过元素
  - [x] SubTask 3.8: 新增 `model/stream_reader_test.go`，覆盖读取、关闭、复制、合并、拼接、管道、转换、过滤、EOF 处理

## 阶段 3：组件抽象（依赖阶段 2 的 StreamReader）

- [x] Task 4: 创建 components 顶层模块
  - [x] SubTask 4.1: 新增 `components/go.mod`（module `github.com/inferglow/components`，go 1.21）
  - [x] SubTask 4.2: 新增 `components/tool/interface.go`，定义 `BaseTool` 接口（`Info(ctx) (*ToolInfo, error)`、`Invoke(ctx, input string) (string, error)`）和 `ToolInfo` 结构体
  - [x] SubTask 4.3: 新增 `components/tool/option.go`，定义工具选项模式
  - [x] SubTask 4.4: 新增 `components/tool/adapt_action.go`，提供 `ActionToTool(action *action.Action) BaseTool` 适配器
  - [x] SubTask 4.5: 新增 `components/tool/interface_test.go` 和 `adapt_action_test.go`

- [x] Task 5: 实现 Prompt 模板抽象
  - [x] SubTask 5.1: 新增 `components/prompt/interface.go`，定义 `ChatTemplate` 接口（`Format(ctx, vars map[string]any) ([]model.ChatMessage, error)`）
  - [x] SubTask 5.2: 新增 `components/prompt/string_template.go`，实现 `StringTemplate`（基于 Go text/template）
  - [x] SubTask 5.3: 新增 `components/prompt/option.go`，定义模板选项
  - [x] SubTask 5.4: 新增 `components/prompt/string_template_test.go`，覆盖变量替换、条件渲染、多消息格式

## 阶段 4：开箱即用 Agent（依赖阶段 3 的 Tool 抽象）

- [x] Task 6: 实现 ChatModelAgent
  - [x] SubTask 6.1: 新增 `orchestrator/agent/chatmodel_agent.go`，定义 `ChatModelAgent` 结构体，封装 Session + ActionExtension + ModelRequester + Engine
  - [x] SubTask 6.2: 实现 `NewChatModelAgent(sess, actionExt, modelReq, opts...) *ChatModelAgent` 构造函数，预置默认值（maxRounds=10、streamTimeout=5min）
  - [x] SubTask 6.3: 实现 `Run(ctx, userMessage) (string, error)` 方法，一行调用完成 Agent 循环
  - [x] SubTask 6.4: 支持所有现有 RunOption（WithMaxRounds、WithSystemPrompt、WithStreamTimeout、WithPIIMasker）
  - [x] SubTask 6.5: 新增 `orchestrator/agent/chatmodel_agent_test.go`，验证开箱即用、自定义配置、mock LLM 链路

## 阶段 5：自动 Checkpoint 持久化（无依赖，可与阶段 3/4 并行）

- [x] Task 7: 扩展 Flow Checkpoint 能力
  - [x] SubTask 7.1: 在 `flow/persistence.go` 新增 `CheckpointStore` 接口（`Save(snapshot *ExecutionSnapshot) error`、`Load(id string) (*ExecutionSnapshot, error)`、`Delete(id string) error`）
  - [x] SubTask 7.2: 实现 `FileCheckpointStore`（基于文件系统的存储，路径为 `{dir}/{executionID}.json`）
  - [x] SubTask 7.3: 新增 `Serializer` 接口（`Marshal(s *ExecutionSnapshot) ([]byte, error)`、`Unmarshal(data []byte) (*ExecutionSnapshot, error)`），提供默认 JSON 实现
  - [x] SubTask 7.4: 在 `Flow` 结构体新增 `autoCheckpoint bool` 和 `checkpointStore CheckpointStore` 字段
  - [x] SubTask 7.5: 修改 `Pause()` 方法，当 `autoCheckpoint` 为 true 时自动调用 `checkpointStore.Save()`
  - [x] SubTask 7.6: 新增 `ResumeFromSnapshot(snapshot *ExecutionSnapshot) *Execution` 方法，从快照恢复执行
  - [x] SubTask 7.7: 新增 `WithCheckPointID(id)`、`WithWriteToCheckPointID(id)`、`WithForceNewRun()`、`WithStateModifier(fn)` 选项
  - [x] SubTask 7.8: 新增 `flow/checkpoint_test.go`，覆盖自动保存、恢复执行、FileCheckpointStore、崩溃恢复、ID 定位、版本化、强制重跑、状态修改场景

## 阶段 6：运行中抢占与取消（依赖阶段 4 的 ChatModelAgent）

- [x] Task 8: 实现 TurnLoop 三态状态机
  - [x] SubTask 8.1: 新增 `orchestrator/agent/turn_loop.go`，定义 `TurnLoop` 结构体和三态（`idle`/`planning`/`active`）
  - [x] SubTask 8.2: 实现 `Preempt(reason string) error` 方法，抢占当前轮次
  - [x] SubTask 8.3: 实现 `enterPlanning()`、`enterActive()`、`enterIdle()` 内部状态切换方法
  - [x] SubTask 8.4: 新增 `orchestrator/agent/turn_loop_test.go`，覆盖三态切换、抢占、并发安全

- [x] Task 9: 实现 CancelMode 三种取消模式
  - [x] SubTask 9.1: 新增 `orchestrator/agent/cancel.go`，定义 `CancelMode` 类型（`CancelImmediate`/`CancelAfterChatModel`/`CancelAfterToolCalls`）支持位运算组合
  - [x] SubTask 9.2: 实现 `Cancel(mode CancelMode, recursive bool) *CancelHandle` 方法
  - [x] SubTask 9.3: 实现 `CancelHandle.Wait()` 方法，阻塞等待取消完成
  - [x] SubTask 9.4: 实现 `WithRecursive` 选项，传播取消到子 Agent
  - [x] SubTask 9.5: 实现超时升级机制：安全点取消超时后自动升级为立即取消
  - [x] SubTask 9.6: 修改 `orchestrator/agent/engine.go`，在 executeLoop 关键点（LLM 调用前、工具执行前、轮次结束）集成 TurnLoop 和 Cancel 检查
  - [x] SubTask 9.7: 新增 `orchestrator/agent/cancel_test.go`，覆盖三种取消模式、递归传播、超时升级、位运算组合

## 阶段 7：模型故障转移（无依赖，可与阶段 6 并行）

- [x] Task 10: 实现 FailoverModelRequester
  - [x] SubTask 10.1: 新增 `model/failover.go`，定义 `FailoverModelRequester` 结构体，包装多个 `ModelRequester`
  - [x] SubTask 10.2: 实现健康检查机制：记录每个 Provider 的失败次数、冷却时间、最后失败时间
  - [x] SubTask 10.3: 实现故障转移策略：主 Provider 失败后按优先级切换到备用 Provider
  - [x] SubTask 10.4: 实现恢复策略：冷却期过后自动恢复主 Provider，重置失败计数器
  - [x] SubTask 10.5: 定义 `AllProvidersFailedError` 错误类型，聚合所有 Provider 的失败原因
  - [x] SubTask 10.6: 新增 `model/failover_test.go`，覆盖主故障切换、全部故障、冷却恢复、优先级排序场景

# Task Dependencies

- **Task 1（CI/CD）** 无依赖，可独立开始
- **Task 2（License）** 无依赖，可与 Task 1 并行
- **Task 3（StreamReader）** 无依赖，可与阶段 1 并行
- **Task 4（Tool 组件）** 无强依赖，可与 Task 3 并行（但测试可能需要 model 模块）
- **Task 5（Prompt 组件）** 依赖 Task 4（同属 components 模块，共享 go.mod）
- **Task 6（ChatModelAgent）** 依赖 Task 4（Tool 抽象可用后，Agent 可统一工具接口）
- **Task 7（Checkpoint）** 无依赖，可与阶段 3/4 并行
- **Task 8（TurnLoop）** 依赖 Task 6（需要 ChatModelAgent 作为集成基础）
- **Task 9（Cancel）** 依赖 Task 8（需要 TurnLoop 状态机）
- **Task 10（Failover）** 无依赖，可与阶段 6 并行

# Parallelizable Work

以下任务组可并行执行：
1. **阶段 1 + 阶段 2 + 阶段 5 + 阶段 7**：Task 1（CI/CD）+ Task 2（License）+ Task 3（StreamReader）+ Task 7（Checkpoint）+ Task 10（Failover）完全独立
2. **阶段 3 + 阶段 5**：Task 4/5（组件抽象）与 Task 7（Checkpoint）独立
3. **阶段 4** 依赖阶段 3 的 Tool 抽象完成
4. **阶段 6** 依赖阶段 4 的 ChatModelAgent 完成
