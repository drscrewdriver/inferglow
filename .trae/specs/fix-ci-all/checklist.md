# Checklist

- [x] CI 工作流 vet 根模块步骤有 `ls *.go` 守卫
- [x] CI 工作流 test 根模块步骤有 `ls *.go` 守卫
- [x] `gofmt -d` 对 11 个文件无 diff 输出
- [x] `workspace/context_source.go` 中 `WorkspaceContextSource` 已改名 + 别名
- [x] `workspace/context_source.go` 的别名有 `//nolint:revive` 抑制
- [x] `go vet ./...` 在各子模块通过（workspace 已本地验证，其余无改动）
- [ ] GitHub CI 全部 5 个 job 通过（等待 CI 完成）