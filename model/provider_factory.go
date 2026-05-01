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
