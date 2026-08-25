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

import "fmt"

// developerToSystemRoleMapping is the shared RoleMapping used by providers
// that reject the "developer" role (DeepSeek/Qwen/GLM/Kimi). Mapping it to
// "system" causes GenerateRequestData to merge developer content into the
// system message instead of emitting a separate "developer" message.
var developerToSystemRoleMapping = map[string]string{
	"developer": "system",
}

// NewOpenAIProviderFromConfig 从 ConfigProvider 构造 OpenAICompatibleProvider
// 复用 LoadProviderConfig(cp, "openai") 加载配置
func NewOpenAIProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "openai")
	if err != nil {
		return nil, fmt.Errorf("load openai provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:        cfg.BaseURL,
		APIKey:         cfg.APIKey,
		Model:          cfg.Model,
		HTTPClient:     cfg.HTTPClient,
		FullURL:        cfg.FullURL,
		ContentMapping: cfg.ContentMap,
	}, nil
}

// NewAnthropicProviderFromConfig 从 ConfigProvider 构造 AnthropicCompatibleProvider
// 复用 LoadProviderConfig(cp, "anthropic") 加载配置
func NewAnthropicProviderFromConfig(cp ConfigProvider) (*AnthropicCompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "anthropic")
	if err != nil {
		return nil, fmt.Errorf("load anthropic provider config: %w", err)
	}
	return &AnthropicCompatibleProvider{
		BaseURL:    cfg.BaseURL,
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: cfg.HTTPClient,
		FullURL:    cfg.FullURL,
	}, nil
}

// NewDeepSeekProviderFromConfig 构造 DeepSeek Provider，使用 OpenAI 兼容协议。
// DeepSeek API 不接受 "developer" 角色，因此 RoleMapping 将 developer 映射为
// system（在 GenerateRequestData 中合并到 system 消息）。
// 默认 base_url: https://api.deepseek.com/v1
func NewDeepSeekProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "deepseek")
	if err != nil {
		return nil, fmt.Errorf("load deepseek provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "deepseek",
		RoleMapping:  developerToSystemRoleMapping,
	}, nil
}

// NewQwenProviderFromConfig 构造 Qwen Provider（通义千问，阿里云 DashScope）。
// Qwen 使用 OpenAI 兼容协议但不支持 "developer" 角色。
// 默认 base_url: https://dashscope.aliyuncs.com/compatible-mode/v1
func NewQwenProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "qwen")
	if err != nil {
		return nil, fmt.Errorf("load qwen provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "qwen",
		RoleMapping:  developerToSystemRoleMapping,
	}, nil
}

// NewGLMProviderFromConfig 构造 GLM Provider（智谱 AI）。
// GLM 使用 OpenAI 兼容协议但不支持 "developer" 角色。
// 默认 base_url: https://open.bigmodel.cn/api/paas/v4
func NewGLMProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "glm")
	if err != nil {
		return nil, fmt.Errorf("load glm provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "glm",
		RoleMapping:  developerToSystemRoleMapping,
	}, nil
}

// NewKimiProviderFromConfig 构造 Kimi Provider（Moonshot AI）。
// Kimi 使用 OpenAI 兼容协议但不支持 "developer" 角色。
// 默认 base_url: https://api.moonshot.cn/v1
func NewKimiProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "kimi")
	if err != nil {
		return nil, fmt.Errorf("load kimi provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "kimi",
		RoleMapping:  developerToSystemRoleMapping,
	}, nil
}

// NewStepFunProviderFromConfig 构造阶跃星辰 OpenAI 兼容 Provider。
// 默认 base_url: https://api.stepfun.com/v1
// StepFun 同时提供 Anthropic 兼容端点，见 NewStepFunAnthropicProviderFromConfig。
func NewStepFunProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "stepfun")
	if err != nil {
		return nil, fmt.Errorf("load stepfun provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "stepfun",
	}, nil
}

// NewStepFunAnthropicProviderFromConfig 构造阶跃星辰 Anthropic 兼容 Provider。
// 默认 base_url: https://api.stepfun.com/step_plan（注意：不含 /v1）
func NewStepFunAnthropicProviderFromConfig(cp ConfigProvider) (*AnthropicCompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "stepfun_anthropic")
	if err != nil {
		return nil, fmt.Errorf("load stepfun_anthropic provider config: %w", err)
	}
	return &AnthropicCompatibleProvider{
		BaseURL:    cfg.BaseURL,
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: cfg.HTTPClient,
		FullURL:    cfg.FullURL,
	}, nil
}

// NewBaiduProviderFromConfig 构造百度千帆 Provider（ERNIE / DeepSeek 等）。
// 注意：base_url 路径为 /v2（非标准 /v1），但协议完全兼容 OpenAI。
// API Key 格式为 bce-v3/ALTAK-xxxx/xxxx，认证仍是标准 Bearer Token。
func NewBaiduProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "baidu")
	if err != nil {
		return nil, fmt.Errorf("load baidu provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "baidu",
	}, nil
}

// NewSparkProviderFromConfig 构造讯飞星火 Provider。
// 注意：base_url 含 /agent/v1/ 前缀（非标准 /v1），但协议完全兼容 OpenAI。
// HTTP 认证使用 AK:SK 拼接格式，本质仍是 Bearer Token。
// WebSocket 原生端点不纳入适配范围。
func NewSparkProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "spark")
	if err != nil {
		return nil, fmt.Errorf("load spark provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "spark",
	}, nil
}

// NewSenseNovaProviderFromConfig 构造商汤 SenseNova OpenAI 兼容 Provider。
// 默认 base_url 使用 Token Plan 域名（token.sensenova.cn），适合试用；
// 标准商用域名 api.sensenova.cn 可通过配置覆盖。
func NewSenseNovaProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "sensenova")
	if err != nil {
		return nil, fmt.Errorf("load sensenova provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "sensenova",
	}, nil
}

// NewSenseNovaAnthropicProviderFromConfig 构造商汤 SenseNova Anthropic 兼容 Provider。
// 默认 base_url: https://token.sensenova.cn/v1/messages
func NewSenseNovaAnthropicProviderFromConfig(cp ConfigProvider) (*AnthropicCompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "sensenova_anthropic")
	if err != nil {
		return nil, fmt.Errorf("load sensenova_anthropic provider config: %w", err)
	}
	return &AnthropicCompatibleProvider{
		BaseURL:    cfg.BaseURL,
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: cfg.HTTPClient,
		FullURL:    cfg.FullURL,
	}, nil
}

// === 小米 MiMo ===

// NewMiMoProviderFromConfig 构造 MiMo OpenAI 兼容 Provider。
// 默认 base_url: https://api.xiaomimimo.com/v1
// MiMo 的推理内容通过 reasoning_content 字段返回（G1-02 已支持），
// 深度思考通过 thinking.type 参数控制（G1-03 通过 Options 透传）。
// MiMo 是否支持 developer 角色需实测，暂不设置 RoleMapping。
func NewMiMoProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "mimo")
	if err != nil {
		return nil, fmt.Errorf("load mimo provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "mimo",
	}, nil
}

// NewMiMoAnthropicProviderFromConfig 构造 MiMo Anthropic 兼容 Provider。
// 默认 base_url: https://api.xiaomimimo.com/anthropic
func NewMiMoAnthropicProviderFromConfig(cp ConfigProvider) (*AnthropicCompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "mimo_anthropic")
	if err != nil {
		return nil, fmt.Errorf("load mimo_anthropic provider config: %w", err)
	}
	return &AnthropicCompatibleProvider{
		BaseURL:    cfg.BaseURL,
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: cfg.HTTPClient,
		FullURL:    cfg.FullURL,
	}, nil
}

// === 腾讯混元 ===

// NewTencentProviderFromConfig 构造腾讯混元 Provider。
// 默认 base_url: https://api.hunyuan.cloud.tencent.com/v1
// 标准 OpenAI 兼容协议，无需特殊适配。
func NewTencentProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "tencent")
	if err != nil {
		return nil, fmt.Errorf("load tencent provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "tencent",
	}, nil
}

// === 字节豆包（火山引擎）===

// NewVolcengineProviderFromConfig 构造字节豆包 Provider。
// 默认 base_url: https://ark.cn-beijing.volces.com/api/v3
// 标准 OpenAI 兼容协议；如有 X-Api-Key 等特殊认证需求后续按需扩展。
func NewVolcengineProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "volcengine")
	if err != nil {
		return nil, fmt.Errorf("load volcengine provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "volcengine",
	}, nil
}

// === 零一万物 ===

// NewZeroOneProviderFromConfig 构造零一万物 Provider。
// 默认 base_url: https://api.01.ai/v1
// 标准 OpenAI 兼容协议。
func NewZeroOneProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "zeroone")
	if err != nil {
		return nil, fmt.Errorf("load zeroone provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "zeroone",
	}, nil
}

// === MiniMax（稀宇科技）===

// NewMiniMaxProviderFromConfig 构造 MiniMax Provider（海螺 AI 母公司）。
// 默认 base_url: https://api.minimax.chat/v1
// 标准 OpenAI 兼容协议；多模态+语音能力不在本适配范围。
func NewMiniMaxProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "minimax")
	if err != nil {
		return nil, fmt.Errorf("load minimax provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "minimax",
	}, nil
}

// === 硅基流动（聚合平台）===

// NewSiliconFlowProviderFromConfig 构造 SiliconFlow Provider。
// 默认 base_url: https://api.siliconflow.cn/v1
// 聚合平台：一个 Key 可调用多个开源模型（Qwen/GLM/DeepSeek 等）。
// 标准 OpenAI 兼容协议，切换模型只需覆盖 .model 配置项。
func NewSiliconFlowProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "siliconflow")
	if err != nil {
		return nil, fmt.Errorf("load siliconflow provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "siliconflow",
	}, nil
}

// === OpenRouter（聚合平台）===

// NewOpenRouterProviderFromConfig 构造 OpenRouter Provider。
// 默认 base_url: https://openrouter.ai/api/v1
// 聚合平台：一个 Key 可调用多个模型（OpenAI/Anthropic/开源模型等）。
// OpenRouter 使用 reasoning_details 字段返回推理信息（G1-02 扩展已支持解析）。
// 标准 OpenAI 兼容协议，切换模型只需覆盖 .model 配置项。
func NewOpenRouterProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, "openrouter")
	if err != nil {
		return nil, fmt.Errorf("load openrouter provider config: %w", err)
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: "openrouter",
	}, nil
}

// === OpenAI Responses API ===

// NewOpenAIResponsesProviderFromConfig 构造 OpenAIResponsesProvider。
// 默认 base_url: https://api.openai.com/v1，默认 model: gpt-4o。
// 对接 OpenAI Responses API (/responses 端点)，推荐用于 o-series 推理模型。
// Spec: model-parity Phase 2, P0 — OpenAI Responses API Provider.
func NewOpenAIResponsesProviderFromConfig(cp ConfigProvider, prefix string) (*OpenAIResponsesProvider, error) {
	cfg, err := LoadProviderConfig(cp, prefix)
	if err != nil {
		return nil, fmt.Errorf("load %s provider config: %w", prefix, err)
	}
	return &OpenAIResponsesProvider{
		BaseURL:    cfg.BaseURL,
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: cfg.HTTPClient,
		FullURL:    cfg.FullURL,
	}, nil
}

// === LLM-provider-port P5: 新增 OpenAI 兼容 provider（Mistral/Groq/xAI/Together/ZAI/Moonshot 国际/NVIDIA）===
// 这些 provider 均复用 OpenAI 兼容协议，仅 prefix 与 ProviderName 不同。
// 统一构造器避免逐个复制；NewOpenAICompatProviderFromConfig(cp, prefix, providerName)。

// NewOpenAICompatProviderFromConfig 按 prefix 加载配置并构造
// OpenAICompatibleProvider（ProviderName=providerName，空则用 prefix）。
func NewOpenAICompatProviderFromConfig(cp ConfigProvider, prefix, providerName string) (*OpenAICompatibleProvider, error) {
	cfg, err := LoadProviderConfig(cp, prefix)
	if err != nil {
		return nil, fmt.Errorf("load %s provider config: %w", prefix, err)
	}
	if providerName == "" {
		providerName = prefix
	}
	return &OpenAICompatibleProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		FullURL:      cfg.FullURL,
		ContentMapping: cfg.ContentMap,
		ProviderName: providerName,
	}, nil
}

// NewMistralProviderFromConfig 构造 Mistral AI Provider（OpenAI 兼容端点）。
func NewMistralProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	return NewOpenAICompatProviderFromConfig(cp, "mistral", "mistral")
}

// NewGroqProviderFromConfig 构造 Groq Provider。
func NewGroqProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	return NewOpenAICompatProviderFromConfig(cp, "groq", "groq")
}

// NewXAIProviderFromConfig 构造 xAI Grok Provider。
func NewXAIProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	return NewOpenAICompatProviderFromConfig(cp, "xai", "xai")
}

// NewTogetherProviderFromConfig 构造 Together AI Provider。
func NewTogetherProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	return NewOpenAICompatProviderFromConfig(cp, "together", "together")
}

// NewZAIProviderFromConfig 构造 Z.AI（智谱 GLM 新版）Provider。
func NewZAIProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	return NewOpenAICompatProviderFromConfig(cp, "zai", "zai")
}

// NewMoonshotAIProviderFromConfig 构造 Moonshot 国际版 Provider。
func NewMoonshotAIProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	return NewOpenAICompatProviderFromConfig(cp, "moonshotai", "moonshotai")
}

// NewNVIDIAProviderFromConfig 构造 NVIDIA NIM Provider。
func NewNVIDIAProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	return NewOpenAICompatProviderFromConfig(cp, "nvidia", "nvidia")
}

// NewCerebrasProviderFromConfig 构造 Cerebras Provider（长尾）。
func NewCerebrasProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	return NewOpenAICompatProviderFromConfig(cp, "cerebras", "cerebras")
}

// NewHuggingFaceProviderFromConfig 构造 Hugging Face Router Provider（长尾）。
func NewHuggingFaceProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	return NewOpenAICompatProviderFromConfig(cp, "huggingface", "huggingface")
}

// NewFireworksProviderFromConfig 构造 Fireworks AI Provider（长尾，OpenAI 兼容端点）。
func NewFireworksProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	return NewOpenAICompatProviderFromConfig(cp, "fireworks", "fireworks")
}

// NewQwenTokenPlanCNProviderFromConfig 构造通义 Token 套餐 Provider（长尾）。
func NewQwenTokenPlanCNProviderFromConfig(cp ConfigProvider) (*OpenAICompatibleProvider, error) {
	return NewOpenAICompatProviderFromConfig(cp, "qwen-token-plan-cn", "qwen-token-plan-cn")
}

// NewGoogleProviderFromConfig 构造 Google Generative Language 原生 Provider。
// 默认 base_url: https://generativelanguage.googleapis.com/v1beta
// 走原生 streamGenerateContent 协议（非 OpenAI 兼容），支持 Gemini 3 thinkingLevel。
func NewGoogleProviderFromConfig(cp ConfigProvider) (*GoogleGenerativeProvider, error) {
	cfg, err := LoadProviderConfig(cp, "google")
	if err != nil {
		return nil, fmt.Errorf("load google provider config: %w", err)
	}
	return &GoogleGenerativeProvider{
		BaseURL:      cfg.BaseURL,
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		HTTPClient:   cfg.HTTPClient,
		ProviderName: "google",
	}, nil
}
