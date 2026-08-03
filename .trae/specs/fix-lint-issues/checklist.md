# Checklist

- [x] gofmt 修复后 `flow/stage/builtin/stages.go` 格式正确
- [x] gofmt 修复后 `flow/port.go` 格式正确
- [x] gofmt 修复后 `flow/flow_context.go` 格式正确
- [x] `flow/flowdef/expr.go` 中 `reflect.Ptr` 替换为 `reflect.Pointer`
- [x] `flow/stage/meta.go` const 块前有注释
- [x] `flow/port.go` const 块前有注释
- [x] `flow/stage/registry.go` StageFunc 声明有 `//nolint:revive`
- [x] `flow/stage/meta.go` StageMeta 声明有 `//nolint:revive`
- [x] `flow/flow_context.go` FlowContext 和 FlowContextFrom 有 `//nolint:revive`
- [x] `go test ./...` 全部通过（flow 子模块 3 个包 OK）
- [ ] GitHub CI lint 步骤完全变绿（等待 CI 运行）