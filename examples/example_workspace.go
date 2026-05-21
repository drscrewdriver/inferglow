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

//go:build ignore

// 示例：如何使用 workspace 模块进行安全文件 IO 与血缘管理
//
// 运行: go run example_workspace.go
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/inferglow/workspace"
)

func main() {
	// --- 示例 1: 创建 Workspace ---
	fmt.Println("=== Example 1: 创建 Workspace ===")

	tmpDir := filepath.Join(os.TempDir(), "workspace-demo")
	os.RemoveAll(tmpDir) // 清理上一次运行残留
	defer os.RemoveAll(tmpDir)

	ws, err := workspace.New(workspace.Config{
		RootDir:      tmpDir,
		MaxFileSize:  1024 * 1024, // 1MB
		MaxFileCount: 100,
	})
	if err != nil {
		fmt.Printf("创建 Workspace 失败: %v\n", err)
		return
	}
	fmt.Printf("Root():        %s\n", ws.Root())
	fmt.Printf("MaxFileSize:   %d bytes\n", ws.Config().MaxFileSize)
	fmt.Printf("MaxFileCount:  %d\n", ws.Config().MaxFileCount)
	fmt.Println()

	// --- 示例 2: 路径穿越防护 ---
	fmt.Println("=== Example 2: 路径穿越防护 ===")

	paths := []string{
		"data/file.txt",      // 合法
		"../../etc/passwd",   // 穿越 - 应失败
		"subdir/../file.txt", // 清洗后合法
		"/etc/passwd",        // 绝对路径 - 应失败
	}
	for _, p := range paths {
		abs, err := ws.SafePath(p)
		if err != nil {
			fmt.Printf("  [拒绝] %-25q -> 错误: %v\n", p, err)
			if errors.Is(err, workspace.ErrPathOutsideRoot) {
				fmt.Println("         (检测到路径穿越，已拦截)")
			}
			continue
		}
		fmt.Printf("  [通过] %-25q -> %s\n", p, abs)
	}
	fmt.Println()

	// --- 示例 3: 安全文件 IO ---
	fmt.Println("=== Example 3: 安全文件 IO ===")

	if err := ws.MkdirAll("data/input"); err != nil {
		fmt.Printf("MkdirAll 失败: %v\n", err)
		return
	}
	if err := ws.WriteFile("data/input/raw.txt", []byte("raw data\n")); err != nil {
		fmt.Printf("WriteFile raw.txt 失败: %v\n", err)
		return
	}
	if err := ws.WriteFile("data/processed.txt", []byte("processed data\n")); err != nil {
		fmt.Printf("WriteFile processed.txt 失败: %v\n", err)
		return
	}

	data, err := ws.ReadFile("data/processed.txt")
	if err != nil {
		fmt.Printf("ReadFile 失败: %v\n", err)
		return
	}
	fmt.Printf("ReadFile(\"data/processed.txt\"): %q\n", string(data))

	files, err := ws.ListDir("data")
	if err != nil {
		fmt.Printf("ListDir 失败: %v\n", err)
		return
	}
	fmt.Printf("ListDir(\"data\"): %v\n", files)

	count, err := ws.FileCount()
	if err != nil {
		fmt.Printf("FileCount 失败: %v\n", err)
		return
	}
	fmt.Printf("FileCount(): %d\n", count)

	// 演示只读模式：创建第二个 Workspace 指向同一根目录
	roWs, err := workspace.New(workspace.Config{
		RootDir:  tmpDir,
		ReadOnly: true,
	})
	if err != nil {
		fmt.Printf("创建只读 Workspace 失败: %v\n", err)
		return
	}
	writeErr := roWs.WriteFile("data/should-fail.txt", []byte("nope\n"))
	fmt.Printf("只读模式 WriteFile 返回错误: %v\n", writeErr)
	if errors.Is(writeErr, workspace.ErrReadOnly) {
		fmt.Println("  (符合预期: ErrReadOnly)")
	}
	fmt.Println()

	// --- 示例 4: 文件血缘管理 ---
	fmt.Println("=== Example 4: 文件血缘管理 ===")

	store := workspace.NewMemoryLineageStore()

	// raw.txt (无父节点 - 根源数据源)
	if err := store.Record(workspace.LineageNode{
		Path:      "data/input/raw.txt",
		Operation: "write",
		CreatedBy: "ingest-tool",
	}); err != nil {
		fmt.Printf("Record raw.txt 失败: %v\n", err)
		return
	}

	// cleaned.txt (由 raw.txt 衍生)
	if err := store.Record(workspace.LineageNode{
		Path:      "data/cleaned.txt",
		Operation: "transform",
		CreatedBy: "cleaner",
		Parents:   []string{"data/input/raw.txt"},
		Metadata:  map[string]any{"filter": "remove-nulls"},
	}); err != nil {
		fmt.Printf("Record cleaned.txt 失败: %v\n", err)
		return
	}

	// report.txt (由 cleaned.txt 衍生)
	if err := store.Record(workspace.LineageNode{
		Path:      "data/report.txt",
		Operation: "transform",
		CreatedBy: "reporter",
		Parents:   []string{"data/cleaned.txt"},
	}); err != nil {
		fmt.Printf("Record report.txt 失败: %v\n", err)
		return
	}

	// summary.txt (由 report.txt AND cleaned.txt 衍生)
	if err := store.Record(workspace.LineageNode{
		Path:      "data/summary.txt",
		Operation: "aggregate",
		CreatedBy: "summarizer",
		Parents:   []string{"data/report.txt", "data/cleaned.txt"},
	}); err != nil {
		fmt.Printf("Record summary.txt 失败: %v\n", err)
		return
	}

	fmt.Printf("store.Size(): %d (应为 4)\n", store.Size())

	ancestors, err := store.Ancestors("data/summary.txt")
	if err != nil {
		fmt.Printf("Ancestors 失败: %v\n", err)
		return
	}
	fmt.Printf("Ancestors(\"data/summary.txt\"): %v\n", ancestors)
	fmt.Println("  (应包含 report.txt, cleaned.txt, raw.txt)")

	descendants, err := store.Descendants("data/input/raw.txt")
	if err != nil {
		fmt.Printf("Descendants 失败: %v\n", err)
		return
	}
	fmt.Printf("Descendants(\"data/input/raw.txt\"): %v\n", descendants)
	fmt.Println("  (应包含 cleaned.txt, report.txt, summary.txt)")

	children, err := store.Children("data/cleaned.txt")
	if err != nil {
		fmt.Printf("Children 失败: %v\n", err)
		return
	}
	fmt.Printf("Children(\"data/cleaned.txt\"): %v (应为 report.txt, summary.txt)\n", children)

	parents, err := store.Parents("data/summary.txt")
	if err != nil {
		fmt.Printf("Parents 失败: %v\n", err)
		return
	}
	fmt.Printf("Parents(\"data/summary.txt\"): %v (应为 report.txt, cleaned.txt)\n", parents)

	// 环检测演示：重新记录 raw.txt 并指向 summary.txt
	// 会形成环: raw -> cleaned -> report -> summary -> raw
	cycleErr := store.Record(workspace.LineageNode{
		Path:    "data/input/raw.txt",
		Parents: []string{"data/summary.txt"},
	})
	fmt.Printf("环检测 Record 返回错误: %v\n", cycleErr)
	if errors.Is(cycleErr, workspace.ErrLineageCycle) {
		fmt.Println("  (符合预期: ErrLineageCycle)")
	}
	fmt.Println()

	// --- 示例 5: 血缘持久化 ---
	fmt.Println("=== Example 5: 血缘持久化 ===")

	// 将血缘保存到 workspace 内的 sidecar 文件
	lineagePath := filepath.Join(ws.Root(), "lineage.json")
	if err := workspace.SaveLineageToFile(store, lineagePath); err != nil {
		fmt.Printf("SaveLineageToFile 失败: %v\n", err)
		return
	}
	fmt.Printf("已保存血缘到: %s\n", lineagePath)

	exists, err := ws.Exists("lineage.json")
	if err != nil {
		fmt.Printf("Exists 失败: %v\n", err)
		return
	}
	fmt.Printf("ws.Exists(\"lineage.json\"): %v (应为 true)\n", exists)

	// 从文件加载回新的 store
	loaded, err := workspace.LoadLineageFromFile(lineagePath)
	if err != nil {
		fmt.Printf("LoadLineageFromFile 失败: %v\n", err)
		return
	}
	fmt.Printf("loaded.Size() == store.Size(): %v (均为 %d)\n", loaded.Size() == store.Size(), loaded.Size())

	loadedAncestors, err := loaded.Ancestors("data/summary.txt")
	if err != nil {
		fmt.Printf("loaded.Ancestors 失败: %v\n", err)
		return
	}
	fmt.Printf("loaded.Ancestors(\"data/summary.txt\"): %v\n", loadedAncestors)
	fmt.Printf("与原始 ancestors 内容一致: %v\n", equalStringSet(ancestors, loadedAncestors))
	fmt.Println()

	fmt.Println("=== All examples completed ===")
}

// equalStringSet 比较两个字符串切片是否包含相同元素集合（顺序无关）。
func equalStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]int, len(a))
	for _, s := range a {
		set[s]++
	}
	for _, s := range b {
		set[s]--
		if set[s] < 0 {
			return false
		}
	}
	return true
}
