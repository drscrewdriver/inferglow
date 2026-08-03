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

import (
	"testing"
)

func TestLevelThresholds(t *testing.T) {
	thresholds := LevelThresholds()

	if thresholds.L1Tool != 16000 {
		t.Errorf("L1Tool: expected 16000, got %d", thresholds.L1Tool)
	}
	if thresholds.L1Reasoning != 32000 {
		t.Errorf("L1Reasoning: expected 32000, got %d", thresholds.L1Reasoning)
	}
	if thresholds.L2Tool != 48000 {
		t.Errorf("L2Tool: expected 48000, got %d", thresholds.L2Tool)
	}
	if thresholds.L2Reasoning != 96000 {
		t.Errorf("L2Reasoning: expected 96000, got %d", thresholds.L2Reasoning)
	}
	if thresholds.L3Tool != 128000 {
		t.Errorf("L3Tool: expected 128000, got %d", thresholds.L3Tool)
	}
	if thresholds.L3Reasoning != 256000 {
		t.Errorf("L3Reasoning: expected 256000, got %d", thresholds.L3Reasoning)
	}
	if thresholds.L4Reasoning != 512000 {
		t.Errorf("L4Reasoning: expected 512000, got %d", thresholds.L4Reasoning)
	}
}

func TestTypeConstraintMatrix(t *testing.T) {
	matrix := TypeConstraintMatrix()

	tests := []struct {
		stepType string
		expected int
	}{
		{"user", 2},
		{"tool", 3},
		{"reasoning", 4},
		{"plan", 4},
		{"failed", 4},
	}

	for _, tt := range tests {
		got, ok := matrix[tt.stepType]
		if !ok {
			t.Errorf("step type %q not found in matrix", tt.stepType)
			continue
		}
		if got != tt.expected {
			t.Errorf("step type %q: expected max level %d, got %d", tt.stepType, tt.expected, got)
		}
	}

	// Should have exactly 5 entries
	if len(matrix) != 5 {
		t.Errorf("expected 5 entries in matrix, got %d", len(matrix))
	}
}
