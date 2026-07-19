* [x] `.github/workflows/ci.yml` 存在且配置了 go test、go vet、golangci-lint

* [x] `.golangci.yaml` 存在且启用了 gofmt、goimports、govet、revive

* [x] `.licenserc.yaml` 存在且配置了 MIT license header 检查

* [x] 所有 `.go` 文件头部包含 MIT license header

* [x] `model/stream_reader.go` 存在且定义了泛型 `StreamReader[T]`

* [x] StreamReader 支持 Recv、Close、Copy、Merge、Concat 操作

* [x] StreamReader 支持 Pipe（带背压的流管道）、StreamReaderWithConvert（流转换）、ErrNoValue（流过滤）

* [x] `model/stream_reader_test.go` 覆盖读取、关闭、复制、合并、拼接、管道、转换、过滤、EOF

* [x] `components/go.mod` 存在且 module 名为 `github.com/inferglow/components`

* [x] `components/tool/interface.go` 定义了 `BaseTool` 接口和 `ToolInfo` 结构体

* [x] `components/tool/adapt_action.go` 提供了 Action 到 Tool 的适配器

* [x] `components/prompt/interface.go` 定义了 `ChatTemplate` 接口

* [x] `components/prompt/string_template.go` 实现了基于 text/template 的 StringTemplate

* [x] `orchestrator/agent/chatmodel_agent.go` 定义了 `ChatModelAgent` 结构体

* [x] `NewChatModelAgent(sess, actionExt, modelReq)` 返回可用的 Agent

* [x] `ChatModelAgent.Run(ctx, userMessage)` 一行调用完成 Agent 循环

* [x] `flow/persistence.go` 新增了 `CheckpointStore` 接口和 `FileCheckpointStore` 实现

* [x] `flow/persistence.go` 新增了 `Serializer` 接口（Marshal/Unmarshal）

* [x] `Flow` 支持 `AutoCheckpoint` 配置，Pause 时自动保存

* [x] `WithCheckPointID`、`WithWriteToCheckPointID`、`WithForceNewRun`、`WithStateModifier` 选项可用

* [x] `ResumeFromSnapshot` 方法能从快照恢复执行

* [x] `flow/checkpoint_test.go` 覆盖自动保存、恢复、崩溃恢复、ID 定位、版本化、强制重跑、状态修改场景

* [x] `orchestrator/agent/turn_loop.go` 定义了 `TurnLoop` 三态状态机（idle/planning/active）

* [x] `Preempt(reason)` 能抢占当前轮次，中断 LLM 或工具执行

* [x] `orchestrator/agent/cancel.go` 定义了 `CancelMode`（Immediate/AfterChatModel/AfterToolCalls）支持位运算组合

* [x] `Cancel(mode, recursive)` 返回 `CancelHandle`，`Wait()` 阻塞等待取消完成

* [x] 支持 `WithRecursive` 递归传播取消到子 Agent

* [x] 支持超时升级：安全点取消超时后自动升级为立即取消

* [x] `orchestrator/agent/engine.go` 在 executeLoop 关键点集成了 TurnLoop 和 Cancel 检查

* [x] `orchestrator/agent/turn_loop_test.go` 和 `cancel_test.go` 覆盖三态切换、抢占、三种取消模式、递归传播、超时升级

* [x] `model/failover.go` 定义了 `FailoverModelRequester`，包装多个 ModelRequester

* [x] 主 Provider 故障时自动切换到备用 Provider

* [x] 全部 Provider 故障时返回 `AllProvidersFailedError` 聚合错误

* [x] 冷却期过后自动恢复主 Provider

* [x] `model/failover_test.go` 覆盖主故障切换、全部故障、冷却恢复、优先级排序

* [x] 所有新增代码有对应的测试文件

* [x] `go test ./...` 全部通过

* [x] `golangci-lint run` 无 error

