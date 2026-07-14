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

// 示例：如何使用 middleware 构建洋葱模型中间件链
//
// 运行: go run example_middleware.go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/inferglow/orchestrator/middleware"
)

func main() {
	// --- 核心 handler：原样返回 input 中的消息 ---
	echoHandler := func(ctx context.Context, input *middleware.Input) (*middleware.Output, error) {
		return &middleware.Output{
			Messages: input.Messages,
			Metadata: map[string]any{"echo": "ok"},
		}, nil
	}

	// --- 中间件 1: 日志中间件（最外层）---
	// 在消息内容前添加 "[LOG]" 前缀，演示洋葱模型的外层先执行。
	loggingMiddleware := func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, input *middleware.Input) (*middleware.Output, error) {
			fmt.Println("  [logging]  before → 外层日志中间件进入")
			output, err := next(ctx, input)
			fmt.Println("  [logging]  after  → 外层日志中间件退出")
			if err != nil {
				return nil, err
			}
			for i := range output.Messages {
				output.Messages[i].Content = "[LOG] " + output.Messages[i].Content
			}
			return output, nil
		}
	}

	// --- 中间件 2: 耗时统计中间件（中间层）---
	timingMiddleware := func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, input *middleware.Input) (*middleware.Output, error) {
			fmt.Println("    [timing]  before → 耗时统计中间件进入")
			start := time.Now()
			output, err := next(ctx, input)
			elapsed := time.Since(start)
			fmt.Printf("    [timing]  after  → 耗时统计中间件退出 (耗时: %v)\n", elapsed)
			if err != nil {
				return nil, err
			}
			if output.Metadata == nil {
				output.Metadata = make(map[string]any)
			}
			output.Metadata["elapsed"] = elapsed.String()
			return output, nil
		}
	}

	// --- 中间件 3: 元数据中间件（最内层）---
	metadataMiddleware := func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, input *middleware.Input) (*middleware.Output, error) {
			fmt.Println("      [metadata] before → 元数据中间件进入")
			output, err := next(ctx, input)
			fmt.Println("      [metadata] after  → 元数据中间件退出")
			if err != nil {
				return nil, err
			}
			if output.Metadata == nil {
				output.Metadata = make(map[string]any)
			}
			output.Metadata["middleware_count"] = 3
			output.Metadata["pipeline"] = "onion-model"
			return output, nil
		}
	}

	// --- 组合中间件链 ---
	// Chain 中第一个参数是最外层，最后一个是最内层。
	chain := middleware.Chain(loggingMiddleware, timingMiddleware, metadataMiddleware)
	wrappedHandler := chain(echoHandler)

	// --- 执行 ---
	fmt.Println("=== 洋葱模型中间件链示例 ===")
	fmt.Println()

	ctx := context.Background()
	input := &middleware.Input{
		Messages: []middleware.Message{
			{Role: "user", Content: "Hello, middleware!"},
		},
		SessionID: "demo-session",
		Metadata:  map[string]any{"request_id": "req-001"},
	}

	fmt.Println("执行中间件链:")
	fmt.Println("  (洋葱模型: logging → timing → metadata → echo)")
	fmt.Println()

	output, err := wrappedHandler(ctx, input)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("=== 执行结果 ===")
	for _, msg := range output.Messages {
		fmt.Printf("  Role:    %s\n", msg.Role)
		fmt.Printf("  Content: %s\n", msg.Content)
	}
	fmt.Println()
	fmt.Println("  Metadata:")
	for k, v := range output.Metadata {
		fmt.Printf("    %s: %v\n", k, v)
	}
}