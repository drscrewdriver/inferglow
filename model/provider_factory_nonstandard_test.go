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
	"errors"
	"strings"
	"testing"
)

// 非标准 OpenAI 兼容 Provider factory 单元测试
// 覆盖 4 家 Provider（阶跃星辰、百度千帆、讯飞星火、商汤日日新）共 6 个 factory。
// 每个 factory 测试 3 种场景：默认配置加载、StaticConfigProvider 覆盖、APIKey 缺失。

// === 阶跃星辰（StepFun）OpenAI 兼容 ===
func TestNewStepFunProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"stepfun": map[string]any{
				"api_key": "sk-test",
			},
		}}
		p, err := NewStepFunProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewStepFunProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://api.stepfun.com/v1" {
			t.Errorf("BaseURL = %q, want https://api.stepfun.com/v1", p.BaseURL)
		}
		if p.Model != "step-3.7-flash" {
			t.Errorf("Model = %q, want step-3.7-flash", p.Model)
		}
		if p.Name() != "stepfun" {
			t.Errorf("Name() = %q, want stepfun", p.Name())
		}
		if p.APIKey != "sk-test" {
			t.Errorf("APIKey = %q, want sk-test", p.APIKey)
		}
		if p.ProviderName != "stepfun" {
			t.Errorf("ProviderName = %q, want stepfun", p.ProviderName)
		}
	})

	t.Run("override_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"stepfun": map[string]any{
				"api_key":  "sk-test",
				"base_url": "https://custom.example.com/v1",
				"model":    "step-custom",
			},
		}}
		p, err := NewStepFunProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewStepFunProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://custom.example.com/v1" {
			t.Errorf("BaseURL = %q, want https://custom.example.com/v1", p.BaseURL)
		}
		if p.Model != "step-custom" {
			t.Errorf("Model = %q, want step-custom", p.Model)
		}
		if p.APIKey != "sk-test" {
			t.Errorf("APIKey = %q, want sk-test", p.APIKey)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewStepFunProviderFromConfig(cp)
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "stepfun") {
			t.Errorf("error should contain prefix 'stepfun', got %v", err)
		}
	})
}

// === 阶跃星辰（StepFun）Anthropic 兼容 ===
func TestNewStepFunAnthropicProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"stepfun_anthropic": map[string]any{
				"api_key": "sk-test",
			},
		}}
		p, err := NewStepFunAnthropicProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewStepFunAnthropicProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://api.stepfun.com/step_plan" {
			t.Errorf("BaseURL = %q, want https://api.stepfun.com/step_plan", p.BaseURL)
		}
		if p.Model != "step-3.7-flash" {
			t.Errorf("Model = %q, want step-3.7-flash", p.Model)
		}
		if p.APIKey != "sk-test" {
			t.Errorf("APIKey = %q, want sk-test", p.APIKey)
		}
	})

	t.Run("override_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"stepfun_anthropic": map[string]any{
				"api_key":  "sk-test",
				"base_url": "https://custom.example.com/anthropic",
				"model":    "step-custom",
			},
		}}
		p, err := NewStepFunAnthropicProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewStepFunAnthropicProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://custom.example.com/anthropic" {
			t.Errorf("BaseURL = %q, want https://custom.example.com/anthropic", p.BaseURL)
		}
		if p.Model != "step-custom" {
			t.Errorf("Model = %q, want step-custom", p.Model)
		}
		if p.APIKey != "sk-test" {
			t.Errorf("APIKey = %q, want sk-test", p.APIKey)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewStepFunAnthropicProviderFromConfig(cp)
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "stepfun_anthropic") {
			t.Errorf("error should contain prefix 'stepfun_anthropic', got %v", err)
		}
	})
}

// === 百度千帆（Qianfan）===
func TestNewBaiduProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"baidu": map[string]any{
				"api_key": "bce-v3/ALTAK-xxxx/xxxx",
			},
		}}
		p, err := NewBaiduProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewBaiduProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://qianfan.baidubce.com/v2" {
			t.Errorf("BaseURL = %q, want https://qianfan.baidubce.com/v2", p.BaseURL)
		}
		if p.Model != "ernie-5.0" {
			t.Errorf("Model = %q, want ernie-5.0", p.Model)
		}
		if p.Name() != "baidu" {
			t.Errorf("Name() = %q, want baidu", p.Name())
		}
		if p.APIKey != "bce-v3/ALTAK-xxxx/xxxx" {
			t.Errorf("APIKey = %q, want bce-v3/ALTAK-xxxx/xxxx", p.APIKey)
		}
		if p.ProviderName != "baidu" {
			t.Errorf("ProviderName = %q, want baidu", p.ProviderName)
		}
	})

	t.Run("override_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"baidu": map[string]any{
				"api_key":  "bce-v3/ALTAK-xxxx/xxxx",
				"base_url": "https://custom.example.com/v2",
				"model":    "ernie-custom",
			},
		}}
		p, err := NewBaiduProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewBaiduProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://custom.example.com/v2" {
			t.Errorf("BaseURL = %q, want https://custom.example.com/v2", p.BaseURL)
		}
		if p.Model != "ernie-custom" {
			t.Errorf("Model = %q, want ernie-custom", p.Model)
		}
		if p.APIKey != "bce-v3/ALTAK-xxxx/xxxx" {
			t.Errorf("APIKey = %q, want bce-v3/ALTAK-xxxx/xxxx", p.APIKey)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewBaiduProviderFromConfig(cp)
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "baidu") {
			t.Errorf("error should contain prefix 'baidu', got %v", err)
		}
	})
}

// === 讯飞星火（Spark）===
func TestNewSparkProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"spark": map[string]any{
				"api_key": "AK123:SK456",
			},
		}}
		p, err := NewSparkProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewSparkProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://spark-api-open.xf-yun.com/agent/v1/" {
			t.Errorf("BaseURL = %q, want https://spark-api-open.xf-yun.com/agent/v1/", p.BaseURL)
		}
		if p.Model != "spark-x" {
			t.Errorf("Model = %q, want spark-x", p.Model)
		}
		if p.Name() != "spark" {
			t.Errorf("Name() = %q, want spark", p.Name())
		}
		if p.APIKey != "AK123:SK456" {
			t.Errorf("APIKey = %q, want AK123:SK456", p.APIKey)
		}
		if p.ProviderName != "spark" {
			t.Errorf("ProviderName = %q, want spark", p.ProviderName)
		}
	})

	t.Run("override_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"spark": map[string]any{
				"api_key":  "AK123:SK456",
				"base_url": "https://custom.example.com/agent/v1/",
				"model":    "spark-custom",
			},
		}}
		p, err := NewSparkProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewSparkProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://custom.example.com/agent/v1/" {
			t.Errorf("BaseURL = %q, want https://custom.example.com/agent/v1/", p.BaseURL)
		}
		if p.Model != "spark-custom" {
			t.Errorf("Model = %q, want spark-custom", p.Model)
		}
		if p.APIKey != "AK123:SK456" {
			t.Errorf("APIKey = %q, want AK123:SK456", p.APIKey)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewSparkProviderFromConfig(cp)
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "spark") {
			t.Errorf("error should contain prefix 'spark', got %v", err)
		}
	})
}

// === 商汤日日新（SenseNova）OpenAI 兼容 ===
func TestNewSenseNovaProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"sensenova": map[string]any{
				"api_key": "sk-test",
			},
		}}
		p, err := NewSenseNovaProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewSenseNovaProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://token.sensenova.cn/v1" {
			t.Errorf("BaseURL = %q, want https://token.sensenova.cn/v1", p.BaseURL)
		}
		if p.Model != "sensenova-6.7-flash-lite" {
			t.Errorf("Model = %q, want sensenova-6.7-flash-lite", p.Model)
		}
		if p.Name() != "sensenova" {
			t.Errorf("Name() = %q, want sensenova", p.Name())
		}
		if p.APIKey != "sk-test" {
			t.Errorf("APIKey = %q, want sk-test", p.APIKey)
		}
		if p.ProviderName != "sensenova" {
			t.Errorf("ProviderName = %q, want sensenova", p.ProviderName)
		}
	})

	t.Run("override_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"sensenova": map[string]any{
				"api_key":  "sk-test",
				"base_url": "https://api.sensenova.cn/compatible-mode/v2",
				"model":    "sensenova-custom",
			},
		}}
		p, err := NewSenseNovaProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewSenseNovaProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://api.sensenova.cn/compatible-mode/v2" {
			t.Errorf("BaseURL = %q, want https://api.sensenova.cn/compatible-mode/v2", p.BaseURL)
		}
		if p.Model != "sensenova-custom" {
			t.Errorf("Model = %q, want sensenova-custom", p.Model)
		}
		if p.APIKey != "sk-test" {
			t.Errorf("APIKey = %q, want sk-test", p.APIKey)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewSenseNovaProviderFromConfig(cp)
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "sensenova") {
			t.Errorf("error should contain prefix 'sensenova', got %v", err)
		}
	})
}

// === 商汤日日新（SenseNova）Anthropic 兼容 ===
func TestNewSenseNovaAnthropicProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"sensenova_anthropic": map[string]any{
				"api_key": "sk-test",
			},
		}}
		p, err := NewSenseNovaAnthropicProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewSenseNovaAnthropicProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://token.sensenova.cn/v1/messages" {
			t.Errorf("BaseURL = %q, want https://token.sensenova.cn/v1/messages", p.BaseURL)
		}
		if p.Model != "sensenova-6.7-flash-lite" {
			t.Errorf("Model = %q, want sensenova-6.7-flash-lite", p.Model)
		}
		if p.APIKey != "sk-test" {
			t.Errorf("APIKey = %q, want sk-test", p.APIKey)
		}
	})

	t.Run("override_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"sensenova_anthropic": map[string]any{
				"api_key":  "sk-test",
				"base_url": "https://api.sensenova.cn/compatible-mode/v1/messages",
				"model":    "sensenova-custom",
			},
		}}
		p, err := NewSenseNovaAnthropicProviderFromConfig(cp)
		if err != nil {
			t.Fatalf("NewSenseNovaAnthropicProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://api.sensenova.cn/compatible-mode/v1/messages" {
			t.Errorf("BaseURL = %q, want https://api.sensenova.cn/compatible-mode/v1/messages", p.BaseURL)
		}
		if p.Model != "sensenova-custom" {
			t.Errorf("Model = %q, want sensenova-custom", p.Model)
		}
		if p.APIKey != "sk-test" {
			t.Errorf("APIKey = %q, want sk-test", p.APIKey)
		}
	})

	t.Run("missing_api_key", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewSenseNovaAnthropicProviderFromConfig(cp)
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "sensenova_anthropic") {
			t.Errorf("error should contain prefix 'sensenova_anthropic', got %v", err)
		}
	})
}
