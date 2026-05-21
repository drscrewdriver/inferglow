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

package model

import (
	"testing"
)

// G1-07.5: Provider Config Load Benchmark
// 覆盖：所有 Provider 的初始化耗时。

// staticBenchConfig 是一个内存级 ConfigProvider，避免 IO 影响基准。
// 用于测量 LoadProviderConfig + Factory 构造的纯逻辑开销。
type staticBenchConfig struct {
	data map[string]any
}

func (s *staticBenchConfig) Get(key string) (any, bool) {
	v, ok := s.data[key]
	return v, ok
}

// newStaticBenchConfig 构造一份带完整配置（含 api_key）的静态 ConfigProvider。
func newStaticBenchConfig() *staticBenchConfig {
	return &staticBenchConfig{
		data: map[string]any{
			"openai.api_key":               "sk-bench",
			"openai.base_url":              "https://api.openai.com/v1",
			"openai.model":                 "gpt-4",
			"anthropic.api_key":            "sk-bench",
			"anthropic.base_url":           "https://api.anthropic.com",
			"anthropic.model":              "claude-3-5-sonnet",
			"deepseek.api_key":             "sk-bench",
			"deepseek.base_url":            "https://api.deepseek.com/v1",
			"deepseek.model":               "deepseek-chat",
			"qwen.api_key":                 "sk-bench",
			"qwen.base_url":                "https://dashscope.aliyuncs.com/compatible-mode/v1",
			"qwen.model":                   "qwen-max",
			"glm.api_key":                  "sk-bench",
			"glm.base_url":                 "https://open.bigmodel.cn/api/paas/v4",
			"glm.model":                    "glm-4-plus",
			"kimi.api_key":                 "sk-bench",
			"kimi.base_url":                "https://api.moonshot.cn/v1",
			"kimi.model":                   "moonshot-v1-32k",
			"mimo.api_key":                 "sk-bench",
			"mimo.base_url":                "https://api.xiaomimimo.com/v1",
			"mimo.model":                   "mimo-v2.5-pro",
			"mimo_anthropic.api_key":       "sk-bench",
			"mimo_anthropic.base_url":      "https://api.xiaomimimo.com/anthropic",
			"mimo_anthropic.model":         "mimo-v2.5-pro",
			"tencent.api_key":              "sk-bench",
			"tencent.base_url":             "https://api.hunyuan.cloud.tencent.com/v1",
			"tencent.model":                "hunyuan-max",
			"volcengine.api_key":           "sk-bench",
			"volcengine.base_url":          "https://ark.cn-beijing.volces.com/api/v3",
			"volcengine.model":             "doubao-seed-2-0-pro",
			"zeroone.api_key":              "sk-bench",
			"zeroone.base_url":             "https://api.01.ai/v1",
			"zeroone.model":                "yi-lightning",
			"minimax.api_key":              "sk-bench",
			"minimax.base_url":             "https://api.minimax.chat/v1",
			"minimax.model":                "MiniMax-M2.7",
			"siliconflow.api_key":          "sk-bench",
			"siliconflow.base_url":         "https://api.siliconflow.cn/v1",
			"siliconflow.model":            "Qwen/Qwen2.5-72B-Instruct",
			"stepfun.api_key":              "sk-bench",
			"stepfun.base_url":             "https://api.stepfun.com/v1",
			"stepfun.model":                "step-3.7-flash",
			"stepfun_anthropic.api_key":    "sk-bench",
			"stepfun_anthropic.base_url":   "https://api.stepfun.com/step_plan",
			"stepfun_anthropic.model":      "step-3.7-flash",
			"baidu.api_key":                "sk-bench",
			"baidu.base_url":               "https://qianfan.baidubce.com/v2",
			"baidu.model":                  "ernie-5.0",
			"spark.api_key":                "sk-bench",
			"spark.base_url":               "https://spark-api-open.xf-yun.com/agent/v1/",
			"spark.model":                  "spark-x",
			"sensenova.api_key":            "sk-bench",
			"sensenova.base_url":           "https://token.sensenova.cn/v1",
			"sensenova.model":              "sensenova-6.7-flash-lite",
			"sensenova_anthropic.api_key":  "sk-bench",
			"sensenova_anthropic.base_url": "https://token.sensenova.cn/v1/messages",
			"sensenova_anthropic.model":    "sensenova-6.7-flash-lite",
		},
	}
}

// BenchmarkLoadProviderConfig 测量 LoadProviderConfig 单次调用开销（不含 Factory 构造）。
func BenchmarkLoadProviderConfig(b *testing.B) {
	cp := newStaticBenchConfig()
	providers := []string{
		"openai", "deepseek", "qwen", "glm", "kimi",
		"mimo", "tencent", "volcengine", "zeroone", "minimax", "siliconflow",
	}
	for _, name := range providers {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := LoadProviderConfig(cp, name)
				if err != nil {
					b.Fatalf("LoadProviderConfig %s: %v", name, err)
				}
			}
		})
	}
}

// BenchmarkProviderFactoryFromConfig 测量 LoadProviderConfig + Factory 构造的总开销。
func BenchmarkProviderFactoryFromConfig(b *testing.B) {
	cp := newStaticBenchConfig()
	cases := []struct {
		name string
		fn   func(ConfigProvider) (*OpenAICompatibleProvider, error)
	}{
		{"openai", NewOpenAIProviderFromConfig},
		{"deepseek", NewDeepSeekProviderFromConfig},
		{"qwen", NewQwenProviderFromConfig},
		{"glm", NewGLMProviderFromConfig},
		{"kimi", NewKimiProviderFromConfig},
		{"mimo", NewMiMoProviderFromConfig},
		{"tencent", NewTencentProviderFromConfig},
		{"volcengine", NewVolcengineProviderFromConfig},
		{"zeroone", NewZeroOneProviderFromConfig},
		{"minimax", NewMiniMaxProviderFromConfig},
		{"siliconflow", NewSiliconFlowProviderFromConfig},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := c.fn(cp)
				if err != nil {
					b.Fatalf("%s: %v", c.name, err)
				}
			}
		})
	}
}

// BenchmarkProviderFactoryAnthropic 测量 Anthropic 兼容端点的 Factory 开销。
func BenchmarkProviderFactoryAnthropic(b *testing.B) {
	cp := newStaticBenchConfig()
	b.Run("mimo_anthropic", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := NewMiMoAnthropicProviderFromConfig(cp)
			if err != nil {
				b.Fatalf("mimo_anthropic: %v", err)
			}
		}
	})
	b.Run("stepfun_anthropic", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := NewStepFunAnthropicProviderFromConfig(cp)
			if err != nil {
				b.Fatalf("stepfun_anthropic: %v", err)
			}
		}
	})
	b.Run("sensenova_anthropic", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, err := NewSenseNovaAnthropicProviderFromConfig(cp)
			if err != nil {
				b.Fatalf("sensenova_anthropic: %v", err)
			}
		}
	})
}

// BenchmarkEnvConfigProviderGet 测量 EnvConfigProvider.Get 的开销
// （每次调用都查环境变量，是 Provider 配置层的最低开销基线）。
func BenchmarkEnvConfigProviderGet(b *testing.B) {
	cp := &EnvConfigProvider{Prefix: "INFERGLOW_"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cp.Get("openai.api_key")
	}
}

// BenchmarkFileConfigProviderGet 测量 FileConfigProvider.Get 的开销
// （首次加载后会复用内存数据，benchmark 测稳态查询）。
func BenchmarkFileConfigProviderGet(b *testing.B) {
	// 不存在的文件，避免依赖外部资源；Get 返回 (nil, false) 但 still 测开销
	cp := &FileConfigProvider{Path: "/nonexistent/bench.yaml"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cp.Get("openai.api_key")
	}
}

// Sanity test 确保所有 factory 在配置完整时都能成功构造。
// OpenAI provider 不设置 ProviderName（默认返回 "openai-compatible"），
// 因此这里只校验新加入的 P0 Provider 的 Name()。
func TestBenchConfigSanity(t *testing.T) {
	cp := newStaticBenchConfig()
	factories := []struct {
		name string
		fn   func(ConfigProvider) (*OpenAICompatibleProvider, error)
	}{
		{"mimo", NewMiMoProviderFromConfig},
		{"tencent", NewTencentProviderFromConfig},
		{"volcengine", NewVolcengineProviderFromConfig},
		{"zeroone", NewZeroOneProviderFromConfig},
		{"minimax", NewMiniMaxProviderFromConfig},
		{"siliconflow", NewSiliconFlowProviderFromConfig},
	}
	for _, f := range factories {
		p, err := f.fn(cp)
		if err != nil {
			t.Errorf("%s factory failed: %v", f.name, err)
			continue
		}
		if p.Name() != f.name {
			t.Errorf("%s: Name() = %q, want %q", f.name, p.Name(), f.name)
		}
	}
}
