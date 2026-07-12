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

// 示例：如何使用 action 模块注册和调用函数
//
// 运行: go run example_action.go
package main

import (
	"context"
	"fmt"

	"github.com/inferglow/action"
)

// 1. 定义一个普通 Go 函数
type AddRequest struct {
	A int `json:"a"`
	B int `json:"b"`
}

func addNumbers(ctx context.Context, req AddRequest) (int, error) {
	return req.A + req.B, nil
}

// 2. 定义另一个函数（不同的签名：func(InputT) (OutputT, error)）
type GreetRequest struct {
	Name string `json:"name"`
}

func greet(req GreetRequest) (string, error) {
	if req.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	return "Hello, " + req.Name + "!", nil
}

// 3. 带 error 返回的函数
type GreetTitleRequest struct {
	Name  string `json:"name"`
	Title string `json:"title,omitempty"`
}

func greetWithTitleFn(req GreetTitleRequest) (string, error) {
	if req.Name == "" {
		return "", fmt.Errorf("name is required")
	}
	if req.Title != "" {
		return fmt.Sprintf("Hello, %s %s!", req.Title, req.Name), nil
	}
	return fmt.Sprintf("Hello, %s!", req.Name), nil
}

func main() {
	ctx := context.Background()

	// --- 方式 1: 使用 New() 自动包装 ---
	// New() 会自动:
	// 1. 从函数签名推导 JSON Schema
	// 2. 创建 LocalFunctionExecutor
	// 3. 生成 Action 对象

	addAction, err := action.New("add", "Add two numbers together", addNumbers)
	if err != nil {
		fmt.Printf("Failed to create add action: %v\n", err)
		return
	}
	fmt.Printf("Created action: %s - %s\n", addAction.Name, addAction.Description)
	fmt.Printf("Schema: %+v\n\n", addAction.Schema)

	// --- 方式 2: 手动构建 Action ---
	greetAction, err := action.New("greet", "Greet someone", greet)
	if err != nil {
		fmt.Printf("Failed to create greet action: %v\n", err)
		return
	}

	// --- 方式 3: 用 New() 自动包装另一函数 ---
	greetWithTitleAction, _ := action.New("greet_with_title", "Greet with title", greetWithTitleFn)

	// --- 注册到 Registry ---
	registry := action.NewRegistry()

	for _, a := range []*action.Action{addAction, greetAction, greetWithTitleAction} {
		if err := registry.Register(a); err != nil {
			fmt.Printf("Failed to register %s: %v\n", a.Name, err)
			return
		}
	}

	fmt.Printf("Registered %d actions: %v\n\n", len(registry.List()), registry.List())

	// --- 执行 Action ---
	fmt.Println("=== Execute 'add' action ===")
	result, _ := registry.Execute(ctx, "add", map[string]any{
		"a": 10,
		"b": 20,
	})
	fmt.Printf("Result OK=%v, Status=%s, Result=%+v\n\n", result.OK, result.Status, result.Result)

	fmt.Println("=== Execute 'greet' action ===")
	result, _ = registry.Execute(ctx, "greet", map[string]any{
		"name": "World",
	})
	fmt.Printf("Result OK=%v, Status=%s, Result=%+v\n\n", result.OK, result.Status, result.Result)

	fmt.Println("=== Execute 'greet_with_title' action ===")
	result, _ = registry.Execute(ctx, "greet_with_title", map[string]any{
		"name":  "Joshua",
		"title": "Mr.",
	})
	fmt.Printf("Result OK=%v, Status=%s, Result=%+v\n\n", result.OK, result.Status, result.Result)

	// --- 错误处理 ---
	fmt.Println("=== Execute with invalid input ===")
	result, _ = registry.Execute(ctx, "greet_with_title", map[string]any{})
	fmt.Printf("Result OK=%v, Status=%s, Error=%s\n", result.OK, result.Status, result.Error)
}
