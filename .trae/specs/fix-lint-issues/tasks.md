# Tasks

- [ ] Task 1: 修复 gofmt 格式问题（3 个文件）
  - [ ] SubTask 1.1: 对 `flow/stage/builtin/stages.go` 运行 `gofmt -w`
  - [ ] SubTask 1.2: 对 `flow/port.go` 运行 `gofmt -w`
  - [ ] SubTask 1.3: 对 `flow/flow_context.go` 运行 `gofmt -w`
- [ ] Task 2: 修复 govet/inline 问题（`reflect.Ptr` → `reflect.Pointer`）
- [ ] Task 3: 修复缺少注释问题（2 个 const 块）
  - [ ] SubTask 3.1: `flow/stage/meta.go` const 块前添加注释
  - [ ] SubTask 3.2: `flow/port.go` const 块前添加注释
- [ ] Task 4: 修复 stutter 名称（添加 `//nolint:revive` 抑制，4 处）
  - [ ] SubTask 4.1: `flow/stage/registry.go` StageFunc 加 nolint
  - [ ] SubTask 4.2: `flow/stage/meta.go` StageMeta 加 nolint
  - [ ] SubTask 4.3: `flow/flow_context.go` FlowContext 和 FlowContextFrom 加 nolint
- [ ] Task 5: 验证修复结果
  - [ ] SubTask 5.1: 运行 `go test ./...` 确认功能不变
  - [ ] SubTask 5.2: 提交并推送至 GitHub 触发 CI

# Task Dependencies
- 无依赖，所有任务可并行执行