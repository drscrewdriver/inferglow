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

package contextmgr

import "time"

// Config holds the full configuration for the context manager.
type Config struct {
	// Mode is the context management mode.
	Mode Mode `json:"mode"`

	// WindowTokens is the target context window size in tokens.
	WindowTokens int `json:"window_tokens"`

	// TailKeepSteps is the number of recent steps to keep at L0 (default: 5).
	TailKeepSteps int `json:"tail_keep_steps"`

	// Compression thresholds for effective_decay (128K baseline).
	Thresholds ThresholdConfig `json:"thresholds"`

	// Small model configuration for compression.
	SmallModel ModelConfig `json:"small_model"`

	// Redis configuration for caching.
	Redis RedisConfig `json:"redis"`

	// Long-term memory configuration.
	LongMem LongMemConfig `json:"longmem"`

	// Idle consolidation configuration.
	IdleConsolidation IdleConfig `json:"idle_consolidation"`

	// HotFacts configuration.
	HotFacts HotFactsConfig `json:"hot_facts"`

	// SweetSpotTokens is the sweet-spot threshold (token count).
	// When total prompt tokens are below this value, per-step decay
	// compression is suppressed (passthrough mode) to maximise prefix
	// cache hit rate. 0 or negative = always compress (current behaviour).
	// Recommended: 256000 for 1M context window models.
	SweetSpotTokens int `json:"sweet_spot_tokens"`

	// WarmupRatio triggers async pre-compression when total tokens
	// exceed WarmupRatio × SweetSpotTokens. Default: 0.8.
	WarmupRatio float64 `json:"warmup_ratio"`

	// ToleranceDecayRate is the per-step decay rate for sweet-spot
	// tolerance after a high-reference burst. Default: 0.98.
	ToleranceDecayRate float64 `json:"tolerance_decay_rate"`

	// DriftCheckInterval is the number of steps between semantic drift
	// checks (CM-4). 0 or negative disables drift detection. Default: 5.
	DriftCheckInterval int `json:"drift_check_interval"`

	// DriftThreshold is the Jaccard overlap ratio below which the current
	// step content is considered drifted from Zone 1 background. Default: 0.15.
	DriftThreshold float64 `json:"drift_threshold"`
}

// ThresholdConfig holds compression level thresholds.
type ThresholdConfig struct {
	// L1Tool is the effective_decay threshold for L1 compression on tool results.
	L1Tool int `json:"l1_tool"`
	// L1Reasoning is the threshold for L1 on reasoning.
	L1Reasoning int `json:"l1_reasoning"`
	// L2Tool is the threshold for L2 on tool results.
	L2Tool int `json:"l2_tool"`
	// L2Reasoning is the threshold for L2 on reasoning.
	L2Reasoning int `json:"l2_reasoning"`
	// L3Tool is the threshold for L3 on tool results.
	L3Tool int `json:"l3_tool"`
	// L3Reasoning is the threshold for L3 on reasoning.
	L3Reasoning int `json:"l3_reasoning"`
	// L4Reasoning is the threshold for L4 on reasoning.
	L4Reasoning int `json:"l4_reasoning"`
}

// ModelConfig holds model configuration for compression.
type ModelConfig struct {
	// Endpoint is the model API endpoint.
	Endpoint string `json:"endpoint"`
	// Model is the model name/identifier.
	Model string `json:"model"`
	// Timeout is the request timeout.
	Timeout time.Duration `json:"timeout"`
	// Retries is the number of retries on failure.
	Retries int `json:"retries"`
}

// RedisConfig holds Redis configuration.
type RedisConfig struct {
	// Addr is the Redis address (host:port).
	Addr string `json:"addr"`
	// Password is the Redis password.
	Password string `json:"password"`
	// DB is the Redis database number.
	DB int `json:"db"`
	// Enabled controls whether Redis caching is active.
	Enabled bool `json:"enabled"`
}

// LongMemConfig holds long-term memory configuration.
type LongMemConfig struct {
	// Enabled controls whether long-term memory is active.
	Enabled bool `json:"enabled"`
	// MinRefCount is the minimum ref_count for promotion (default: 5).
	MinRefCount int `json:"min_ref_count"`
	// MinTaskGroups is the minimum number of task groups for promotion (default: 2).
	MinTaskGroups int `json:"min_task_groups"`
	// InitialConfidence is the starting confidence for new memories (default: 0.8).
	InitialConfidence float64 `json:"initial_confidence"`
}

// IdleConfig holds idle consolidation configuration.
type IdleConfig struct {
	// IdleSteps is the number of idle steps before triggering consolidation (default: 10).
	IdleSteps int `json:"idle_steps"`
	// Enabled controls whether idle consolidation is active.
	Enabled bool `json:"enabled"`
}

// HotFactsConfig holds hot facts configuration.
type HotFactsConfig struct {
	// MinRefCount is the minimum ref_count for hot facts (default: 3).
	MinRefCount int `json:"min_ref_count"`
	// MinStrength is the minimum strength for hot facts (default: 1.3).
	MinStrength float64 `json:"min_strength"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Mode:          ModeHybrid,
		WindowTokens:  128000,
		TailKeepSteps: 5,
		Thresholds: ThresholdConfig{
			L1Tool:      16000,
			L1Reasoning: 32000,
			L2Tool:      48000,
			L2Reasoning: 96000,
			L3Tool:      128000,
			L3Reasoning: 256000,
			L4Reasoning: 512000,
		},
		SmallModel: ModelConfig{
			Timeout: 5 * time.Second,
			Retries: 1,
		},
		LongMem: LongMemConfig{
			Enabled:           true,
			MinRefCount:       5,
			MinTaskGroups:     2,
			InitialConfidence: 0.8,
		},
		IdleConsolidation: IdleConfig{
			IdleSteps: 10,
			Enabled:   true,
		},
		HotFacts: HotFactsConfig{
			MinRefCount: 3,
			MinStrength: 1.3,
		},
		SweetSpotTokens:    0, // disabled by default
		WarmupRatio:        0.8,
		ToleranceDecayRate: 0.98,
		DriftCheckInterval: 5,
		DriftThreshold:     0.15,
	}
}
