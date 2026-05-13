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
		BaseURL:    cfg.BaseURL,
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		HTTPClient: cfg.HTTPClient,
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
		ProviderName: "siliconflow",
	}, nil
}
