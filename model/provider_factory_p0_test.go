package model

import (
	"errors"
	"strings"
	"testing"
)

// 第二批 P0 Provider factory 单元测试
// 覆盖 6 家 Provider（MiMo/MiMo Anthropic/腾讯混元/豆包 Seed/01.AI/MiniMax/SiliconFlow）
// 共 7 个 factory。每个 factory 测试 3 种场景：默认配置加载、StaticConfigProvider
// 覆盖、APIKey 缺失。

// === 小米 MiMo（OpenAI 兼容）===
func TestNewMiMoProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"mimo": map[string]any{
				"api_key": "sk-mimo-test",
			},
		}}
		p, err := NewMiMoProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewMiMoProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://api.xiaomimimo.com/v1" {
			t.Errorf("BaseURL = %q, want https://api.xiaomimimo.com/v1", p.BaseURL)
		}
		if p.Model != "mimo-v2.5-pro" {
			t.Errorf("Model = %q, want mimo-v2.5-pro", p.Model)
		}
		if p.Name() != "mimo" {
			t.Errorf("Name() = %q, want mimo", p.Name())
		}
		if p.APIKey != "sk-mimo-test" {
			t.Errorf("APIKey = %q, want sk-mimo-test", p.APIKey)
		}
		if p.ProviderName != "mimo" {
			t.Errorf("ProviderName = %q, want mimo", p.ProviderName)
		}
	})

	t.Run("override_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"mimo": map[string]any{
				"api_key":  "sk-mimo-test",
				"base_url": "https://custom.example.com/v1",
				"model":    "mimo-v2-flash",
			},
		}}
		p, err := NewMiMoProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewMiMoProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://custom.example.com/v1" {
			t.Errorf("BaseURL = %q, want https://custom.example.com/v1", p.BaseURL)
		}
		if p.Model != "mimo-v2-flash" {
			t.Errorf("Model = %q, want mimo-v2-flash", p.Model)
		}
		if p.APIKey != "sk-mimo-test" {
			t.Errorf("APIKey = %q, want sk-mimo-test", p.APIKey)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewMiMoProviderFromConfig(cp)
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "mimo") {
			t.Errorf("error should contain prefix 'mimo', got %v", err)
		}
	})
}

// === 小米 MiMo（Anthropic 兼容）===
func TestNewMiMoAnthropicProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"mimo_anthropic": map[string]any{
				"api_key": "sk-mimo-test",
			},
		}}
		p, err := NewMiMoAnthropicProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewMiMoAnthropicProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://api.xiaomimimo.com/anthropic" {
			t.Errorf("BaseURL = %q, want https://api.xiaomimimo.com/anthropic", p.BaseURL)
		}
		if p.Model != "mimo-v2.5-pro" {
			t.Errorf("Model = %q, want mimo-v2.5-pro", p.Model)
		}
		if p.APIKey != "sk-mimo-test" {
			t.Errorf("APIKey = %q, want sk-mimo-test", p.APIKey)
		}
	})

	t.Run("override_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"mimo_anthropic": map[string]any{
				"api_key":  "sk-mimo-test",
				"base_url": "https://custom.example.com/anthropic",
				"model":    "mimo-v2-flash",
			},
		}}
		p, err := NewMiMoAnthropicProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewMiMoAnthropicProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://custom.example.com/anthropic" {
			t.Errorf("BaseURL = %q, want https://custom.example.com/anthropic", p.BaseURL)
		}
		if p.Model != "mimo-v2-flash" {
			t.Errorf("Model = %q, want mimo-v2-flash", p.Model)
		}
		if p.APIKey != "sk-mimo-test" {
			t.Errorf("APIKey = %q, want sk-mimo-test", p.APIKey)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewMiMoAnthropicProviderFromConfig(cp)
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "mimo_anthropic") {
			t.Errorf("error should contain prefix 'mimo_anthropic', got %v", err)
		}
	})
}

// === 腾讯混元 ===
func TestNewTencentProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"tencent": map[string]any{
				"api_key": "sk-tencent-test",
			},
		}}
		p, err := NewTencentProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewTencentProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://api.hunyuan.cloud.tencent.com/v1" {
			t.Errorf("BaseURL = %q, want https://api.hunyuan.cloud.tencent.com/v1", p.BaseURL)
		}
		if p.Model != "hunyuan-max" {
			t.Errorf("Model = %q, want hunyuan-max", p.Model)
		}
		if p.Name() != "tencent" {
			t.Errorf("Name() = %q, want tencent", p.Name())
		}
		if p.APIKey != "sk-tencent-test" {
			t.Errorf("APIKey = %q, want sk-tencent-test", p.APIKey)
		}
		if p.ProviderName != "tencent" {
			t.Errorf("ProviderName = %q, want tencent", p.ProviderName)
		}
	})

	t.Run("override_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"tencent": map[string]any{
				"api_key":  "sk-tencent-test",
				"base_url": "https://custom.example.com/v1",
				"model":    "hunyuan-pro",
			},
		}}
		p, err := NewTencentProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewTencentProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://custom.example.com/v1" {
			t.Errorf("BaseURL = %q, want https://custom.example.com/v1", p.BaseURL)
		}
		if p.Model != "hunyuan-pro" {
			t.Errorf("Model = %q, want hunyuan-pro", p.Model)
		}
		if p.APIKey != "sk-tencent-test" {
			t.Errorf("APIKey = %q, want sk-tencent-test", p.APIKey)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewTencentProviderFromConfig(cp)
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "tencent") {
			t.Errorf("error should contain prefix 'tencent', got %v", err)
		}
	})
}

// === 字节豆包（火山引擎）===
func TestNewVolcengineProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"volcengine": map[string]any{
				"api_key": "sk-volc-test",
			},
		}}
		p, err := NewVolcengineProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewVolcengineProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://ark.cn-beijing.volces.com/api/v3" {
			t.Errorf("BaseURL = %q, want https://ark.cn-beijing.volces.com/api/v3", p.BaseURL)
		}
		if p.Model != "doubao-seed-2-0-pro" {
			t.Errorf("Model = %q, want doubao-seed-2-0-pro", p.Model)
		}
		if p.Name() != "volcengine" {
			t.Errorf("Name() = %q, want volcengine", p.Name())
		}
		if p.APIKey != "sk-volc-test" {
			t.Errorf("APIKey = %q, want sk-volc-test", p.APIKey)
		}
		if p.ProviderName != "volcengine" {
			t.Errorf("ProviderName = %q, want volcengine", p.ProviderName)
		}
	})

	t.Run("override_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"volcengine": map[string]any{
				"api_key":  "sk-volc-test",
				"base_url": "https://custom.example.com/api/v3",
				"model":    "doubao-pro-32k",
			},
		}}
		p, err := NewVolcengineProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewVolcengineProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://custom.example.com/api/v3" {
			t.Errorf("BaseURL = %q, want https://custom.example.com/api/v3", p.BaseURL)
		}
		if p.Model != "doubao-pro-32k" {
			t.Errorf("Model = %q, want doubao-pro-32k", p.Model)
		}
		if p.APIKey != "sk-volc-test" {
			t.Errorf("APIKey = %q, want sk-volc-test", p.APIKey)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewVolcengineProviderFromConfig(cp)
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "volcengine") {
			t.Errorf("error should contain prefix 'volcengine', got %v", err)
		}
	})
}

// === 零一万物 ===
func TestNewZeroOneProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"zeroone": map[string]any{
				"api_key": "sk-yi-test",
			},
		}}
		p, err := NewZeroOneProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewZeroOneProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://api.01.ai/v1" {
			t.Errorf("BaseURL = %q, want https://api.01.ai/v1", p.BaseURL)
		}
		if p.Model != "yi-lightning" {
			t.Errorf("Model = %q, want yi-lightning", p.Model)
		}
		if p.Name() != "zeroone" {
			t.Errorf("Name() = %q, want zeroone", p.Name())
		}
		if p.APIKey != "sk-yi-test" {
			t.Errorf("APIKey = %q, want sk-yi-test", p.APIKey)
		}
		if p.ProviderName != "zeroone" {
			t.Errorf("ProviderName = %q, want zeroone", p.ProviderName)
		}
	})

	t.Run("override_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"zeroone": map[string]any{
				"api_key":  "sk-yi-test",
				"base_url": "https://custom.example.com/v1",
				"model":    "yi-large",
			},
		}}
		p, err := NewZeroOneProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewZeroOneProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://custom.example.com/v1" {
			t.Errorf("BaseURL = %q, want https://custom.example.com/v1", p.BaseURL)
		}
		if p.Model != "yi-large" {
			t.Errorf("Model = %q, want yi-large", p.Model)
		}
		if p.APIKey != "sk-yi-test" {
			t.Errorf("APIKey = %q, want sk-yi-test", p.APIKey)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewZeroOneProviderFromConfig(cp)
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "zeroone") {
			t.Errorf("error should contain prefix 'zeroone', got %v", err)
		}
	})
}

// === MiniMax（稀宇科技）===
func TestNewMiniMaxProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"minimax": map[string]any{
				"api_key": "sk-minimax-test",
			},
		}}
		p, err := NewMiniMaxProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewMiniMaxProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://api.minimax.chat/v1" {
			t.Errorf("BaseURL = %q, want https://api.minimax.chat/v1", p.BaseURL)
		}
		if p.Model != "MiniMax-M2.7" {
			t.Errorf("Model = %q, want MiniMax-M2.7", p.Model)
		}
		if p.Name() != "minimax" {
			t.Errorf("Name() = %q, want minimax", p.Name())
		}
		if p.APIKey != "sk-minimax-test" {
			t.Errorf("APIKey = %q, want sk-minimax-test", p.APIKey)
		}
		if p.ProviderName != "minimax" {
			t.Errorf("ProviderName = %q, want minimax", p.ProviderName)
		}
	})

	t.Run("override_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"minimax": map[string]any{
				"api_key":  "sk-minimax-test",
				"base_url": "https://custom.example.com/v1",
				"model":    "abab6.5s-chat",
			},
		}}
		p, err := NewMiniMaxProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewMiniMaxProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://custom.example.com/v1" {
			t.Errorf("BaseURL = %q, want https://custom.example.com/v1", p.BaseURL)
		}
		if p.Model != "abab6.5s-chat" {
			t.Errorf("Model = %q, want abab6.5s-chat", p.Model)
		}
		if p.APIKey != "sk-minimax-test" {
			t.Errorf("APIKey = %q, want sk-minimax-test", p.APIKey)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewMiniMaxProviderFromConfig(cp)
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "minimax") {
			t.Errorf("error should contain prefix 'minimax', got %v", err)
		}
	})
}

// === 硅基流动（聚合平台）===
func TestNewSiliconFlowProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"siliconflow": map[string]any{
				"api_key": "sk-sf-test",
			},
		}}
		p, err := NewSiliconFlowProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewSiliconFlowProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://api.siliconflow.cn/v1" {
			t.Errorf("BaseURL = %q, want https://api.siliconflow.cn/v1", p.BaseURL)
		}
		if p.Model != "Qwen/Qwen2.5-72B-Instruct" {
			t.Errorf("Model = %q, want Qwen/Qwen2.5-72B-Instruct", p.Model)
		}
		if p.Name() != "siliconflow" {
			t.Errorf("Name() = %q, want siliconflow", p.Name())
		}
		if p.APIKey != "sk-sf-test" {
			t.Errorf("APIKey = %q, want sk-sf-test", p.APIKey)
		}
		if p.ProviderName != "siliconflow" {
			t.Errorf("ProviderName = %q, want siliconflow", p.ProviderName)
		}
	})

	t.Run("override_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"siliconflow": map[string]any{
				"api_key":  "sk-sf-test",
				"base_url": "https://custom.example.com/v1",
				"model":    "deepseek-ai/DeepSeek-V3",
			},
		}}
		p, err := NewSiliconFlowProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewSiliconFlowProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://custom.example.com/v1" {
			t.Errorf("BaseURL = %q, want https://custom.example.com/v1", p.BaseURL)
		}
		if p.Model != "deepseek-ai/DeepSeek-V3" {
			t.Errorf("Model = %q, want deepseek-ai/DeepSeek-V3", p.Model)
		}
		if p.APIKey != "sk-sf-test" {
			t.Errorf("APIKey = %q, want sk-sf-test", p.APIKey)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewSiliconFlowProviderFromConfig(cp)
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "siliconflow") {
			t.Errorf("error should contain prefix 'siliconflow', got %v", err)
		}
	})
}
