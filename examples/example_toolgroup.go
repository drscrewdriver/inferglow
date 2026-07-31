// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

// go:build ignore
//go:build ignore

// 示例：如何使用 ToolGroup / GroupRegistry 按组组织、列举、过滤工具
//
// 运行: go run example_toolgroup.go
package main

import (
	"context"
	"fmt"

	"github.com/inferglow/action"
)

// 1. 定义若干普通 Go 函数（只读 / 写 / 执行）
//    action.New 要求函数带一个输入参数，故各定义一个请求结构体。
type LsRequest struct{ Dir string `json:"dir"` }
type CatRequest struct{ Path string `json:"path"` }
type WriteRequest struct{ Path string `json:"path"` }
type RunRequest struct{ Cmd string `json:"cmd"` }

func listFiles(req LsRequest) (string, error)   { return "README.md\nmain.go", nil }
func catFile(req CatRequest) (string, error)     { return "file content...", nil }
func writeFile(req WriteRequest) (string, error) { return "written", nil }
func runCommand(req RunRequest) (string, error)  { return "executed", nil }

func main() {
	ctx := context.Background()

	// 2. 用 action.New 包装为 Action
	ls, _ := action.New("ls", "List files", listFiles)
	cat, _ := action.New("cat", "Show file content", catFile)
	write, _ := action.New("write_file", "Write a file", writeFile)
	run, _ := action.New("run", "Run a shell command", runCommand)

	// 3. 注册到 ActionRegistry（扁平注册）
	registry := action.NewRegistry()
	for _, a := range []*action.Action{ls, cat, write, run} {
		if err := registry.Register(a); err != nil {
			fmt.Printf("register %s failed: %v\n", a.Name, err)
			return
		}
	}

	// 4. 用保留标签约定 group:<name> 给动作打组标签
	registry.Tag([]string{"ls", "cat"}, []string{"group:readonly", "readonly"})
	registry.Tag([]string{"write_file"}, []string{"group:write", "write"})
	registry.Tag([]string{"run"}, []string{"group:exec", "exec"})

	// 5. 创建 GroupRegistry（派生视图，不复制数据）
	gr := action.NewGroupRegistry(registry)

	// 6. 注册命名组
	_ = gr.Register(&action.ToolGroup{
		Name:        "readonly",
		Description: "只读工具组",
		Tags:        []string{"group:readonly"},
		Policy:      &action.GroupPolicy{ReadOnly: true, MaxLevel: action.SideEffectRead},
	})
	_ = gr.Register(&action.ToolGroup{
		Name:        "plan",
		Description: "plan 模式可用工具（只读）",
		Tags:        []string{"group:readonly"},
	})

	// 7. 按组列举
	readonlyNames, _ := gr.ListActionNames("readonly")
	fmt.Printf("readonly group members: %v\n", readonlyNames)

	fmt.Printf("all groups: %v\n", gr.List())
	fmt.Printf("HasAction(readonly, ls) = %v\n", gr.HasAction("readonly", "ls"))
	fmt.Printf("HasAction(readonly, run) = %v\n\n", gr.HasAction("readonly", "run"))

	// 8. 组合 ToolFilter 做请求期过滤（plan 模式 = 只读组 + ReadOnlyProfile）
	filter := action.ReadOnlyProfile() // MaxLevel = SideEffectRead
	fmt.Println("=== plan 模式：只读组 + ReadOnlyProfile ===")
	for _, name := range readonlyNames {
		if filter.IsAllowed(name, nil) {
			fmt.Printf("  allowed: %s\n", name)
		}
	}

	// 9. 注销组
	if gr.Unregister("plan") {
		fmt.Println("\nplan group unregistered")
	}
	fmt.Printf("remaining groups: %v\n", gr.List())

	// 10. 组内工具仍可正常执行（执行路径不变）
	result, _ := registry.Execute(ctx, "ls", map[string]any{"dir": "."})
	fmt.Printf("\nexecute ls: OK=%v Result=%+v\n", result.OK, result.Result)
}