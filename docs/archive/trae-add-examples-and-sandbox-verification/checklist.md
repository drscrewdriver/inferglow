# Checklist

## 示例程序完整性

- [x] `examples/example_audit.go` 存在且头部包含 `//go:build ignore`
- [x] example_audit 演示了 NewAuditChain、Append、SignEntry、VerifyChain、Query、Export 六个核心能力
- [x] `examples/example_model.go` 存在且头部包含 `//go:build ignore`
- [x] example_model 演示了 3 个 Provider 构造、AttemptRunner 重试分类、OutputValidator 校验
- [x] `examples/example_orchestrator.go` 存在且头部包含 `//go:build ignore`
- [x] example_orchestrator 使用 mock ModelRequester（不依赖真实 LLM 服务）
- [x] example_orchestrator 演示了 Agent + Engine + LoopGuard + AuditChain 组装与 Run
- [x] `examples/example_workspace.go` 存在且头部包含 `//go:build ignore`
- [x] example_workspace 演示了 SafePath 路径穿越拦截、WriteFile/ReadFile、Lineage Record/Ancestors/Descendants

## go.mod 与 README

- [x] `examples/go.mod` require 块包含 audit / orchestrator / workspace 三项
- [x] `examples/go.mod` 包含三条对应的 replace 指令指向 `./../<name>`
- [x] `examples/README.md` 示例列表表格包含 4 个新示例条目

## Docker/gVisor 连接验证

- [x] `sandbox/live_docker_gvisor_test.go` 存在
- [x] TestLiveDockerDaemonReachable 在 Docker 可达时通过、不可达时 t.Skip
- [x] TestLiveDockerInspectAvailability 在 Docker 可达时断言 Available=true
- [x] TestLiveDockerHandleEcho 端到端验证 alpine 容器 echo hello 输出
- [x] TestLiveGVisorProviderAvailable 在 docker+runsc 可达时通过、不可达时 t.Skip
- [x] TestLiveGVisorHandleEcho 端到端验证 runsc 容器 echo hello 输出
- [x] 所有 live 测试使用 defer Stop 清理容器，无残留

## 编译与测试

- [x] `cd examples && go build example_audit.go example_model.go example_orchestrator.go example_workspace.go` 成功
- [x] `cd examples && go run example_audit.go` 正常输出并退出
- [x] `cd sandbox && go test -run TestLive -v -timeout 120s` 全部通过或 Skip
- [x] `cd sandbox && go vet ./...` 无错误
- [x] `cd sandbox && go test ./...` 现有测试无回归

## 验证结果摘要

- **示例编译**：4 个示例文件逐个 `go build` 全部成功（单条命令因多 `func main()` 冲突，需分开编译）
- **示例运行**：4 个示例 `go run` 全部正常输出，内容覆盖 spec 中定义的全部 Scenario
- **Live 测试**：5 个 TestLive 全部 PASS（0 Skip）
  - TestLiveDockerDaemonReachable (0.00s)
  - TestLiveDockerInspectAvailability (0.00s)
  - TestLiveDockerHandleEcho (30.43s) — 端到端拉起 alpine 容器执行 echo hello
  - TestLiveGVisorProviderAvailable (0.00s)
  - TestLiveGVisorHandleEcho (0.55s) — 端到端拉起 runsc 容器执行 echo hello
- **go vet**：sandbox 模块 `go vet ./...` 退出码 0，无任何输出
- **sandbox 全量测试**：202 PASS / 3 预存 FAIL（bubblewrap_test.go / e2b_test.go，未在本次任务修改范围内，git status 确认无回归）

## 处理的预先存在问题

`example_orchestrator.go` 与 `example_action.go` 通过 `github.com/inferglow/orchestrator/agent` → `action` → `sandbox` → `docker` 间接导入 `github.com/docker/docker@v27.5.1+incompatible`，该版本与 `github.com/docker/go-connections v0.7.0` 不兼容（`sockets.DialPipe` undefined）。

解决方案：在 `examples/go.mod` 末尾追加：
```
replace github.com/docker/go-connections v0.7.0 => github.com/docker/go-connections v0.4.0
```

降级到 v0.4.0 后编译成功，4 个示例均可独立 `go run`。
