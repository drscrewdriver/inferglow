# Checklist

- [ ] CI 工作流 vet 根模块步骤有 `ls *.go` 守卫
- [ ] CI 工作流 test 根模块步骤有 `ls *.go` 守卫
- [ ] `gofmt -d` 对 9 个文件无 diff 输出
- [ ] `workspace/context_source.go` 中 `WorkspaceContextSource` 已改名 + 别名
- [ ] `workspace/context_source.go` 的别名有 `//nolint:revive` 抑制
- [ ] `go vet ./...` 在各子模块通过
- [ ] GitHub CI 全部 5 个 job 通过