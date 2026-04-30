package model

import "fmt"

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
