- [x] 所有 14 个子模块执行 `gofmt -w` 后，`gofmt -l .`（含子模块）输出为空

- [x] action/local_executor.go、builtins/tools/schema_from_func.go、schema 模块中无 `reflect.Ptr` 用法（已替换为 `reflect.Pointer`）

- [x] 所有缺失注释的 exported 符号已补全文档注释（revive exported 无 "should have comment" 报错）

- [x] 所有 stuttering 类型名已添加 `//nolint:revive` 指令抑制（无未抑制的 stuttering 报错）

- [x] `.golangci.yaml` 未被修改（保留原有 gofmt/goimports/govet/revive 强约束）

- [x] 无任何 exported 符号被重命名（API 保持不变，无 BREAKING 变更）

- [x] 对每个含 `go.mod` 的子模块执行 `golangci-lint run --config=<root>/.golangci.yaml ./...` 返回 exit code 0

- [x] 对 14 个子模块执行 `go test ./...` 全部通过（注释/格式化/nolint 改动未破坏测试）

- [x] components 模块的 2 处具体问题已修复（adapt_action_test.go 的 gofmt、interface.go 的 ToolInfo stuttering）

- [x] `implement-p0-maturity-improvements` checklist 的 "`golangci-lint run` 无 error" 项经实际运行确认真正达成
