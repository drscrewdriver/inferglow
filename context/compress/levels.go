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

package compress

import "github.com/inferglow/context"

// LevelThresholds returns the threshold table for 128K baseline window (§6.3).
//
// | 目标级别 | tool 触发     | reasoning 触发   | 窗口占比(tool) |
// |---------|-------------|---------------|--------------|
// | L1      | decay 16K-48K | decay 32K-96K | 12.5%-37.5%  |
// | L2      | decay 48K-128K| decay 96K-256K| 37.5%-100%   |
// | L3      | decay ≥128K   | decay 256K-512K| ≥100%       |
// | L4      | —           | decay ≥512K    | ≥400%        |
func LevelThresholds() contextmgr.ThresholdConfig {
	return contextmgr.ThresholdConfig{
		L1Tool:      16000,
		L1Reasoning: 32000,
		L2Tool:      48000,
		L2Reasoning: 96000,
		L3Tool:      128000,
		L3Reasoning: 256000,
		L4Reasoning: 512000,
	}
}

// TypeConstraintMatrix returns the maximum compression level for each step type (§2.2).
//
// | step 类型             | 最高压缩级别 | 理由                         |
// |----------------------|-----------|------------------------------|
// | tool [result]        | L3 封顶    | 工具输出是事实数据              |
// | tool [call]          | L4        | 调用意图已由推理链覆盖           |
// | reasoning            | L4        | 模型推理可重新生成              |
// | plan / failed        | L4        | 已废弃/已覆盖                  |
// | user                 | L2 封顶    | 用户意图不可丢弃                |
func TypeConstraintMatrix() map[string]int {
	return map[string]int{
		"user":      2,
		"tool":      3,
		"reasoning": 4,
		"plan":      4,
		"failed":    4,
	}
}

// ShouldCompress estimates whether compression is worthwhile for a step.
// Returns true if the estimated token savings exceed the minimum threshold (2K tokens).
func ShouldCompress(originalTokens int) bool {
	return originalTokens >= 4000 // at least 2K savings at 50% reduction
}
