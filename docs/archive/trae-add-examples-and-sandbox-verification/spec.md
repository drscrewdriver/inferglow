# 补齐缺失示例并验证 Docker/gVisor 连接 Spec

## Why

`plan-0721.md` §9.1 列出 4 个缺失的独立示例程序（model / orchestrator / audit / workspace），用户难以快速上手这些模块；§9.5 指出缺少真实端到端的沙箱集成测试。当前 `sandbox/docker_test.go` 与 `gvisor_test.go` 仅在 mock 层面验证构造函数，未真正拉起容器验证 Docker daemon 与 runsc 运行时的连通性。本变更补齐这两类缺口。

## What Changes

* 新增 `examples/example_audit.go`：演示 AuditChain 的 Append / Sign / VerifyChain / Export / Query 完整链路（使用 `//go:build ignore` 与现有示例一致）。

* 新增 `examples/example_model.go`：演示 3 个 Provider（OpenAI / Anthropic / Ollama）的构造、Streaming 消费、AttemptRunner 重试、OutputValidator Schema 校验。

* 新增 `examples/example_orchestrator.go`：演示 Agent + Engine + LoopGuard + AuditHook 的组装与 Run 流程（使用 mock LLM provider 避免依赖外部服务）。

* 新增 `examples/example_workspace.go`：演示 Workspace 安全 IO（WriteFile / ReadFile / SafePath 防穿越）+ MemoryLineageStore 血缘记录与 Ancestors/Descendants 查询。

* 更新 `examples/go.mod`：在 require 与 replace 中新增 `github.com/inferglow/audit`、`github.com/inferglow/orchestrator`、`github.com/inferglow/workspace` 三项本地 replace。

* 更新 `examples/README.md`：在示例列表表格中追加 4 个新示例条目。

* 新增 `sandbox/live_docker_gvisor_test.go`：真实环境集成测试，验证 Docker daemon Ping、`alpine:latest` 容器 echo、gVisor runsc 容器 echo 端到端连通性；Docker 或 runsc 不可用时 `t.Skip` 优雅跳过。

## Impact

* Affected specs: plan-0721.md §9.1（示例缺失）、§9.5（测试覆盖增强）

* Affected code:

  * `examples/` 目录新增 4 个 `.go` 文件与 go.mod / README 修改

  * `sandbox/` 目录新增 1 个 `_test.go` 文件

* 无 **BREAKING** 变更：所有示例文件使用 `//go:build ignore`，不参与 `go build ./...` / `go test ./...`；新增测试在环境不满足时 Skip，不影响现有 CI。

## ADDED Requirements

### Requirement: 四个缺失示例程序

系统 SHALL 在 `examples/` 目录下提供 4 个独立可运行的示例程序，分别演示 audit / model / orchestrator / workspace 模块的核心能力。

#### Scenario: 示例可独立编译运行

* **WHEN** 开发者在 `examples/` 目录执行 `go build example_audit.go`（及其他三个）

* **THEN** 编译成功无错误

* **AND** 执行 `go run example_audit.go` 能输出演示文本并正常退出

#### Scenario: 示例遵循现有约定

* **WHENT** 检查示例文件头部

* **THEN** 包含 `//go:build ignore` 构建标签

* **AND** 使用 `package main` 与 `func main()`

* **AND** 通过 `github.com/inferglow/<module>` 路径导入子模块

#### Scenario: example\_audit 演示完整审计链路

* **WHEN** 运行 example\_audit

* **THEN** 输出包含：NewAuditChain → Append 多条 entry → SignEntry 签名 → VerifyChain 通过 → Query 过滤 → Export(JSON/CSV/Text) 输出

#### Scenario: example\_model 演示多 Provider 与重试

* **WHEN** 运行 example\_model

* **THEN** 输出包含：OpenAI/Anthropic/Ollama Provider 构造（通过 ConfigProvider 加载配置）、Streaming chunk 消费、AttemptRunner 重试决策分类、OutputValidator 校验

#### Scenario: example\_orchestrator 演示 Agent 全链路

* **WHEN** 运行 example\_orchestrator

* **THEN** 输出包含：Session + ActionExtension + mock ModelRequester 组装 Agent → NewEngineWithAuditAndLoopGuard → Run 完成 → AuditChain 验证 → LoopGuard 状态展示

#### Scenario: example\_workspace 演示安全 IO 与血缘

* **WHEN** 运行 example\_workspace

* **THEN** 输出包含：Workspace 创建 → 路径穿越拦截 → WriteFile/ReadFile → MemoryLineageStore.Record → Ancestors/Descendants 查询 → SaveLineageToFile 持久化

### Requirement: Docker 与 gVisor 真实连接验证

系统 SHALL 提供真实环境集成测试，验证 Docker daemon 与 gVisor runsc 运行时的端到端连通性，并在环境不满足时优雅跳过。

#### Scenario: Docker daemon 可达时验证容器执行

* **WHEN** 执行 `TestLiveDockerHandleEcho` 且 Docker daemon 可达

* **THEN** 测试创建 alpine:latest 容器（必要时自动拉取）

* **AND** 启动容器并执行 `echo hello`

* **AND** 断言 ExitCode==0 且 Stdout 包含 "hello"

* **AND** 测试结束后停止并移除容器

#### Scenario: Docker 不可达时优雅跳过

* **WHEN** 执行任一 Docker live 测试且 `NewDockerProvider()` 返回 ErrProviderUnavailable

* **THEN** 测试调用 `t.Skip` 而非失败

#### Scenario: gVisor runsc 可用时验证 runsc 容器执行

* **WHEN** 执行 `TestLiveGVisorHandleEcho` 且 docker + runsc 均可用

* **THEN** 测试创建使用 `runsc` runtime 的 alpine 容器

* **AND** 启动并执行 `echo hello`

* **AND** 断言 ExitCode==0 且 Stdout 包含 "hello"

#### Scenario: runsc 不可用时优雅跳过

* **WHEN** 执行任一 gVisor live 测试且 `NewGVisorProvider()` 返回 ErrProviderUnavailable

* **THEN** 测试调用 `t.Skip` 而非失败

### Requirement: examples 模块依赖更新

系统 SHALL 在 `examples/go.mod` 中声明对 audit / orchestrator / workspace 三个本地子模块的依赖与 replace 指令。

#### Scenario: go.mod 包含新依赖

* **WHEN** 检查 `examples/go.mod`

* **THEN** require 块包含 `github.com/inferglow/audit`、`github.com/inferglow/orchestrator`、`github.com/inferglow/workspace`

* **AND** 存在三条对应的 `replace github.com/inferglow/<name> => ./../<name>` 指令

* **AND** `cd examples && go build ./...` 成功（忽略 `//go:build ignore` 文件）

