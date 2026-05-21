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

// 示例：如何使用 audit 模块构建防篡改审计链
//
// 运行: go run example_audit.go
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/inferglow/audit"
)

func main() {
	// 用于 HMAC-SHA256 签名/验证的密钥
	signatureKey := []byte("secret-key-12345")

	// --- 示例 1: 创建 AuditChain ---
	fmt.Println("=== Example 1: 创建 AuditChain ===")

	chain, err := audit.NewAuditChain(audit.AuditConfig{
		Enabled:        true,
		SignatureKey:   signatureKey,
		StorageBackend: "memory",
		MaxEntries:     100,
	})
	if err != nil {
		fmt.Printf("NewAuditChain 失败: %v\n", err)
		return
	}
	fmt.Printf("IsEnabled: %v\n", chain.IsEnabled())
	fmt.Printf("Len: %d\n\n", chain.Len())

	// --- 示例 2: 追加审计条目 ---
	fmt.Println("=== Example 2: 追加审计条目 ===")

	entriesToAppend := []*audit.AuditEntry{
		{
			Source:   "agent",
			Action:   "decision",
			Input:    "用户请求天气",
			Output:   map[string]any{"next_action": "execute"},
			Metadata: map[string]string{"round": "1"},
		},
		{
			Source:   "action",
			Action:   "execute",
			Input:    map[string]any{"name": "weather_api"},
			Output:   map[string]any{"success": true},
			Duration: 150 * time.Millisecond,
		},
		{
			Source: "model",
			Action: "request",
			Input:  "generate response",
			Output: "今天晴天",
		},
		{
			Source: "agent",
			Action: "decision",
			Input:  "生成最终回复",
			Output: map[string]any{"next_action": "response"},
		},
	}

	for i, e := range entriesToAppend {
		hash, err := chain.Append(e)
		if err != nil {
			fmt.Printf("Append[%d] 失败: %v\n", i, err)
			continue
		}
		shortHash := hash
		if len(shortHash) > 16 {
			shortHash = shortHash[:16]
		}
		fmt.Printf("Append[%d] hash=%s... chain.Len()=%d\n", i, shortHash, chain.Len())
	}
	fmt.Println()

	// --- 示例 3: 签名与验证 ---
	fmt.Println("=== Example 3: 签名与验证 ===")

	allEntries, err := chain.Query(audit.QueryFilter{})
	if err != nil {
		fmt.Printf("Query 失败: %v\n", err)
		return
	}
	if len(allEntries) == 0 {
		fmt.Println("没有查询到条目")
		return
	}
	lastEntry := allEntries[len(allEntries)-1]

	// 验证原始条目签名是否有效
	valid := audit.VerifyEntry(lastEntry, signatureKey)
	fmt.Printf("原始条目签名验证: %v\n", valid)

	// 演示篡改：复制一份后再修改 Hash，避免破坏链中原始条目
	tampered := *lastEntry
	tampered.Hash = "tampered"
	tamperedValid := audit.VerifyEntry(&tampered, signatureKey)
	fmt.Printf("篡改 Hash 后签名验证: %v\n\n", tamperedValid)

	// --- 示例 4: 全链验证 ---
	fmt.Println("=== Example 4: 全链验证 ===")

	if err := chain.VerifyChain(); err != nil {
		if ve, ok := err.(*audit.VerifyError); ok {
			fmt.Printf("VerifyChain: FAILED (index=%d reason=%s)\n", ve.Index, ve.Reason)
		} else {
			fmt.Printf("VerifyChain: FAILED (%v)\n", err)
		}
	} else {
		fmt.Println("VerifyChain: PASSED")
	}
	fmt.Println()

	// --- 示例 5: 查询过滤 ---
	fmt.Println("=== Example 5: 查询过滤 ===")

	agentEntries, err := chain.Query(audit.QueryFilter{Source: "agent"})
	if err != nil {
		fmt.Printf("Query(Source=agent) 失败: %v\n", err)
	} else {
		fmt.Printf("Query(Source=agent) 命中 %d 条:\n", len(agentEntries))
		for _, e := range agentEntries {
			fmt.Printf("  ID=%s Action=%s\n", e.ID, e.Action)
		}
	}

	executeEntries, err := chain.Query(audit.QueryFilter{Action: "execute"})
	if err != nil {
		fmt.Printf("Query(Action=execute) 失败: %v\n", err)
	} else {
		fmt.Printf("Query(Action=execute) 命中 %d 条:\n", len(executeEntries))
		for _, e := range executeEntries {
			fmt.Printf("  ID=%s Action=%s\n", e.ID, e.Action)
		}
	}
	fmt.Println()

	// --- 示例 6: 导出 ---
	fmt.Println("=== Example 6: 导出 ===")

	fmt.Println("--- ExportJSON ---")
	if err := chain.Export(audit.ExportJSON, os.Stdout); err != nil {
		fmt.Printf("Export(JSON) 失败: %v\n", err)
	}

	fmt.Println("--- ExportCSV ---")
	if err := chain.Export(audit.ExportCSV, os.Stdout); err != nil {
		fmt.Printf("Export(CSV) 失败: %v\n", err)
	}

	fmt.Println("--- ExportText ---")
	if err := chain.Export(audit.ExportText, os.Stdout); err != nil {
		fmt.Printf("Export(Text) 失败: %v\n", err)
	}

	fmt.Println("=== All examples completed ===")
}
