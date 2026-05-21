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

// 示例：如何使用 schema 模块推导和验证结构化输出
//
// 运行: go run example_schema.go
package main

import (
	"fmt"

	"github.com/inferglow/schema"
)

// 1. 定义一个输出结构体
type WeatherResponse struct {
	City      string     `json:"city" description:"城市名称"`
	Country   string     `json:"country"`
	Temp      float64    `json:"temp" description:"温度(摄氏度)"`
	Humidity  int        `json:"humidity,omitempty" description:"湿度(百分比)"`
	Forecasts []Forecast `json:"forecasts,omitempty"`
}

type Forecast struct {
	Date string  `json:"date" description:"日期"`
	Low  float64 `json:"low" description:"最低温度"`
	High float64 `json:"high" description:"最高温度"`
}

// 2. 定义更复杂的结构
type AnalysisResult struct {
	Summary  string          `json:"summary" description:"分析摘要"`
	Issues   []string        `json:"issues" description:"发现的问题"`
	Rating   int             `json:"rating" description:"评分 1-10"`
	Sections []SectionDetail `json:"sections,omitempty"`
}

type SectionDetail struct {
	Title   string  `json:"title"`
	Content string  `json:"content"`
	Weight  float64 `json:"weight"`
}

func main() {
	// --- 示例 1: 泛型方法推导 Schema ---
	fmt.Println("=== Example 1: Generic DefineOutput ===")

	weatherSchema := schema.DefineOutput[WeatherResponse]()
	fmt.Printf("WeatherResponse schema:\n")
	fmt.Printf("  EnsureAll: %v\n", weatherSchema.EnsureAll)
	for name, field := range weatherSchema.Fields {
		fmt.Printf("  Field '%s': Type=%s, Required=%v, Desc=%s\n",
			name, field.Type, field.Required, field.Description)
	}
	if len(weatherSchema.Fields["Forecasts"].Children) > 0 {
		fmt.Printf("  Nested (Forecasts[] items):\n")
		for n, f := range weatherSchema.Fields["Forecasts"].ItemDef.Children {
			fmt.Printf("    - %s: Type=%s\n", n, f.Type)
		}
	}
	fmt.Println()

	// --- 示例 2: 非泛型方法 ---
	fmt.Println("=== Example 2: Non-generic DefineOutputFromType ===")

	analysisSchema := schema.DefineOutput[AnalysisResult]()
	fmt.Printf("AnalysisResult schema:\n")
	for name, field := range analysisSchema.Fields {
		fmt.Printf("  Field '%s': Type=%s, Required=%v\n",
			name, field.Type, field.Required)
	}
	fmt.Println()

	// --- 示例 3: JSON Schema 转换 ---
	fmt.Println("=== Example 3: JSON Schema Conversion ===")

	// 从 OutputSchema 生成 JSON Schema
	jsonSchema := schema.GenerateJSONSchema(weatherSchema)
	fmt.Printf("JSON Schema for WeatherResponse:\n")
	fmt.Printf("  type: %v\n", jsonSchema["type"])
	fmt.Printf("  required: %v\n", jsonSchema["required"])
	props := jsonSchema["properties"].(map[string]any)
	for name, prop := range props {
		p := prop.(map[string]any)
		fmt.Printf("  properties.%s: type=%v", name, p["type"])
		if desc, ok := p["description"]; ok {
			fmt.Printf(", description=%v", desc)
		}
		fmt.Println()
	}
	fmt.Println()

	// --- 示例 4: 路径表达式解析 ---
	fmt.Println("=== Example 4: Path Expression Parsing ===")

	testPaths := []string{
		"city",
		"user.name",
		"items[*].id",
		"issues[*].severity",
	}
	for _, path := range testPaths {
		parts := schema.ParsePath(path)
		fmt.Printf("  '%s' -> %v\n", path, parts)
	}
	fmt.Println()

	// --- 示例 5: 手动构建 Schema ---
	fmt.Println("=== Example 5: Manual Schema Construction ===")

	customSchema := &schema.OutputSchema{
		Format:    schema.OutputJSON,
		EnsureAll: true,
		Fields: map[string]*schema.FieldDef{
			"message": {
				Type:        schema.TypeString,
				Description: "回复内容",
				Required:    true,
			},
			"code": {
				Type:        schema.TypeInt,
				Description: "错误码",
				Required:    false,
			},
		},
	}
	fmt.Printf("Custom schema: Format=%s, EnsureAll=%v\n",
		customSchema.Format, customSchema.EnsureAll)
	for name, field := range customSchema.Fields {
		fmt.Printf("  - %s: Type=%s, Required=%v\n",
			name, field.Type, field.Required)
	}
	fmt.Println()

	fmt.Println("=== All examples completed ===")
}
