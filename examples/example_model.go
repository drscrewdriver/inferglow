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

// 示例：如何使用 model 模块构造 Provider、重试与 Schema 校验
//
// 运行: go run example_model.go
package main

import (
	"context"
	"fmt"

	"github.com/inferglow/model"
)

func main() {
	ctx := context.Background()

	// --- 示例 1: 配置加载与 Provider 构造 ---
	fmt.Println("=== Example 1: 配置加载与 Provider 构造 ===")

	cp := &model.StaticConfigProvider{Values: map[string]any{
		"openai":    map[string]any{"api_key": "sk-demo", "model": "gpt-4", "base_url": "https://api.openai.com/v1"},
		"anthropic": map[string]any{"api_key": "sk-ant-demo", "model": "claude-3-5-sonnet-20241022"},
		"ollama":    map[string]any{"api_key": "dummy", "model": "llama3", "base_url": "http://localhost:11434"},
	}}

	// OpenAI 兼容 Provider
	openaiProvider, err := model.NewOpenAIProviderFromConfig(cp)
	if err != nil {
		fmt.Printf("  OpenAI provider 构造失败: %v\n", err)
	} else {
		fmt.Printf("  [OpenAI]      Name=%s, BaseURL=%s, Model=%s\n",
			openaiProvider.Name(), openaiProvider.BaseURL, openaiProvider.Model)
	}

	// Anthropic 兼容 Provider
	anthropicProvider, err := model.NewAnthropicProviderFromConfig(cp)
	if err != nil {
		fmt.Printf("  Anthropic provider 构造失败: %v\n", err)
	} else {
		fmt.Printf("  [Anthropic]   Name=%s, BaseURL=%s, Model=%s\n",
			anthropicProvider.Name(), anthropicProvider.BaseURL, anthropicProvider.Model)
	}

	// Ollama Provider：model 包未提供 NewOllamaProviderFromConfig 工厂，
	// 这里通过 LoadProviderConfig 加载配置后直接构造 OllamaProvider
	// （与 OpenAI/Anthropic 工厂内部逻辑一致）。
	// 注意：LoadProviderConfig 要求 api_key 非空，即使 Ollama 本身不需要 key，
	// 因此静态配置中填入 "dummy" 占位。
	ollamaCfg, err := model.LoadProviderConfig(cp, "ollama")
	if err != nil {
		fmt.Printf("  Ollama 配置加载失败: %v\n", err)
	} else {
		ollamaProvider := &model.OllamaProvider{
			BaseURL:    ollamaCfg.BaseURL,
			APIKey:     ollamaCfg.APIKey,
			Model:      ollamaCfg.Model,
			HTTPClient: ollamaCfg.HTTPClient,
		}
		fmt.Printf("  [Ollama]      Name=%s, BaseURL=%s, Model=%s\n",
			ollamaProvider.Name(), ollamaProvider.BaseURL, ollamaProvider.Model)
	}

	// Ollama 直接构造器：无需配置，默认 BaseURL=http://localhost:11434
	directOllama := model.NewOllamaProvider()
	fmt.Printf("  [Ollama(直接)] Name=%s, BaseURL=%s, Model=%q\n",
		directOllama.Name(), directOllama.BaseURL, directOllama.Model)
	fmt.Println()

	// --- 示例 2: ModelRequest 构造与 GenerateRequestData 转换 ---
	fmt.Println("=== Example 2: ModelRequest 构造与 GenerateRequestData 转换 ===")

	req := &model.ModelRequest{
		System: "你是一个严谨的天气助手，必须以 JSON 形式回复。",
		ChatHistory: []model.ChatMessage{
			{Role: "user", Content: "北京今天天气怎么样？"},
			{Role: "assistant", Content: `{"city":"北京","temp":26}`},
		},
		Tools: []model.ToolDefinition{
			{
				Name:        "get_weather",
				Description: "查询指定城市的实时天气",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
					"required": []string{"city"},
				},
			},
		},
		Output: &model.OutputSchema{
			Type: "object",
			Properties: map[string]any{
				"city": map[string]any{"type": "string"},
				"temp": map[string]any{"type": "number"},
			},
			Required: []string{"city", "temp"},
		},
		Model:       "gpt-4",
		Temperature: 0.2,
	}

	fmt.Printf("  ModelRequest:\n")
	fmt.Printf("    System=%q\n", req.System)
	fmt.Printf("    ChatHistory 条数=%d\n", len(req.ChatHistory))
	fmt.Printf("    Tools 条数=%d (首个: %s)\n", len(req.Tools), req.Tools[0].Name)
	fmt.Printf("    Output.Type=%s, Required=%v\n", req.Output.Type, req.Output.Required)

	// GenerateRequestData 是本地转换（构造请求体结构），不会发起网络请求。
	if openaiProvider != nil {
		data, err := openaiProvider.GenerateRequestData(ctx, req)
		if err != nil {
			fmt.Printf("  GenerateRequestData 失败: %v\n", err)
		} else {
			fmt.Printf("  RequestData (OpenAI 转换结果):\n")
			fmt.Printf("    Model=%s\n", data.Model)
			fmt.Printf("    Messages 条数=%d\n", len(data.Messages))
			fmt.Printf("    Tools 条数=%d\n", len(data.Tools))
			fmt.Printf("    Temperature=%.2f\n", data.Temperature)
			if data.Output != nil {
				fmt.Printf("    Output.Type=%s\n", data.Output.Type)
			}
		}
	}
	fmt.Println()

	// --- 示例 3: 错误分类 (ClassifyError) ---
	fmt.Println("=== Example 3: 错误分类 (ClassifyError) ===")

	classNames := map[model.ErrorClass]string{
		model.ErrorClassFatal:        "Fatal(401/403)",
		model.ErrorClassBackoffRetry: "BackoffRetry(429)",
		model.ErrorClassRetry:        "Retry(5xx)",
		model.ErrorClassRetryOnce:    "RetryOnce(unknown)",
	}

	testErrors := []error{
		fmt.Errorf("API error (status 401): unauthorized"),
		fmt.Errorf("API error (status 429): rate limited"),
		fmt.Errorf("API error (status 500): internal server error"),
		fmt.Errorf("connection refused"),
	}
	for _, e := range testErrors {
		cls := model.ClassifyError(e)
		fmt.Printf("  %q\n    -> 分类: %s (%d)\n", e.Error(), classNames[cls], int(cls))
	}
	fmt.Println()

	// --- 示例 4: AttemptRunner 重试配置 ---
	fmt.Println("=== Example 4: AttemptRunner 重试配置 ===")

	runner := model.NewAttemptRunner()
	fmt.Printf("  MaxAttempts=%d\n", runner.MaxAttempts)
	fmt.Printf("  BackoffBase=%v\n", runner.BackoffBase)
	fmt.Printf("  BackoffMax=%v\n", runner.BackoffMax)
	fmt.Println("  说明: Run(ctx, fn) 会对 fn 重试最多 MaxAttempts 次，")
	fmt.Println("        失败时按指数退避 (BackoffBase * 2^(n-1)，上限 BackoffMax) 等待后重试。")
	fmt.Println("        其中 ErrorClassFatal (401/403) 会跳过重试，立即返回错误。")
	fmt.Println("        （此处仅展示配置，不实际调用 Run，以避免发起真实网络请求。）")
	fmt.Println()

	// --- 示例 5: OutputValidator 校验 ---
	fmt.Println("=== Example 5: OutputValidator 校验 ===")

	weatherSchema := &model.OutputSchema{
		Type: "object",
		Properties: map[string]any{
			"city": map[string]any{"type": "string"},
			"temp": map[string]any{"type": "number"},
		},
		Required: []string{"city", "temp"},
	}
	validator := model.NewOutputValidator(weatherSchema)

	fmt.Printf("  OutputSchema:\n")
	fmt.Printf("    Type=%s\n", weatherSchema.Type)
	fmt.Printf("    Properties=%v\n", weatherSchema.Properties)
	fmt.Printf("    Required=%v\n", weatherSchema.Required)
	fmt.Printf("  Validator.MaxRetries=%d\n", validator.MaxRetries)
	fmt.Println("  说明: ValidateAndRetryWithFetch 会在校验失败时通过 ResponseFetcher")
	fmt.Println("        重新向 LLM 拉取响应并再次校验，最多重试 MaxRetries 次。")
	fmt.Println("        （此处仅展示配置，不实际调用 Validate，以避免发起真实网络请求。）")
	fmt.Println()

	// --- 示例 6: full_url 覆盖与 content_mapping 非标字段提取 ---
	fmt.Println("=== Example 6: full_url 覆盖与 content_mapping ===")

	// full_url: 完全覆盖 base_url + default_path 拼接
	cpWithFullURL := &model.StaticConfigProvider{Values: map[string]any{
		"custom": map[string]any{
			"api_key":  "sk-demo",
			"model":    "gpt-4",
			"base_url": "https://api.openai.com/v1",
			"full_url": "https://gateway.proxy.com/custom/llm/chat",
		},
	}}
	customCfg, err := model.LoadProviderConfig(cpWithFullURL, "custom")
	if err != nil {
		fmt.Printf("  custom 配置加载失败: %v\n", err)
	} else {
		resolved := model.ResolveURL(customCfg.BaseURL, "/chat/completions", customCfg.FullURL)
		fmt.Printf("  [full_url] base_url=%q + /chat/completions → 覆盖为 %q\n",
			customCfg.BaseURL, resolved)
	}

	// content_mapping: 从非标 SSE JSON 路径提取 delta/reasoning
	cpWithMapping := &model.StaticConfigProvider{Values: map[string]any{
		"nonstandard": map[string]any{
			"api_key": "sk-demo",
			"model":   "custom-model",
			"content_mapping": map[string]any{
				"reasoning": "data.thinking",
				"delta":     "message.content",
			},
		},
	}}
	mappingCfg, err := model.LoadProviderConfig(cpWithMapping, "nonstandard")
	if err != nil {
		fmt.Printf("  nonstandard 配置加载失败: %v\n", err)
	} else {
		fmt.Printf("  [content_mapping] reasoning=%q, delta=%q\n",
			mappingCfg.ContentMap["reasoning"], mappingCfg.ContentMap["delta"])
		// 演示 ExtractByPath 从非标 JSON 提取
		rawData := map[string]any{
			"data":    map[string]any{"thinking": "custom reasoning text"},
			"message": map[string]any{"content": "answer text"},
		}
		if v, ok := model.ExtractByPath(rawData, "data.thinking"); ok {
			fmt.Printf("  [ExtractByPath] data.thinking → %v\n", v)
		}
		if v, ok := model.ExtractByPath(rawData, "message.content"); ok {
			fmt.Printf("  [ExtractByPath] message.content → %v\n", v)
		}
	}
	fmt.Println()

	// --- 示例 7: OpenAIResponsesProvider 构造 ---
	fmt.Println("=== Example 7: OpenAIResponsesProvider 构造 ===")

	cpResponses := &model.StaticConfigProvider{Values: map[string]any{
		"openai_responses": map[string]any{
			"api_key": "sk-demo",
			"model":   "gpt-4o",
		},
	}}
	responsesProvider, err := model.NewOpenAIResponsesProviderFromConfig(cpResponses, "openai_responses")
	if err != nil {
		fmt.Printf("  OpenAIResponsesProvider 构造失败: %v\n", err)
	} else {
		fmt.Printf("  [Responses] Name=%s, Model=%s\n",
			responsesProvider.Name(), responsesProvider.Model)
		fmt.Println("  说明: Responses API 使用 /responses 端点，")
		fmt.Println("        SSE 事件为 response.output_text.delta / response.reasoning_summary_text.delta")
	}
	fmt.Println()

	// --- 示例 8: LeadingThinkNormalizer 流式 <think> 分离 ---
	fmt.Println("=== Example 8: LeadingThinkNormalizer 流式 <think> 分离 ===")

	var normalizer model.LeadingThinkNormalizer
	fmt.Println("  模拟流式 chunk: <think>step1</think>answer")
	et1, p1 := normalizer.FeedDelta("<think>step1")
	fmt.Printf("    FeedDelta(\"<think>step1\") → eventType=%q, payload=%q\n", et1, p1)
	et2, p2 := normalizer.FeedDelta("</think>answer")
	fmt.Printf("    FeedDelta(\"</think>answer\") → eventType=%q, payload=%q\n", et2, p2)
	et3, p3 := normalizer.FeedDelta("")
	fmt.Printf("    FeedDelta(\"\") → eventType=%q, payload=%q\n", et3, p3)
	fmt.Println("  结果: reasoning=\"step1\", answer=\"answer\"")

	// 非流式 FeedDone 提取
	var normalizer2 model.LeadingThinkNormalizer
	reasoning, answer := normalizer2.FeedDone("<think>reasoning content</think>actual answer")
	fmt.Printf("  FeedDone(\"<think>reasoning content</think>actual answer\")\n")
	fmt.Printf("    → reasoning=%q, answer=%q\n", reasoning, answer)
	fmt.Println()

	fmt.Println("=== All examples completed ===")
}
