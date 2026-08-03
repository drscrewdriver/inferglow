# Tasks

- [x] Task 1: 修复 CI 工作流（vet/test 根模块守卫）
  - [x] SubTask 1.1: 在 `Run go vet (root module)` 步骤前加 `ls *.go 2>/dev/null` 守卫
  - [x] SubTask 1.2: 在 `Run go test (root module)` 步骤前加 `ls *.go 2>/dev/null` 守卫
- [x] Task 2: 修复 gofmt 格式问题（9 个文件，可并行运行 `gofmt -w`）
- [x] Task 3: 修复 `workspace/context_source.go` stutter（`WorkspaceContextSource` → `ContextSource` + 别名）
- [x] Task 4: 验证修复结果
  - [x] SubTask 4.1: 运行 `gofmt -d` 确认无格式问题（11 个文件无 diff）
  - [x] SubTask 4.2: 在子模块运行 `go vet ./...` 确认无 vet 问题（workspace 模块通过）
  - [x] SubTask 4.3: 提交并推送至 GitHub 触发 CI（7378892）

# Task Dependencies
- Task 2、3 无依赖，可并行执行