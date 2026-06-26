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

package eval

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
	"github.com/inferglow/orchestrator/agent"
	"github.com/inferglow/session"
)

// Runner executes evaluation suites and collects results.
type Runner struct {
	// DefaultMaxRounds is the default max PLAN→EXECUTE rounds when a Case
	// does not override it. Zero means 10.
	DefaultMaxRounds int
}

// Run executes all cases in the suite and returns a report.
func (r *Runner) Run(ctx context.Context, suite Suite) (*Report, error) {
	if len(suite.Cases) == 0 {
		return &Report{Suite: suite.Name}, nil
	}

	maxRounds := r.DefaultMaxRounds
	if maxRounds <= 0 {
		maxRounds = 10
	}

	parallelism := suite.Parallelism
	if parallelism <= 1 {
		parallelism = 1
	}

	results := make([]CaseResult, len(suite.Cases))
	sem := make(chan struct{}, parallelism)
	var wg sync.WaitGroup

	for i, c := range suite.Cases {
		wg.Add(1)
		go func(idx int, tc Case) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results[idx] = r.runCase(ctx, tc, maxRounds)
		}(i, c)
	}
	wg.Wait()

	return buildReport(suite.Name, results), nil
}

// runCase executes a single evaluation case and returns the result.
func (r *Runner) runCase(ctx context.Context, tc Case, defaultMaxRounds int) CaseResult {
	res := CaseResult{CaseName: tc.Name}
	start := time.Now()

	// Build per-case context with optional timeout.
	caseCtx := ctx
	if tc.Timeout > 0 {
		var cancel context.CancelFunc
		caseCtx, cancel = context.WithTimeout(ctx, tc.Timeout)
		defer cancel()
	}

	// Create a ScriptedProvider from the case's expected responses.
	// For simple cases, we synthesize a response from the Expect.Contains
	// fields. For more complex scenarios, the caller should pre-configure
	// the provider via the Tools and expected tool-call sequence.
	provider := buildProvider(tc)

	// Create a fresh session and action extension for isolation.
	sess := session.NewSession("eval-"+tc.Name, 100000)
	actExt := agent.NewActionExtension()

	// Register tool stubs as mock actions.
	var toolCalls []string
	var toolCallsMu sync.Mutex
	for _, ts := range tc.Tools {
		ts := ts // capture
		act, err := action.New(ts.Name, ts.Description, func(_ context.Context, _ map[string]any) (any, error) {
			toolCallsMu.Lock()
			toolCalls = append(toolCalls, ts.Name)
			toolCallsMu.Unlock()
			if ts.Error != nil {
				return nil, ts.Error
			}
			return ts.Response, nil
		})
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("register tool %q: %v", ts.Name, err))
			res.Latency = time.Since(start)
			return res
		}
		if err := actExt.Register(act); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("register tool %q: %v", ts.Name, err))
			res.Latency = time.Since(start)
			return res
		}
	}

	// Build run options.
	maxRounds := defaultMaxRounds
	if tc.MaxRounds > 0 {
		maxRounds = tc.MaxRounds
	}
	if tc.Expect.MaxRounds > 0 && tc.Expect.MaxRounds < maxRounds {
		maxRounds = tc.Expect.MaxRounds
	}

	opts := []agent.RunOption{
		agent.WithMaxRounds(maxRounds),
	}
	if tc.SystemPrompt != "" {
		opts = append(opts, agent.WithSystemPrompt(tc.SystemPrompt))
	}

	// Create and run the agent.
	a := agent.New(sess, actExt, provider)
	response, err := a.Run(caseCtx, tc.Input, opts...)
	res.Latency = time.Since(start)
	res.Response = response
	res.ToolCalls = toolCalls

	if err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("agent.Run: %v", err))
		res.Pass = false
		return res
	}

	// Evaluate assertions.
	res.Errors = evaluateAssertions(tc.Expect, response, toolCalls)
	res.Pass = len(res.Errors) == 0
	return res
}

// buildProvider creates a ScriptedProvider from the case definition.
// It synthesizes responses based on the expected tool sequence and contains.
func buildProvider(tc Case) *ScriptedProvider {
	responses := buildScriptedResponses(tc)
	return &ScriptedProvider{Responses: responses}
}

// buildScriptedResponses synthesizes the response sequence for a case.
// Strategy:
//   - If Tools are defined and ToolSequence is expected, generate tool-call
//     responses followed by a final response.
//   - Otherwise, generate a single direct response from Expect.Contains.
func buildScriptedResponses(tc Case) []ScriptedResponse {
	// If no tools, produce a simple direct response.
	if len(tc.Tools) == 0 && len(tc.Expect.ToolSequence) == 0 {
		content := `{"next_action":"response","final_response":"`
		if len(tc.Expect.Contains) > 0 {
			content += tc.Expect.Contains[0]
		} else {
			content += "ok"
		}
		content += `"}`
		return []ScriptedResponse{{Content: content}}
	}

	// If ToolSequence is specified, generate tool-call responses for each
	// step, then a final response.
	if len(tc.Expect.ToolSequence) > 0 {
		var resps []ScriptedResponse
		for _, toolName := range tc.Expect.ToolSequence {
			resps = append(resps, ScriptedResponse{
				Content: "",
				ToolCalls: []model.ToolCall{
					{
						ID:        "call_" + toolName,
						Name:      toolName,
						Arguments: map[string]any{},
					},
				},
			})
		}
		// Final response.
		finalContent := "ok"
		if len(tc.Expect.Contains) > 0 {
			finalContent = tc.Expect.Contains[0]
		}
		resps = append(resps, ScriptedResponse{
			Content: `{"next_action":"response","final_response":"` + finalContent + `"}`,
		})
		return resps
	}

	// Tools defined but no ToolSequence: single response (agent may or
	// may not call tools depending on the input).
	content := `{"next_action":"response","final_response":"`
	if len(tc.Expect.Contains) > 0 {
		content += tc.Expect.Contains[0]
	} else {
		content += "ok"
	}
	content += `"}`
	return []ScriptedResponse{{Content: content}}
}

// evaluateAssertions checks the response and tool calls against expectations.
func evaluateAssertions(expect Expectation, response string, toolCalls []string) []string {
	var errs []string

	for _, substr := range expect.Contains {
		if !strings.Contains(response, substr) {
			errs = append(errs, fmt.Sprintf("response missing %q", substr))
		}
	}
	for _, substr := range expect.NotContains {
		if strings.Contains(response, substr) {
			errs = append(errs, fmt.Sprintf("response should not contain %q", substr))
		}
	}
	if len(expect.ToolSequence) > 0 {
		if !matchToolSequence(expect.ToolSequence, toolCalls) {
			errs = append(errs, fmt.Sprintf("tool sequence mismatch: expected %v, got %v",
				expect.ToolSequence, toolCalls))
		}
	}

	return errs
}

// matchToolSequence checks if the expected tool sequence is a subsequence of
// the actual tool calls.
func matchToolSequence(expected, actual []string) bool {
	if len(expected) == 0 {
		return true
	}
	if len(expected) > len(actual) {
		return false
	}
	ei := 0
	for ai := 0; ai < len(actual) && ei < len(expected); ai++ {
		if actual[ai] == expected[ei] {
			ei++
		}
	}
	return ei == len(expected)
}

// buildReport constructs a Report from the collected results.
func buildReport(suiteName string, results []CaseResult) *Report {
	r := &Report{
		Suite:   suiteName,
		Total:   len(results),
		Results: results,
	}

	var latencies []time.Duration
	for _, res := range results {
		if res.Pass {
			r.Passed++
		} else {
			r.Failed++
		}
		latencies = append(latencies, res.Latency)
	}

	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		r.P50Latency = latencies[len(latencies)*50/100]
		p95Idx := len(latencies)*95/100 - 1
		if p95Idx < 0 {
			p95Idx = 0
		}
		if p95Idx >= len(latencies) {
			p95Idx = len(latencies) - 1
		}
		r.P95Latency = latencies[p95Idx]
	}

	return r
}
