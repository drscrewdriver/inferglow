# Checklist

- [ ] gofmt 修复后 `flow/stage/builtin/stages.go` 格式正确
- [ ] gofmt 修复后 `flow/port.go` 格式正确
- [ ] gofmt 修复后 `flow/flow_context.go` 格式正确
- [ ] `flow/flowdef/expr.go` 中 `reflect.Ptr` 替换为 `reflect.Pointer`
- [ ] `flow/stage/meta.go` const 块前有注释
- [ ] `flow/port.go` const 块前有注释
- [ ] `flow/stage/registry.go` StageFunc 声明有 `//nolint:revive`
- [ ] `flow/stage/meta.go` StageMeta 声明有 `//nolint:revive`
- [ ] `flow/flow_context.go` FlowContext 和 FlowContextFrom 有 `//nolint:revive`
- [ ] `go test ./...` 全部通过
- [ ] GitHub CI lint 步骤完全变绿（无 error annotations）