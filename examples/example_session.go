// go:build ignore
//go:build ignore

// 示例：如何使用 session 模块管理对话记忆
//
// 运行: go run example_session.go
package main

import (
	"fmt"

	"github.com/inferglow/session"
)

func main() {
	// --- 示例 1: 创建 Session 并添加消息 ---
	fmt.Println("=== Example 1: Basic Session Usage ===")
	
	sess := session.NewSession("demo-session", 1000)
	fmt.Printf("Created session: ID=%s, MaxLength=%d\n", sess.ID, sess.MaxLength)
	fmt.Printf("FullContext: %d messages\n", len(sess.FullContext))
	fmt.Printf("ContextWindow: %d messages\n\n", len(sess.ContextWindow))

	// 添加消息
	sess.AddMessage("system", "You are a helpful assistant.", "")
	sess.AddMessage("user", "Hello! I need help with coding.", "")
	sess.AddMessage("assistant", "Of course! I'd be happy to help. What language are you using?", "")
	sess.AddMessage("user", "I'm working with Go.", "")
	sess.AddMessage("assistant", "Go is a great choice! What specific issue are you facing?", "")

	fmt.Printf("After 5 messages:\n")
	fmt.Printf("  FullContext: %d messages\n", len(sess.FullContext))
	fmt.Printf("  ContextWindow: %d messages\n\n", len(sess.ContextWindow))

	// 获取 Prompt
	prompt := sess.PreparePrompt()
	fmt.Printf("PreparePrompt() returns %d messages for LLM\n\n", len(prompt))

	// --- 示例 2: 上下文窗口自动裁剪 ---
	fmt.Println("=== Example 2: Context Window Auto-Resize ===")
	
	sess2 := session.NewSession("resize-demo", 200)
	sess2.AutoResize = true
	sess2.RegisterResizeHandler("simple_cut", session.SimpleCutResizeHandler)
	sess2.SetDefaultResizeHandler("simple_cut")

	for i := 1; i <= 20; i++ {
		sess2.AddMessage("user", fmt.Sprintf("Message %d: This is a test message with some content.", i), "")
		sess2.AddMessage("assistant", fmt.Sprintf("Reply %d: I received your message.", i), "")
	}
	fmt.Printf("Added 40 messages with MaxLength=200\n")
	fmt.Printf("FullContext: %d messages (never trimmed)\n", len(sess2.FullContext))
	fmt.Printf("ContextWindow: %d messages (auto-resized)\n\n", len(sess2.ContextWindow))

	// --- 示例 3: 摘要裁剪策略 ---
	fmt.Println("=== Example 3: Summary Resize Strategy ===")
	
	sess3 := session.NewSession("summary-demo", 300)
	sess3.AutoResize = true
	sess3.RegisterResizeHandler("summary_first", session.SummaryFirstResizeHandler)
	sess3.SetDefaultResizeHandler("summary_first")

	// 添加多条消息
	for i := 1; i <= 10; i++ {
		sess3.AddMessage("user", fmt.Sprintf("User message %d: Some conversation content here.", i), "")
		sess3.AddMessage("assistant", fmt.Sprintf("Assistant reply %d: Response to the above.", i), "")
	}
	
	fmt.Printf("Added 20 messages with MaxLength=300\n")
	fmt.Printf("FullContext: %d messages\n", len(sess3.FullContext))
	fmt.Printf("ContextWindow: %d messages\n", len(sess3.ContextWindow))
	
	// 打印 ContextWindow 内容摘要
	fmt.Println("ContextWindow contents:")
	for i, msg := range sess3.ContextWindow {
		content := fmt.Sprintf("%v", msg.Content)
		if len(content) > 50 {
			content = content[:50] + "..."
		}
		fmt.Printf("  [%d] %s: %s\n", i, msg.Role, content)
	}
	fmt.Println()

	// --- 示例 4: Token 感知裁剪 ---
	fmt.Println("=== Example 4: Token-Aware Resize ===")
	
	sess4 := session.NewSession("token-demo", 8000)
	sess4.AutoResize = true
	sess4.RegisterResizeHandler("token_aware", session.TokenAwareResizeHandler)
	sess4.SetDefaultResizeHandler("token_aware")

	for i := 1; i <= 15; i++ {
		sess4.AddMessage("user", fmt.Sprintf("Longer message %d with more text content to simulate token usage in a real conversation context.", i), "")
		sess4.AddMessage("assistant", fmt.Sprintf("Response %d: Acknowledging the longer input with equally detailed reply text.", i), "")
	}
	
	fmt.Printf("Added 30 messages with MaxLength=8000 (token aware)\n")
	fmt.Printf("FullContext: %d messages\n", len(sess4.FullContext))
	fmt.Printf("ContextWindow: %d messages (token-aware trimmed)\n\n", len(sess4.ContextWindow))

	// --- 示例 5: 持久化 ---
	fmt.Println("=== Example 5: Persistence ===")
	
	sess5 := session.NewSession("persist-demo", 1000)
	sess5.AddMessage("user", "Hello!", "")
	sess5.AddMessage("assistant", "Hi there!", "")

	jsonStr, err := sess5.ToJSON()
	if err != nil {
		fmt.Printf("Error generating JSON: %v\n", err)
	} else {
		fmt.Printf("Generated JSON (first 100 chars):\n  %s...\n\n", jsonStr[:100])
	}

	yamlStr, err := sess5.ToYAML()
	if err != nil {
		fmt.Printf("Error generating YAML: %v\n", err)
	} else {
		fmt.Printf("Generated YAML:\n%s\n", yamlStr)
	}

	fmt.Println("=== All examples completed ===")
}
