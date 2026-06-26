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

// Package eval provides a framework for evaluating agent behavior against
// predefined test scenarios. It supports mock model providers, tool stubs,
// and integration with the golden session replay framework.
package eval

import (
	"time"

	"github.com/inferglow/model"
)

// Suite is a collection of evaluation test cases.
type Suite struct {
	// Name identifies the suite in reports.
	Name string

	// Cases is the list of evaluation scenarios to run.
	Cases []Case

	// Parallelism controls how many cases run concurrently.
	// 0 or 1 means serial execution (default and safest).
	Parallelism int
}

// Case is a single evaluation test scenario.
type Case struct {
	// Name identifies this case in reports.
	Name string

	// Input is the user message sent to Agent.Run.
	Input string

	// SystemPrompt overrides the default system prompt for this case.
	SystemPrompt string

	// Expect defines what to assert about the result.
	Expect Expectation

	// Tools are mock tools available to the agent during this case.
	Tools []ToolStub

	// MaxRounds overrides the default max rounds for Agent.Run.
	MaxRounds int

	// Timeout limits how long this case can run. Zero means no timeout.
	Timeout time.Duration
}

// Expectation defines assertions for a Case result.
type Expectation struct {
	// Contains requires the response to include each substring.
	Contains []string

	// NotContains requires the response to NOT include each substring.
	NotContains []string

	// ToolSequence is the expected ordered list of tool names called.
	// Empty means skip tool sequence check.
	ToolSequence []string

	// MaxRounds is the maximum number of PLAN→EXECUTE rounds allowed.
	// Zero means use the case-level or default value.
	MaxRounds int
}

// ToolStub is a mock tool definition for evaluation.
type ToolStub struct {
	// Name is the tool name as seen by the agent.
	Name string

	// Description is shown to the agent in the tool listing.
	Description string

	// Response is returned when the tool is called.
	Response any

	// Error is returned instead of Response when non-nil.
	Error error
}

// CaseResult captures the outcome of a single evaluation case.
type CaseResult struct {
	// CaseName is the name of the evaluated case.
	CaseName string

	// Pass indicates whether all assertions passed.
	Pass bool

	// Response is the agent's final response text.
	Response string

	// Latency is how long the case took to execute.
	Latency time.Duration

	// Usage is the token usage reported by the model provider.
	Usage model.UsageInfo

	// ToolCalls records the names of tools called in order.
	ToolCalls []string

	// Errors lists any assertion failures or execution errors.
	Errors []string
}

// Report summarizes the results of an entire evaluation suite.
type Report struct {
	// Suite is the name of the evaluated suite.
	Suite string

	// Total is the number of cases in the suite.
	Total int

	// Passed is the number of cases that passed all assertions.
	Passed int

	// Failed is the number of cases that had at least one assertion failure.
	Failed int

	// P50Latency is the median case execution latency.
	P50Latency time.Duration

	// P95Latency is the 95th percentile case execution latency.
	P95Latency time.Duration

	// Results is the per-case outcome for every case in the suite.
	Results []CaseResult
}
