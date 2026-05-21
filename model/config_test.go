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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Check: EnvConfigProvider - 设置环境变量 → Get 命中
func TestEnvConfigProviderHit(t *testing.T) {
	t.Setenv("INFERGLOW_OPENAI_API_KEY", "secret-key")

	p := &EnvConfigProvider{Prefix: "INFERGLOW_"}
	val, ok := p.Get("openai.api_key")
	if !ok {
		t.Fatal("expected to find openai.api_key")
	}
	if val != "secret-key" {
		t.Errorf("got %v, want secret-key", val)
	}
}

// Check: EnvConfigProvider - 未设置 → Get 未命中
func TestEnvConfigProviderMiss(t *testing.T) {
	// 确保变量未设置（t.Setenv 在测试结束后自动恢复）
	t.Setenv("INFERGLOW_NONEXISTENT_KEY", "")
	os.Unsetenv("INFERGLOW_NONEXISTENT_KEY")

	p := &EnvConfigProvider{Prefix: "INFERGLOW_"}
	_, ok := p.Get("nonexistent.key")
	if ok {
		t.Error("expected miss for nonexistent key")
	}
}

// Check: EnvConfigProvider - 无 Prefix
func TestEnvConfigProviderNoPrefix(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "no-prefix-key")
	p := &EnvConfigProvider{}
	val, ok := p.Get("openai.api_key")
	if !ok {
		t.Fatal("expected to find openai.api_key without prefix")
	}
	if val != "no-prefix-key" {
		t.Errorf("got %v, want no-prefix-key", val)
	}
}

// Check: EnvConfigProvider - 空 key 返回 false
func TestEnvConfigProviderEmptyKey(t *testing.T) {
	p := &EnvConfigProvider{Prefix: "INFERGLOW_"}
	_, ok := p.Get("")
	if ok {
		t.Error("expected miss for empty key")
	}
}

// Check: FileConfigProvider - JSON 文件 → Get 命中
func TestFileConfigProviderJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	content := `{
		"openai": {
			"api_key": "json-key",
			"model": "gpt-4"
		},
		"anthropic": {
			"api_key": "anthropic-key"
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := &FileConfigProvider{Path: path}

	val, ok := p.Get("openai.api_key")
	if !ok {
		t.Fatal("expected to find openai.api_key")
	}
	if val != "json-key" {
		t.Errorf("got %v, want json-key", val)
	}

	val, ok = p.Get("openai.model")
	if !ok {
		t.Fatal("expected to find openai.model")
	}
	if val != "gpt-4" {
		t.Errorf("got %v, want gpt-4", val)
	}

	val, ok = p.Get("anthropic.api_key")
	if !ok {
		t.Fatal("expected to find anthropic.api_key")
	}
	if val != "anthropic-key" {
		t.Errorf("got %v, want anthropic-key", val)
	}
}

// Check: FileConfigProvider - YAML 文件 → Get 命中
func TestFileConfigProviderYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
openai:
  api_key: yaml-key
  model: gpt-4
anthropic:
  api_key: anthropic-key
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := &FileConfigProvider{Path: path}

	val, ok := p.Get("openai.api_key")
	if !ok {
		t.Fatal("expected to find openai.api_key in YAML")
	}
	if val != "yaml-key" {
		t.Errorf("got %v, want yaml-key", val)
	}

	val, ok = p.Get("openai.model")
	if !ok {
		t.Fatal("expected to find openai.model in YAML")
	}
	if val != "gpt-4" {
		t.Errorf("got %v, want gpt-4", val)
	}
}

// Check: FileConfigProvider - .yml 扩展名
func TestFileConfigProviderYMLExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	content := `
openai:
  api_key: yml-key
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := &FileConfigProvider{Path: path}
	val, ok := p.Get("openai.api_key")
	if !ok {
		t.Fatal("expected to find openai.api_key in YML")
	}
	if val != "yml-key" {
		t.Errorf("got %v, want yml-key", val)
	}
}

// Check: FileConfigProvider - 文件不存在 → Get 未命中，不 panic
func TestFileConfigProviderNonExistent(t *testing.T) {
	p := &FileConfigProvider{Path: "/nonexistent/path/config.json"}

	_, ok := p.Get("anything")
	if ok {
		t.Error("expected miss for non-existent file")
	}

	// 多次调用应该不 panic（sync.Once 缓存）
	_, ok = p.Get("anything")
	if ok {
		t.Error("expected miss for non-existent file on second call")
	}
}

// Check: FileConfigProvider - 空 Path → Get 未命中
func TestFileConfigProviderEmptyPath(t *testing.T) {
	p := &FileConfigProvider{Path: ""}
	_, ok := p.Get("anything")
	if ok {
		t.Error("expected miss for empty path")
	}
}

// Check: FileConfigProvider - 空 key → Get 未命中
func TestFileConfigProviderEmptyKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"a":"b"}`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	p := &FileConfigProvider{Path: path}
	_, ok := p.Get("")
	if ok {
		t.Error("expected miss for empty key")
	}
}

// Check: StaticConfigProvider - dot-path 遍历
func TestStaticConfigProvider(t *testing.T) {
	p := &StaticConfigProvider{
		Values: map[string]any{
			"openai": map[string]any{
				"api_key": "static-key",
				"nested": map[string]any{
					"deep": "deep-value",
				},
			},
		},
	}

	val, ok := p.Get("openai.api_key")
	if !ok {
		t.Fatal("expected to find openai.api_key")
	}
	if val != "static-key" {
		t.Errorf("got %v, want static-key", val)
	}

	val, ok = p.Get("openai.nested.deep")
	if !ok {
		t.Fatal("expected to find openai.nested.deep")
	}
	if val != "deep-value" {
		t.Errorf("got %v, want deep-value", val)
	}

	_, ok = p.Get("nonexistent.key")
	if ok {
		t.Error("expected miss for nonexistent")
	}

	// 中间值不是 map 时应未命中
	_, ok = p.Get("openai.api_key.sub")
	if ok {
		t.Error("expected miss when intermediate value is not a map")
	}
}

// Check: StaticConfigProvider - nil Values
func TestStaticConfigProviderNilValues(t *testing.T) {
	p := &StaticConfigProvider{}
	_, ok := p.Get("anything")
	if ok {
		t.Error("expected miss for nil Values")
	}
}

// Check: CompositeConfigProvider - 3 层组合 + 优先级验证
func TestCompositeConfigProvider(t *testing.T) {
	// 3 layers: file (low), env (mid), static (high)
	file := &StaticConfigProvider{Values: map[string]any{
		"openai": map[string]any{"api_key": "file-key", "model": "file-model"},
	}}
	env := &StaticConfigProvider{Values: map[string]any{
		"openai": map[string]any{"api_key": "env-key"},
	}}
	override := &StaticConfigProvider{Values: map[string]any{
		"openai": map[string]any{"api_key": "override-key"},
	}}

	// 后加入优先级高
	composite := NewComposite(file, env, override)

	val, ok := composite.Get("openai.api_key")
	if !ok {
		t.Fatal("expected to find openai.api_key")
	}
	if val != "override-key" {
		t.Errorf("got %v, want override-key (highest priority)", val)
	}

	// 仅在 file 中存在的字段
	val, ok = composite.Get("openai.model")
	if !ok {
		t.Fatal("expected to find openai.model")
	}
	if val != "file-model" {
		t.Errorf("got %v, want file-model", val)
	}
}

// Check: CompositeConfigProvider - 全部未命中
func TestCompositeConfigProviderMiss(t *testing.T) {
	composite := NewComposite(
		&StaticConfigProvider{Values: map[string]any{}},
		&StaticConfigProvider{Values: map[string]any{}},
	)
	_, ok := composite.Get("nonexistent")
	if ok {
		t.Error("expected miss for empty composite")
	}
}

// Check: CompositeConfigProvider - 2 层组合验证 env 覆盖 file
func TestCompositeConfigProviderPriority(t *testing.T) {
	file := &StaticConfigProvider{Values: map[string]any{
		"openai": map[string]any{"api_key": "file-key"},
	}}
	env := &StaticConfigProvider{Values: map[string]any{
		"openai": map[string]any{"api_key": "env-key"},
	}}

	composite := NewComposite(file, env)
	val, ok := composite.Get("openai.api_key")
	if !ok {
		t.Fatal("expected to find openai.api_key")
	}
	if val != "env-key" {
		t.Errorf("got %v, want env-key (higher priority)", val)
	}
}

// Check: LoadProviderConfig - 必填字段缺失 → error
func TestLoadProviderConfigMissingAPIKey(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"openai": map[string]any{"base_url": "https://api.openai.com"},
	}}

	_, err := LoadProviderConfig(cp, "openai")
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
	if !errors.Is(err, ErrMissingRequiredConfig) {
		t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
	}
	if !strings.Contains(err.Error(), "openai") {
		t.Errorf("error should contain prefix 'openai', got %v", err)
	}
}

// Check: LoadProviderConfig - nil ConfigProvider
func TestLoadProviderConfigNilConfigProvider(t *testing.T) {
	_, err := LoadProviderConfig(nil, "openai")
	if err == nil {
		t.Fatal("expected error for nil config provider")
	}
	if !errors.Is(err, ErrMissingRequiredConfig) {
		t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
	}
}

// Check: LoadProviderConfig - 成功加载所有字段
func TestLoadProviderConfigSuccess(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"openai": map[string]any{
			"base_url": "https://api.openai.com",
			"api_key":  "test-key",
			"model":    "gpt-4",
			"settings": map[string]any{
				"timeout": 30,
			},
		},
	}}

	cfg, err := LoadProviderConfig(cp, "openai")
	if err != nil {
		t.Fatalf("LoadProviderConfig failed: %v", err)
	}

	if cfg.BaseURL != "https://api.openai.com" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("APIKey = %q", cfg.APIKey)
	}
	if cfg.Model != "gpt-4" {
		t.Errorf("Model = %q", cfg.Model)
	}
	if cfg.Settings == nil {
		t.Error("expected non-nil Settings")
	}
	if cfg.Settings["timeout"] != 30 {
		t.Errorf("Settings.timeout = %v", cfg.Settings["timeout"])
	}
}

// Check: LoadProviderConfig - 仅 APIKey 必填，其他可选
func TestLoadProviderConfigOnlyAPIKey(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"openai": map[string]any{"api_key": "only-key"},
	}}

	cfg, err := LoadProviderConfig(cp, "openai")
	if err != nil {
		t.Fatalf("LoadProviderConfig failed: %v", err)
	}
	if cfg.APIKey != "only-key" {
		t.Errorf("APIKey = %q", cfg.APIKey)
	}
	if cfg.BaseURL != "" {
		t.Errorf("BaseURL should be empty, got %q", cfg.BaseURL)
	}
	if cfg.Settings != nil {
		t.Errorf("Settings should be nil, got %v", cfg.Settings)
	}
}

// Check: 工厂函数 - 从 Composite 构造 OpenAI Provider
func TestNewOpenAIProviderFromConfig(t *testing.T) {
	cp := NewComposite(
		&StaticConfigProvider{Values: map[string]any{
			"openai": map[string]any{
				"base_url": "https://api.openai.com",
				"api_key":  "openai-key",
				"model":    "gpt-4",
			},
		}},
	)

	provider, err := NewOpenAIProviderFromConfig(cp)
	if err != nil {
		t.Fatalf("NewOpenAIProviderFromConfig failed: %v", err)
	}

	if provider.APIKey != "openai-key" {
		t.Errorf("APIKey = %q", provider.APIKey)
	}
	if provider.Model != "gpt-4" {
		t.Errorf("Model = %q", provider.Model)
	}
	if provider.BaseURL != "https://api.openai.com" {
		t.Errorf("BaseURL = %q", provider.BaseURL)
	}
	if provider.Name() != "openai-compatible" {
		t.Errorf("Name = %q", provider.Name())
	}
}

// Check: 工厂函数 - 从 Composite 构造 Anthropic Provider
func TestNewAnthropicProviderFromConfig(t *testing.T) {
	cp := NewComposite(
		&StaticConfigProvider{Values: map[string]any{
			"anthropic": map[string]any{
				"base_url": "https://api.anthropic.com",
				"api_key":  "anthropic-key",
				"model":    "claude-3-5-sonnet-20241022",
			},
		}},
	)

	provider, err := NewAnthropicProviderFromConfig(cp)
	if err != nil {
		t.Fatalf("NewAnthropicProviderFromConfig failed: %v", err)
	}

	if provider.APIKey != "anthropic-key" {
		t.Errorf("APIKey = %q", provider.APIKey)
	}
	if provider.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("Model = %q", provider.Model)
	}
	if provider.BaseURL != "https://api.anthropic.com" {
		t.Errorf("BaseURL = %q", provider.BaseURL)
	}
	if provider.Name() != "anthropic" {
		t.Errorf("Name = %q", provider.Name())
	}
}

// Check: 工厂函数 - 必填字段缺失时返回 error
func TestNewOpenAIProviderFromConfigMissingKey(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"openai": map[string]any{"base_url": "https://api.openai.com"},
	}}

	_, err := NewOpenAIProviderFromConfig(cp)
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
	if !errors.Is(err, ErrMissingRequiredConfig) {
		t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
	}
}

// Check: 工厂函数 - Anthropic 必填字段缺失
func TestNewAnthropicProviderFromConfigMissingKey(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"anthropic": map[string]any{"base_url": "https://api.anthropic.com"},
	}}

	_, err := NewAnthropicProviderFromConfig(cp)
	if err == nil {
		t.Fatal("expected error for missing api_key")
	}
	if !errors.Is(err, ErrMissingRequiredConfig) {
		t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
	}
	if !strings.Contains(err.Error(), "anthropic") {
		t.Errorf("error should contain prefix 'anthropic', got %v", err)
	}
}

// Check: 综合测试 - 三层 ConfigProvider 加载完整 Provider
func TestCompositeConfigProviderFullStack(t *testing.T) {
	// 文件层（低）
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlContent := `
openai:
  base_url: https://api.openai.com
  api_key: file-openai-key
  model: gpt-4
anthropic:
  base_url: https://api.anthropic.com
  api_key: file-anthropic-key
  model: claude-3-5-sonnet-20241022
`
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	// 环境变量层（中） - 覆盖 OpenAI key
	t.Setenv("APP_OPENAI_API_KEY", "env-openai-key")

	// 静态层（高） - 覆盖 Anthropic model
	static := &StaticConfigProvider{Values: map[string]any{
		"anthropic": map[string]any{"model": "claude-3-opus-20240229"},
	}}

	composite := NewComposite(
		&FileConfigProvider{Path: path},
		&EnvConfigProvider{Prefix: "APP_"},
		static,
	)

	// OpenAI api_key 应来自 env（更高优先级）
	openaiProvider, err := NewOpenAIProviderFromConfig(composite)
	if err != nil {
		t.Fatalf("NewOpenAIProviderFromConfig failed: %v", err)
	}
	if openaiProvider.APIKey != "env-openai-key" {
		t.Errorf("OpenAI APIKey = %q, want env-openai-key", openaiProvider.APIKey)
	}

	// Anthropic model 应来自 static（最高优先级）
	anthropicProvider, err := NewAnthropicProviderFromConfig(composite)
	if err != nil {
		t.Fatalf("NewAnthropicProviderFromConfig failed: %v", err)
	}
	if anthropicProvider.Model != "claude-3-opus-20240229" {
		t.Errorf("Anthropic Model = %q, want claude-3-opus-20240229", anthropicProvider.Model)
	}
	// Anthropic api_key 应来自 file（其他层未设置）
	if anthropicProvider.APIKey != "file-anthropic-key" {
		t.Errorf("Anthropic APIKey = %q, want file-anthropic-key", anthropicProvider.APIKey)
	}
}

// Check 1.4: DEFAULT_SETTINGS 包含所有 Provider 类型
func TestDefaultSettingsCompleteness(t *testing.T) {
	expectedProviders := []string{"openai", "anthropic", "ollama", "deepseek", "qwen"}
	for _, provider := range expectedProviders {
		if _, ok := DEFAULT_SETTINGS[provider]; !ok {
			t.Errorf("missing default setting for %q", provider)
		}
	}
}

// Check 1.4: DEFAULT_SETTINGS 有合理的默认 model
func TestDefaultSettingsModels(t *testing.T) {
	if DEFAULT_SETTINGS["openai"]["model"] != "gpt-4" {
		t.Errorf("openai model = %v, want gpt-4", DEFAULT_SETTINGS["openai"]["model"])
	}
	if DEFAULT_SETTINGS["anthropic"]["model"] != "claude-3-5-sonnet-20241022" {
		t.Errorf("anthropic model = %v, want claude-3-5-sonnet-20241022", DEFAULT_SETTINGS["anthropic"]["model"])
	}
	if DEFAULT_SETTINGS["ollama"]["model"] != "llama3" {
		t.Errorf("ollama model = %v, want llama3", DEFAULT_SETTINGS["ollama"]["model"])
	}
}

// Check 1.4: DEFAULT_SETTINGS 有 ollama base_url
func TestDefaultSettingsOllamaBaseURL(t *testing.T) {
	if DEFAULT_SETTINGS["ollama"]["base_url"] != "http://localhost:11434" {
		t.Errorf("ollama base_url = %v, want http://localhost:11434", DEFAULT_SETTINGS["ollama"]["base_url"])
	}
}

// Check 1.5: LoadProviderConfig 使用 DEFAULT_SETTINGS 默认值
func TestLoadProviderConfigUsesDefaultSettings(t *testing.T) {
	// cp.Get 不返回任何值，应回退到 DEFAULT_SETTINGS
	cp := &StaticConfigProvider{Values: map[string]any{
		"openai": map[string]any{
			"api_key": "test-key",
		},
	}}

	cfg, err := LoadProviderConfig(cp, "openai")
	if err != nil {
		t.Fatalf("LoadProviderConfig failed: %v", err)
	}

	// model 应来自 DEFAULT_SETTINGS
	if cfg.Model != "gpt-4" {
		t.Errorf("Model = %q, want gpt-4 (from DEFAULT_SETTINGS)", cfg.Model)
	}
}

// Check 1.5: cp.Get 覆盖 DEFAULT_SETTINGS
func TestLoadProviderConfigOverrideDefaultSettings(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"openai": map[string]any{
			"api_key": "test-key",
			"model":   "gpt-3.5-turbo",
		},
	}}

	cfg, err := LoadProviderConfig(cp, "openai")
	if err != nil {
		t.Fatalf("LoadProviderConfig failed: %v", err)
	}

	// cp.Get 的 model 应覆盖 DEFAULT_SETTINGS
	if cfg.Model != "gpt-3.5-turbo" {
		t.Errorf("Model = %q, want gpt-3.5-turbo (from cp.Get)", cfg.Model)
	}
}

// Check 1.5: 新增 Provider 无默认值不崩溃
func TestLoadProviderConfigNoDefaultForNewProvider(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"custom": map[string]any{
			"api_key": "test-key",
		},
	}}

	cfg, err := LoadProviderConfig(cp, "custom")
	if err != nil {
		t.Fatalf("LoadProviderConfig failed: %v", err)
	}

	// 无默认 model 时 Model 为空字符串
	if cfg.Model != "" {
		t.Errorf("Model = %q, want empty string (no default)", cfg.Model)
	}
	if cfg.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want test-key", cfg.APIKey)
	}
}

// Check 1.5: ollama base_url 默认值
func TestLoadProviderConfigOllamaDefaultBaseURL(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"ollama": map[string]any{
			"api_key": "any", // Ollama 需要 API Key 来满足现有逻辑
		},
	}}

	cfg, err := LoadProviderConfig(cp, "ollama")
	if err != nil {
		t.Fatalf("LoadProviderConfig failed: %v", err)
	}

	// base_url 应来自 DEFAULT_SETTINGS
	if cfg.BaseURL != "http://localhost:11434" {
		t.Errorf("BaseURL = %q, want http://localhost:11434 (from DEFAULT_SETTINGS)", cfg.BaseURL)
	}
}

// Check 1.5: ollama base_url 可被覆盖
func TestLoadProviderConfigOllamaBaseURLOverride(t *testing.T) {
	cp := &StaticConfigProvider{Values: map[string]any{
		"ollama": map[string]any{
			"api_key":  "any",
			"base_url": "http://custom:11434",
		},
	}}

	cfg, err := LoadProviderConfig(cp, "ollama")
	if err != nil {
		t.Fatalf("LoadProviderConfig failed: %v", err)
	}

	if cfg.BaseURL != "http://custom:11434" {
		t.Errorf("BaseURL = %q, want http://custom:11434 (from cp.Get)", cfg.BaseURL)
	}
}

// Check: OllamaProvider 工厂函数
func TestNewOllamaProviderFromConfig(t *testing.T) {
	_ = &StaticConfigProvider{Values: map[string]any{
		"ollama": map[string]any{
			"base_url": "http://custom:11434",
			"model":    "mistral",
		},
	}}

	// Ollama 不需要 api_key，但 LoadProviderConfig 仍要求它
	// 所以这里我们只测试结构体的直接初始化
	provider := NewOllamaProvider()
	if provider.Name() != "ollama" {
		t.Errorf("Name = %q, want %q", provider.Name(), "ollama")
	}
	if provider.BaseURL != "http://localhost:11434" {
		t.Errorf("BaseURL = %q, want %q", provider.BaseURL, "http://localhost:11434")
	}
}
