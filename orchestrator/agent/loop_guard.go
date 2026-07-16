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

package agent

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/inferglow/orchestrator/actionruntime"
)

// VerdictAction is the action recommendation from LoopGuard.Check.
type VerdictAction string

const (
	// VerdictContinue allows the agent loop to keep running.
	VerdictContinue VerdictAction = "continue"
	// VerdictBreak stops the agent loop immediately.
	VerdictBreak VerdictAction = "break"
	// VerdictDegrade continues the loop in a degraded mode.
	VerdictDegrade VerdictAction = "degrade"
)

// LoopGuardConfig controls LoopGuard behavior. Zero-valued fields are replaced
// with sensible defaults by NewLoopGuard.
type LoopGuardConfig struct {
	Disabled                  bool
	RepeatActionWindow        int           // default 3
	OutputStagnationWindow    int           // default 3
	OutputSimilarityThreshold float64       // default 0.9
	TimeBudget                time.Duration // default 5*time.Minute
	TokenBudget               int           // default 100000
	// ReasoningPrefixLen is the number of leading characters used for
	// reasoning stagnation prefix matching. When reasoning output at round
	// N equals N+1, and N+2 starts with the same prefix, the guard breaks.
	// Default 80.
	ReasoningPrefixLen int
}

// LoopGuardState is the per-round state passed to Check.
type LoopGuardState struct {
	Round         int
	ActionCalls   []actionruntime.ActionCall
	LastOutput    string
	LastReasoning string // reasoning/thinking content from the latest LLM round
	TotalTokens   int
	StartedAt     time.Time
}

// LoopGuardVerdict is the result of Check.
type LoopGuardVerdict struct {
	Action VerdictAction
	Reason string
}

// LoopGuard detects Agent death-loop patterns by maintaining sliding windows
// over recent rounds of action calls and LLM outputs.
type LoopGuard struct {
	cfg          LoopGuardConfig
	actionWindow [][]actionruntime.ActionCall
	outputWindow []string
	startedAt    time.Time
	totalTokens  int

	// Reasoning stagnation tracking: prevReasoning holds round N-1 reasoning,
	// reasoningEqualStreak counts consecutive rounds where reasoning == prev.
	prevReasoning        string
	reasoningEqualStreak int
}

// ErrLoopDetected is the sentinel error returned by Engine.executeLoop when
// LoopGuard recommends breaking out of the loop.
var ErrLoopDetected = errors.New("loop detected")

// NewLoopGuard constructs a LoopGuard with the given config, applying defaults
// for any zero-valued fields. If cfg.Disabled is true, Check always returns
// Continue without inspecting state.
func NewLoopGuard(cfg LoopGuardConfig) *LoopGuard {
	if cfg.RepeatActionWindow <= 0 {
		cfg.RepeatActionWindow = 3
	}
	if cfg.OutputStagnationWindow <= 0 {
		cfg.OutputStagnationWindow = 3
	}
	if cfg.OutputSimilarityThreshold <= 0 {
		cfg.OutputSimilarityThreshold = 0.9
	}
	if cfg.TimeBudget <= 0 {
		cfg.TimeBudget = 5 * time.Minute
	}
	if cfg.TokenBudget <= 0 {
		cfg.TokenBudget = 100000
	}
	if cfg.ReasoningPrefixLen <= 0 {
		cfg.ReasoningPrefixLen = 80
	}
	return &LoopGuard{cfg: cfg}
}

// Check inspects the current round state and returns a verdict recommending
// continue, break, or degrade. Checks run in a fixed priority order:
// RepeatAction → OutputStagnation → ReasoningStagnation → TimeBudget →
// TokenBudget. The first matching check wins.
func (g *LoopGuard) Check(state LoopGuardState) (*LoopGuardVerdict, error) {
	if g.cfg.Disabled {
		return &LoopGuardVerdict{Action: VerdictContinue}, nil
	}

	// Capture the run start time on first observation.
	if g.startedAt.IsZero() {
		if !state.StartedAt.IsZero() {
			g.startedAt = state.StartedAt
		} else {
			g.startedAt = time.Now()
		}
	}
	g.totalTokens = state.TotalTokens

	// Append this round's data and trim to window size.
	g.actionWindow = append(g.actionWindow, state.ActionCalls)
	if len(g.actionWindow) > g.cfg.RepeatActionWindow {
		g.actionWindow = g.actionWindow[len(g.actionWindow)-g.cfg.RepeatActionWindow:]
	}

	g.outputWindow = append(g.outputWindow, state.LastOutput)
	if len(g.outputWindow) > g.cfg.OutputStagnationWindow {
		g.outputWindow = g.outputWindow[len(g.outputWindow)-g.cfg.OutputStagnationWindow:]
	}

	// 1. RepeatAction: last N rounds had identical action lists.
	if len(g.actionWindow) >= g.cfg.RepeatActionWindow {
		first := g.actionWindow[0]
		allEqual := true
		for i := 1; i < len(g.actionWindow); i++ {
			if !actionCallsEqual(first, g.actionWindow[i]) {
				allEqual = false
				break
			}
		}
		if allEqual {
			name := ""
			if len(first) > 0 {
				name = first[0].Name
			}
			return &LoopGuardVerdict{
				Action: VerdictBreak,
				Reason: fmt.Sprintf("repeated action calls: %s", name),
			}, nil
		}
	}

	// 2. OutputStagnation: all adjacent output pairs exceed similarity threshold.
	if len(g.outputWindow) >= g.cfg.OutputStagnationWindow {
		allSimilar := true
		minSim := 1.0
		for i := 1; i < len(g.outputWindow); i++ {
			sim := jaccardSimilarity(g.outputWindow[i-1], g.outputWindow[i])
			if sim <= g.cfg.OutputSimilarityThreshold {
				allSimilar = false
				break
			}
			if sim < minSim {
				minSim = sim
			}
		}
		if allSimilar {
			return &LoopGuardVerdict{
				Action: VerdictBreak,
				Reason: fmt.Sprintf("output stagnation: similarity=%g", minSim),
			}, nil
		}
	}

	// 3. ReasoningStagnation: detect thinking loops via head-prefix matching.
	// N == N+1 → silently record streak (no action); N+2 prefix matches → break.
	// Effective truncation at ~2.5 rounds of identical reasoning.
	if state.LastReasoning != "" && g.prevReasoning != "" {
		if state.LastReasoning == g.prevReasoning {
			// Exact match: increment streak silently.
			g.reasoningEqualStreak++
		} else if g.reasoningEqualStreak >= 1 {
			// Not exact, but streak active: check head-prefix match (N+2 case).
			prefixLen := g.cfg.ReasoningPrefixLen
			if prefixLen > len(g.prevReasoning) {
				prefixLen = len(g.prevReasoning)
			}
			prefix := g.prevReasoning[:prefixLen]
			if strings.HasPrefix(state.LastReasoning, prefix) {
				return &LoopGuardVerdict{
					Action: VerdictBreak,
					Reason: fmt.Sprintf("reasoning stagnation: %d rounds identical + prefix match", g.reasoningEqualStreak+1),
				}, nil
			}
			// Prefix didn't match either — reset.
			g.reasoningEqualStreak = 0
		} else {
			g.reasoningEqualStreak = 0
		}
		g.prevReasoning = state.LastReasoning

		// Exact-match streak reached 2+ (N, N+1, N+2 all identical): hard break.
		if g.reasoningEqualStreak >= 2 {
			return &LoopGuardVerdict{
				Action: VerdictBreak,
				Reason: fmt.Sprintf("reasoning stagnation: %d consecutive identical thinking outputs", g.reasoningEqualStreak+1),
			}, nil
		}
	} else if state.LastReasoning != "" {
		// First round with reasoning — just record.
		g.prevReasoning = state.LastReasoning
	}

	// 4. TimeBudget: total elapsed time exceeded.
	if !g.startedAt.IsZero() && time.Since(g.startedAt) > g.cfg.TimeBudget {
		return &LoopGuardVerdict{
			Action: VerdictBreak,
			Reason: "time budget exceeded",
		}, nil
	}

	// 5. TokenBudget: cumulative tokens exceeded.
	if g.totalTokens > g.cfg.TokenBudget {
		return &LoopGuardVerdict{
			Action: VerdictBreak,
			Reason: "token budget exceeded",
		}, nil
	}

	return &LoopGuardVerdict{Action: VerdictContinue}, nil
}

// Reset clears all internal state so the LoopGuard can be reused for a new run.
func (g *LoopGuard) Reset() {
	g.actionWindow = nil
	g.outputWindow = nil
	g.startedAt = time.Time{}
	g.totalTokens = 0
	g.prevReasoning = ""
	g.reasoningEqualStreak = 0
}

// jaccardSimilarity computes the Jaccard similarity between two strings' token
// sets. Tokens are maximal runs of characters that are neither whitespace nor
// basic punctuation. Returns 1.0 if both strings are empty, 0.0 if exactly one
// is empty.
func jaccardSimilarity(a, b string) float64 {
	setA := tokenize(a)
	setB := tokenize(b)
	if len(setA) == 0 && len(setB) == 0 {
		return 1.0
	}
	if len(setA) == 0 || len(setB) == 0 {
		return 0.0
	}
	intersection := 0
	for tok := range setA {
		if setB[tok] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

// tokenize splits s into a set of tokens delimited by whitespace or basic
// ASCII punctuation.
func tokenize(s string) map[string]bool {
	tokens := make(map[string]bool)
	var sb strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) || isPunctuation(r) {
			if sb.Len() > 0 {
				tokens[sb.String()] = true
				sb.Reset()
			}
		} else {
			sb.WriteRune(r)
		}
	}
	if sb.Len() > 0 {
		tokens[sb.String()] = true
	}
	return tokens
}

// isPunctuation reports whether r is a common ASCII punctuation character used
// as a token delimiter.
func isPunctuation(r rune) bool {
	switch r {
	case '.', ',', '!', '?', ';', ':', '"', '\'', '(', ')', '[', ']', '{', '}',
		'<', '>', '/', '\\', '|', '-', '_', '+', '=', '*', '&', '^', '%', '$',
		'#', '@', '`', '~':
		return true
	}
	return false
}

// actionCallsEqual reports whether two ActionCall slices have identical length,
// names, and params (DeepEqual on each Params map).
func actionCallsEqual(a, b []actionruntime.ActionCall) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
		if !reflect.DeepEqual(a[i].Params, b[i].Params) {
			return false
		}
	}
	return true
}
