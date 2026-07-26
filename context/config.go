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

	// Decay holds all effective-decay coefficients (A-5/A-6/A-13).
	Decay DecayConfig `json:"decay"`

	// Retrieval holds three-way fusion retrieval settings (A-7).
	Retrieval RetrievalConfig `json:"retrieval"`

	// Backtrack controls same-group backtrack injection (A-9).
	Backtrack BacktrackConfig `json:"backtrack"`
}

// RetrievalConfig holds three-way fusion retrieval settings (A-7). The zero
// value keeps fusion disabled (legacy naive search); use DefaultRetrievalConfig
// for the enabled defaults.
type RetrievalConfig struct {
	// EnableFusion toggles fusion-based Search in the main assembly path.
	// When false (or fusion not built), HybridManager.Search falls back to the
	// legacy naive keyword path (保壳兜底).
	EnableFusion bool `json:"enable_fusion"`
	// Weights are the fusion weights for [semantic, keyword, recency].
	Weights [3]float64 `json:"weights"`
	// Threshold is the minimum fused score to keep a result.
	Threshold float64 `json:"threshold"`
	// RecencyW is the recency-term weight inside the recency score. Default: 0.6.
	RecencyW float64 `json:"recency_w"`
	// StrengthW is the strength-term weight inside the recency score. Default: 0.4.
	StrengthW float64 `json:"strength_w"`
}

// DefaultRetrievalConfig returns the enabled retrieval defaults. EnableFusion
// defaults to true (test phase, no production debt); flip to false to fall back
// to the legacy naive search path.
func DefaultRetrievalConfig() RetrievalConfig {
	return RetrievalConfig{
		EnableFusion: true,
		Weights:      [3]float64{0.50, 0.30, 0.20},
		Threshold:    0.35,
		RecencyW:     0.6,
		StrengthW:    0.4,
	}
}

// DecayConfig externalises every effective-decay coefficient so the decay
// pipeline is observable and tunable without code changes (A-6).
type DecayConfig struct {
	// RawDecayBase is the baseline multiplier on the raw token count. Default: 1.0.
	RawDecayBase float64 `json:"raw_decay_base"`
	// RefModWeight scales the reference-count discount. Default: 0.2.
	RefModWeight float64 `json:"ref_mod_weight"`
	// FileModWeight is the file-mod factor when a related file is actively edited. Default: 0.3.
	FileModWeight float64 `json:"file_mod_weight"`
	// StrengthDivisor normalises the accumulated access strength. Default: 1.0.
	StrengthDivisor float64 `json:"strength_divisor"`
	// Group holds cross-group aging modulation coefficients (A-5).
	Group GroupModConfig `json:"group"`
	// Heat holds heat-dimension (H-axis) modulation coefficients (A-13).
	Heat HeatModConfig `json:"heat"`
}

// GroupModConfig holds cross-group aging modulation coefficients (A-5).
type GroupModConfig struct {
	// Enabled toggles cross-group modulation. When false, groupMod is always 1.0.
	Enabled bool `json:"enabled"`
	// DistanceW is the distance weight per task-group step. Default: 0.3.
	DistanceW float64 `json:"distance_w"`
	// CrossRefW dampens cross-group modulation per cross-group reference. Default: 0.2.
	CrossRefW float64 `json:"cross_ref_w"`
}

// HeatModConfig holds heat-dimension (H-axis) modulation coefficients (A-13).
type HeatModConfig struct {
	// Enabled toggles the heat dimension. When false (or heat==0), heatMod is 1.0.
	Enabled bool `json:"enabled"`
	// RecallBoost is the heat increment per successful recall/citation. Default: 20.
	RecallBoost int `json:"recall_boost"`
	// SigZoneMin is the lower bound of the significant zone (>= SigZoneMin). Default: 70.
	SigZoneMin int `json:"sig_zone_min"`
	// UnsettledMin is the lower bound of the unsettled zone (>= UnsettledMin). Default: 40.
	UnsettledMin int `json:"unsettled_min"`
	// SigMod slows decay in the significant zone. Default: 0.7.
	SigMod float64 `json:"sig_mod"`
	// DecayMod accelerates decay in the decay zone (< UnsettledMin). Default: 1.3.
	DecayMod float64 `json:"decay_mod"`
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
		Decay:              DefaultDecayConfig(),
		Retrieval:          DefaultRetrievalConfig(),
		Backtrack:          DefaultBacktrackConfig(),
	}
}

// DefaultDecayConfig returns the default decay coefficients, replicating the
// historical magic numbers so behaviour is unchanged unless explicitly tuned.
func DefaultDecayConfig() DecayConfig {
	return DecayConfig{
		RawDecayBase:    1.0,
		RefModWeight:    0.2,
		FileModWeight:   0.3,
		StrengthDivisor: 1.0,
		Group: GroupModConfig{
			Enabled:   true,
			DistanceW: 0.3,
			CrossRefW: 0.2,
		},
		Heat: HeatModConfig{
			Enabled:      true,
			RecallBoost:  20,
			SigZoneMin:   70,
			UnsettledMin: 40,
			SigMod:       0.7,
			DecayMod:     1.3,
		},
	}
}

// BacktrackConfig controls same-group backtrack injection (A-9).
type BacktrackConfig struct {
	Enabled         bool    `json:"enabled"`
	TopK            int     `json:"top_k"`
	MaxCharsPerStep int     `json:"max_chars_per_step"`
	RecencyW        float64 `json:"recency_w"`
	StrengthW       float64 `json:"strength_w"`
}

// DefaultBacktrackConfig returns the default backtrack settings (disabled).
func DefaultBacktrackConfig() BacktrackConfig {
	return BacktrackConfig{
		Enabled:         false,
		TopK:            5,
		MaxCharsPerStep: 500,
		RecencyW:        0.6,
		StrengthW:       0.4,
	}
}
