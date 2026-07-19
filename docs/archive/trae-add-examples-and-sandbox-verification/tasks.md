# Tasks

- [x] Task 1: 更新 examples/go.mod 添加 audit / orchestrator / workspace 依赖与 replace
  - [x] SubTask 1.1: 在 require 块追加 `github.com/inferglow/audit v0.0.0`、`github.com/inferglow/orchestrator v0.0.0`、`github.com/inferglow/workspace v0.0.0`
  - [x] SubTask 1.2: 追加三条 `replace github.com/inferglow/<name> => ./../<name>` 指令
  - [x] SubTask 1.3: 在 examples 目录执行 `go mod tidy` 确保依赖解析成功

- [x] Task 2: 新增 examples/example_audit.go
  - [x] SubTask 2.1: 文件头 `//go:build ignore` + `package main`，导入 `github.com/inferglow/audit`
  - [x] SubTask 2.2: 演示 NewAuditChain(AuditConfig{Enabled:true, SignatureKey:...}) 构造
  - [x] SubTask 2.3: 演示 Append 多条 AuditEntry（Source/Action/Input/Output/Metadata）
  - [x] SubTask 2.4: 演示 SignEntry 签名与 VerifyEntry 验证
  - [x] SubTask 2.5: 演示 VerifyChain 全链验证通过
  - [x] SubTask 2.6: 演示 Query(QueryFilter{Source:"agent"}) 过滤
  - [x] SubTask 2.7: 演示 Export(ExportJSON/ExportCSV/ExportText, os.Stdout) 三种格式输出

- [x] Task 3: 新增 examples/example_model.go
  - [x] SubTask 3.1: 文件头 `//go:build ignore` + `package main`，导入 `github.com/inferglow/model` 与 `github.com/inferglow/schema`
  - [x] SubTask 3.2: 演示通过 StaticConfigProvider + LoadProviderConfig 构造 OpenAI/Anthropic/Ollama Provider（不实际请求网络）
  - [x] SubTask 3.3: 演示 ModelRequest 构造（System/ChatHistory/Tools/Output schema）
  - [x] SubTask 3.4: 演示 AttemptRunner.Run 重试决策（ClassifyError 对 401/429/5xx 的分类）
  - [x] SubTask 3.5: 演示 OutputValidator 对 ModelResponse 的 Schema 校验流程
  - [x] SubTask 3.6: 演示流式消费伪代码（说明 StreamChunk 字段，不实际联网）

- [x] Task 4: 新增 examples/example_orchestrator.go
  - [x] SubTask 4.1: 文件头 `//go:build ignore` + `package main`，导入 orchestrator/agent、audit、session、action、model
  - [x] SubTask 4.2: 实现一个 mock ModelRequester（返回固定 JSON Decision），避免依赖真实 LLM
  - [x] SubTask 4.3: 演示 session.NewSession + ActionExtension + action.New 注册
  - [x] SubTask 4.4: 演示 NewLoopGuard(LoopGuardConfig{...}) 构造与默认值
  - [x] SubTask 4.5: 演示 NewAuditChain + NewEngineWithAuditAndLoopGuard 组装 Agent
  - [x] SubTask 4.6: 演示 agent.New(...).Run(ctx, userMessage) 执行并打印最终响应
  - [x] SubTask 4.7: 演示 Run 后 AuditChain.VerifyChain 通过、Query 查询决策记录

- [x] Task 5: 新增 examples/example_workspace.go
  - [x] SubTask 5.1: 文件头 `//go:build ignore` + `package main`，导入 `github.com/inferglow/workspace`
  - [x] SubTask 5.2: 演示 workspace.New(Config{RootDir:tmpDir, MaxFileSize, MaxFileCount}) 构造
  - [x] SubTask 5.3: 演示 SafePath 拦截路径穿越（../../etc/passwd 返回 ErrPathOutsideRoot）
  - [x] SubTask 5.4: 演示 WriteFile / ReadFile / ListDir / MkdirAll 安全 IO
  - [x] SubTask 5.5: 演示 NewMemoryLineageStore + Record 多个节点（含 Parents）
  - [x] SubTask 5.6: 演示 Ancestors / Descendants / Children 查询
  - [x] SubTask 5.7: 演示 SaveLineageToFile 持久化到 Workspace 内并 LoadLineageFromFile 回读

- [x] Task 6: 新增 sandbox/live_docker_gvisor_test.go
  - [x] SubTask 6.1: TestLiveDockerDaemonReachable — NewDockerProvider 成功（内部 Ping），失败则 t.Skip
  - [x] SubTask 6.2: TestLiveDockerInspectAvailability — InspectAvailability 返回 Available=true，失败则 t.Skip
  - [x] SubTask 6.3: TestLiveDockerHandleEcho — 创建 alpine:latest 容器（必要时 docker pull），Start → Execute(echo hello) → 断言 ExitCode==0 且 Stdout 含 "hello" → Stop
  - [x] SubTask 6.4: TestLiveGVisorProviderAvailable — NewGVisorProvider 成功（docker + runsc），失败则 t.Skip
  - [x] SubTask 6.5: TestLiveGVisorHandleEcho — 创建 runsc runtime 的 alpine 容器，Start → Execute(echo hello) → 断言 → Stop
  - [x] SubTask 6.6: 确保所有 live 测试在容器创建后 defer Stop，避免残留

- [x] Task 7: 更新 examples/README.md
  - [x] SubTask 7.1: 在示例列表表格追加 example_audit / example_model / example_orchestrator / example_workspace 四行
  - [x] SubTask 7.2: 更新"依赖"小节，列出新增的三条 replace 指令

- [x] Task 8: 验证全部变更
  - [x] SubTask 8.1: `cd examples && go build example_audit.go example_model.go example_orchestrator.go example_workspace.go` 全部成功
  - [x] SubTask 8.2: `cd sandbox && go test -run TestLive -v -timeout 120s` 全部通过或 Skip
  - [x] SubTask 8.3: `cd sandbox && go vet ./...` 无错误
  - [x] SubTask 8.4: `cd examples && go run example_audit.go` 输出正常

# Task Dependencies

- Task 1 必须先完成（其他示例任务依赖 go.mod 中的 replace 指令）
- Task 2 / 3 / 4 / 5 / 6 / 7 可并行执行（互不依赖）
- Task 8 依赖 Task 1-7 全部完成

# 完成情况说明

- 8 个 Task 全部完成，所有 SubTask 已勾选
- Task 8 验证结果：
  - 4 个示例文件逐个编译全部通过（单条 `go build a.go b.go c.go d.go` 会因 4 个 `func main()` 冲突，需分开编译）
  - 5 个 TestLive 测试全部 PASS（TestLiveDockerHandleEcho 端到端 30.43s 通过；TestLiveGVisorHandleEcho 通过）
  - `go vet ./...` 退出码 0，无输出
  - `go run example_audit.go` 输出包含全部 6 段示例
  - `go run example_model.go / example_orchestrator.go / example_workspace.go` 全部正常输出
  - `go test ./... -timeout 180s`：202 PASS / 3 预存 FAIL（bubblewrap/e2b 未改动文件，非本次回归）
- 处理的预先存在问题：在 examples/go.mod 末尾追加 `replace github.com/docker/go-connections v0.7.0 => github.com/docker/go-connections v0.4.0`，解决 `sockets.DialPipe undefined` 编译错误
