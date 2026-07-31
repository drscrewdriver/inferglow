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

// 示例：如何使用 context 模块管理对话上下文（选 Mode、Ingest、渲染、transient）
//
// 运行: go run example_context.go
package main

import (
	"context"
	"fmt"

	contextmgr "github.com/inferglow/context"
	"github.com/inferglow/context/store/jsonl"
)

func main() {
	ctx := context.Background()

	// 1. 准备后备存储（JSONL：零依赖、文件可读，适合本地开发）
	store, err := jsonl.New("./data", "example-session")
	if err != nil {
		fmt.Printf("create store failed: %v\n", err)
		return
	}

	// 2. 配置（用默认值，再按需覆盖）
	cfg := contextmgr.DefaultConfig()
	cfg.Mode = contextmgr.ModeHybrid // 完整 L0-L4 压缩 + RAG + 长时记忆
	cfg.WindowTokens = 128000

	// 3. 创建上下文管理器
	cm, err := contextmgr.NewHybridManager(cfg, store)
	if err != nil {
		fmt.Printf("create manager failed: %v\n", err)
		return
	}
	defer cm.Close()

	fmt.Printf("context mode: %s\n\n", cm.Mode())

	// 4. 记录步骤（Ingest 双写：存 L0 + 建引用）
	steps := []contextmgr.StepRecord{
		{Type: "user", Role: "user", Content: "请解释一下这个项目的架构"},
		{Type: "tool", Role: "tool", ToolName: "list_files", Content: `{"files":["README.md","main.go"]}`},
		{Type: "reasoning", Role: "assistant", Content: "项目采用四层架构：基础层/中间层/编排层/应用层"},
	}
	for _, s := range steps {
		if err := cm.Ingest(s); err != nil {
			fmt.Printf("ingest failed: %v\n", err)
			return
		}
	}

	// 5. 渲染上下文（供下一次 LLM 调用）
	blocks, err := cm.BuildContext(ctx, cfg.WindowTokens)
	if err != nil {
		fmt.Printf("build context failed: %v\n", err)
		return
	}
	fmt.Println("=== BuildContext 渲染结果 ===")
	for _, b := range blocks {
		fmt.Printf("[step=%d|level=%d] %s\n", b.StepID, b.Level, truncate(b.Content, 60))
	}

	// 6. 查询统计
	stats := cm.Stats()
	fmt.Printf("\nstats: totalSteps=%d activeSteps=%d totalTokens=%d\n",
		stats.TotalSteps, stats.ActiveSteps, stats.TotalTokens)

	// 7. transient：标记某步骤不进入上下文（如规划草稿）
	// MarkTransient 是 HybridManager 的具体方法，需类型断言
	if hm, ok := cm.(*contextmgr.HybridManager); ok {
		if err := hm.MarkTransient(3, "planning", 1); err != nil {
			fmt.Printf("mark transient failed: %v\n", err)
		} else {
			fmt.Println("\nstep 3 marked transient (excluded from context)")
		}
	}

	// 8. 检索与回溯
	hits, _ := cm.Search(ctx, contextmgr.SearchQuery{Query: "架构", Limit: 3})
	fmt.Printf("\nsearch '架构' hits: %d\n", len(hits))
	for _, h := range hits {
		fmt.Printf("  step=%d level=%d score=%.2f snippet=%s\n", h.StepID, h.Level, h.Score, truncate(h.Snippet, 40))
	}

	// 9. 展开某步骤原始内容
	if expanded, err := cm.Expand(1); err == nil {
		fmt.Printf("\nexpand step 1: level=%d content=%s\n", expanded.Level, truncate(expanded.Content, 60))
	}
}

// truncate 截断字符串以便控制台输出。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}