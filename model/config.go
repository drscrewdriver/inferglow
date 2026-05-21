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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// ConfigProvider 配置提供者接口
// 支持按 dot-path 风格的 key 读取配置值
type ConfigProvider interface {
	Get(key string) (any, bool)
}

// EnvConfigProvider 环境变量配置提供者
// Get("openai.api_key") + Prefix "INFERGLOW_" → 查找 INFERGLOW_OPENAI_API_KEY
type EnvConfigProvider struct {
	Prefix string
}

// Get 从环境变量读取配置
// 转换规则: dot → underscore, uppercase
func (p *EnvConfigProvider) Get(key string) (any, bool) {
	if key == "" {
		return nil, false
	}
	envName := p.Prefix + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	val, ok := os.LookupEnv(envName)
	if !ok {
		return nil, false
	}
	return val, true
}

// FileConfigProvider 文件配置提供者（支持 .yaml/.yml 和 .json）
// 文件不存在时 Get 始终返回 (nil, false)，不 panic
type FileConfigProvider struct {
	Path string

	once   sync.Once
	data   map[string]any
	loaded bool
}

// Get 从文件配置按 dot-path 读取
func (p *FileConfigProvider) Get(key string) (any, bool) {
	if key == "" {
		return nil, false
	}
	p.once.Do(func() {
		p.data, p.loaded = p.loadFile()
	})
	if !p.loaded || p.data == nil {
		return nil, false
	}
	return traverseMap(p.data, key)
}

func (p *FileConfigProvider) loadFile() (map[string]any, bool) {
	if p.Path == "" {
		return nil, false
	}
	content, err := os.ReadFile(p.Path)
	if err != nil {
		return nil, false
	}

	result := make(map[string]any)
	ext := strings.ToLower(filepath.Ext(p.Path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(content, &result); err != nil {
			return nil, false
		}
	case ".json":
		if err := json.Unmarshal(content, &result); err != nil {
			return nil, false
		}
	default:
		// 默认尝试 JSON
		if err := json.Unmarshal(content, &result); err != nil {
			return nil, false
		}
	}
	return result, true
}

// StaticConfigProvider 静态 map 配置提供者
// 可作为最高优先级覆盖层注入 CompositeConfigProvider
type StaticConfigProvider struct {
	Values map[string]any
}

// Get 从 Values map 按 dot-path 查找
func (p *StaticConfigProvider) Get(key string) (any, bool) {
	if key == "" || p.Values == nil {
		return nil, false
	}
	return traverseMap(p.Values, key)
}

// CompositeConfigProvider 组合多个 ConfigProvider
// Providers 索引越大优先级越高（后加入的优先）
type CompositeConfigProvider struct {
	Providers []ConfigProvider
}

// NewComposite 创建 CompositeConfigProvider
// providers 顺序：前者优先级低，后者优先级高
func NewComposite(providers ...ConfigProvider) *CompositeConfigProvider {
	return &CompositeConfigProvider{Providers: providers}
}

// Get 按 Providers 顺序查询（索引小优先级低），返回首个命中
// 即从后往前遍历，第一个命中的值即为返回值
func (c *CompositeConfigProvider) Get(key string) (any, bool) {
	for i := len(c.Providers) - 1; i >= 0; i-- {
		if v, ok := c.Providers[i].Get(key); ok {
			return v, true
		}
	}
	return nil, false
}

// traverseMap 按 dot-path 遍历嵌套 map[string]any
func traverseMap(m map[string]any, key string) (any, bool) {
	parts := strings.Split(key, ".")
	var current any = m
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
		cm, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		val, ok := cm[part]
		if !ok {
			return nil, false
		}
		current = val
	}
	return current, true
}

// ProviderConfig Provider 配置集合
type ProviderConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
	Settings   map[string]any
}

// ErrMissingRequiredConfig 必填配置缺失错误
var ErrMissingRequiredConfig = errors.New("missing required config")

// ModelRequest.Options 支持的深度思考参数键（G1-03）：
//   - "thinking": map[string]string{"type": "enabled"|"disabled"}
//     MiMo/Stepfun/Sensenova 的非标准思考开关，通过 Options 透传至请求体。
//   - "reasoning_effort": "low"|"medium"|"high"
//     OpenAI o-series 的推理力度参数，通过 Options 透传至请求体。
//
// 上述键均不在 reservedFields 中，RequestModel 会原样写入请求体顶层。
// 推理内容预算（G1-05）通过 OpenAICompatibleProvider.MaxReasoningTokens 字段
// 配置，而非 Options。

// DEFAULT_SETTINGS 各 Provider 的默认配置
// 优先级：DEFAULT_SETTINGS < cp.Get（配置文件/环境变量/静态覆盖）
var DEFAULT_SETTINGS = map[string]map[string]any{
	"openai": {
		"model":       "gpt-4",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
	"anthropic": {
		"model":      "claude-3-5-sonnet-20241022",
		"max_tokens": 1024,
	},
	"ollama": {
		"model":    "llama3",
		"base_url": "http://localhost:11434",
	},
	"deepseek": {
		"model":       "deepseek-chat",
		"base_url":    "https://api.deepseek.com/v1",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
	"qwen": {
		"model":       "qwen-max",
		"base_url":    "https://dashscope.aliyuncs.com/compatible-mode/v1",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
	"glm": {
		"model":       "glm-4",
		"base_url":    "https://open.bigmodel.cn/api/paas/v4",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
	"kimi": {
		"model":       "moonshot-v1-8k",
		"base_url":    "https://api.moonshot.cn/v1",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
	// === 阶跃星辰（StepFun）===
	"stepfun": {
		"model":       "step-3.7-flash",
		"base_url":    "https://api.stepfun.com/v1",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
	"stepfun_anthropic": {
		"model":      "step-3.7-flash",
		"base_url":   "https://api.stepfun.com/step_plan",
		"max_tokens": 1024,
	},
	// === 百度千帆（Qianfan）===
	"baidu": {
		"model":       "ernie-5.0",
		"base_url":    "https://qianfan.baidubce.com/v2",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
	// === 讯飞星火（Spark）===
	"spark": {
		"model":       "spark-x",
		"base_url":    "https://spark-api-open.xf-yun.com/agent/v1/",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
	// === 商汤日日新（SenseNova）===
	"sensenova": {
		"model":       "sensenova-6.7-flash-lite",
		"base_url":    "https://token.sensenova.cn/v1",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
	"sensenova_anthropic": {
		"model":      "sensenova-6.7-flash-lite",
		"base_url":   "https://token.sensenova.cn/v1/messages",
		"max_tokens": 1024,
	},
	// === 小米 MiMo（OpenAI 兼容）===
	// 推理字段为 reasoning_content（非 reasoning），思考参数 thinking.type
	// 通过 Options 透传。详见 G1-02 / G1-03。
	"mimo": {
		"model":       "mimo-v2.5-pro",
		"base_url":    "https://api.xiaomimimo.com/v1",
		"temperature": 1.0,
		"top_p":       0.95,
		"max_tokens":  4096,
	},
	// === 小米 MiMo（Anthropic 兼容）===
	"mimo_anthropic": {
		"model":      "mimo-v2.5-pro",
		"base_url":   "https://api.xiaomimimo.com/anthropic",
		"max_tokens": 1024,
	},
	// === 腾讯混元 ===
	"tencent": {
		"model":       "hunyuan-max",
		"base_url":    "https://api.hunyuan.cloud.tencent.com/v1",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
	// === 字节豆包（火山引擎）===
	"volcengine": {
		"model":       "doubao-seed-2-0-pro",
		"base_url":    "https://ark.cn-beijing.volces.com/api/v3",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
	// === 零一万物 ===
	"zeroone": {
		"model":       "yi-lightning",
		"base_url":    "https://api.01.ai/v1",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
	// === MiniMax（稀宇科技）===
	"minimax": {
		"model":       "MiniMax-M2.7",
		"base_url":    "https://api.minimax.chat/v1",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
	// === 硅基流动（聚合平台）===
	// 一个 Key 可调用多个开源模型（Qwen/GLM/DeepSeek 等）
	"siliconflow": {
		"model":       "Qwen/Qwen2.5-72B-Instruct",
		"base_url":    "https://api.siliconflow.cn/v1",
		"temperature": 0.7,
		"max_tokens":  4096,
	},
}

// LoadProviderConfig 从 ConfigProvider 加载 Provider 配置
// 读取 <prefix>.base_url, <prefix>.api_key, <prefix>.model
// 默认值优先级：DEFAULT_SETTINGS < cp.Get（配置文件/环境变量/静态覆盖）
// APIKey 缺失返回 ErrMissingRequiredConfig（包装错误信息包含 prefix）
// Settings 收集 <prefix>.settings 整个子树（可选，无则 nil）
func LoadProviderConfig(cp ConfigProvider, prefix string) (*ProviderConfig, error) {
	if cp == nil {
		return nil, fmt.Errorf("%w: config provider is nil for prefix %q", ErrMissingRequiredConfig, prefix)
	}

	cfg := &ProviderConfig{}

	// 1. 从 DEFAULT_SETTINGS 获取默认值
	defaults := DEFAULT_SETTINGS[prefix]
	if defaults == nil {
		defaults = make(map[string]any)
	}

	// 2. 应用默认值
	if v, ok := defaults["model"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.Model = s
		}
	}
	if v, ok := defaults["base_url"]; ok {
		if s, ok := v.(string); ok && s != "" {
			cfg.BaseURL = s
		}
	}

	// 3. 用 cp.Get 覆盖默认值
	if v, ok := cp.Get(prefix + ".base_url"); ok {
		if s, ok := v.(string); ok {
			cfg.BaseURL = s
		}
	}

	if v, ok := cp.Get(prefix + ".api_key"); ok {
		if s, ok := v.(string); ok {
			cfg.APIKey = s
		}
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("%w: %s.api_key is required", ErrMissingRequiredConfig, prefix)
	}

	if v, ok := cp.Get(prefix + ".model"); ok {
		if s, ok := v.(string); ok {
			cfg.Model = s
		}
	}

	// 收集 <prefix>.settings 整个子树
	if v, ok := cp.Get(prefix + ".settings"); ok {
		if m, ok := v.(map[string]any); ok {
			cfg.Settings = m
		}
	}

	return cfg, nil
}
