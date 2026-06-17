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

package otel

import (
	"context"

	gootel "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics holds all InferGlow OTel metric instruments.
type Metrics struct {
	// Token usage counters
	InputTokenCounter  metric.Int64Counter
	OutputTokenCounter metric.Int64Counter

	// Operation metrics
	LLMCallCounter     metric.Int64Counter
	LLMCallDuration    metric.Float64Histogram
	LLMTimeToFirst     metric.Float64Histogram
	ToolCallCounter    metric.Int64Counter
	ToolCallDuration   metric.Float64Histogram
	AgentRunCounter    metric.Int64Counter
	AgentRunDuration   metric.Float64Histogram

	// Error tracking
	LLMErrorCounter    metric.Int64Counter
	ToolErrorCounter   metric.Int64Counter
}

// NewMetrics creates and initializes all OTel metric instruments.
// Returns nil-safe instruments (noop if meter is unavailable).
func NewMetrics() *Metrics {
	meter := gootel.Meter("inferglow")

	m := &Metrics{}

	// Token usage
	m.InputTokenCounter, _ = meter.Int64Counter(
		"gen_ai.usage.input_tokens",
		metric.WithDescription("Total input tokens consumed"),
		metric.WithUnit("{token}"),
	)
	m.OutputTokenCounter, _ = meter.Int64Counter(
		"gen_ai.usage.output_tokens",
		metric.WithDescription("Total output tokens produced"),
		metric.WithUnit("{token}"),
	)

	// LLM operations
	m.LLMCallCounter, _ = meter.Int64Counter(
		"gen_ai.client.operation.count",
		metric.WithDescription("Number of LLM inference calls"),
		metric.WithUnit("{call}"),
	)
	m.LLMCallDuration, _ = meter.Float64Histogram(
		"gen_ai.client.operation.duration",
		metric.WithDescription("LLM inference call duration"),
		metric.WithUnit("s"),
	)
	m.LLMTimeToFirst, _ = meter.Float64Histogram(
		"gen_ai.client.time_to_first_token",
		metric.WithDescription("Time from request to first response chunk"),
		metric.WithUnit("s"),
	)

	// Tool operations
	m.ToolCallCounter, _ = meter.Int64Counter(
		"gen_ai.tool.call.count",
		metric.WithDescription("Number of tool executions"),
		metric.WithUnit("{call}"),
	)
	m.ToolCallDuration, _ = meter.Float64Histogram(
		"gen_ai.tool.call.duration",
		metric.WithDescription("Tool execution duration"),
		metric.WithUnit("s"),
	)

	// Agent operations
	m.AgentRunCounter, _ = meter.Int64Counter(
		"gen_ai.agent.run.count",
		metric.WithDescription("Number of agent runs"),
		metric.WithUnit("{run}"),
	)
	m.AgentRunDuration, _ = meter.Float64Histogram(
		"gen_ai.agent.run.duration",
		metric.WithDescription("Agent run duration"),
		metric.WithUnit("s"),
	)

	// Error tracking
	m.LLMErrorCounter, _ = meter.Int64Counter(
		"gen_ai.client.error.count",
		metric.WithDescription("Number of LLM call errors"),
		metric.WithUnit("{error}"),
	)
	m.ToolErrorCounter, _ = meter.Int64Counter(
		"gen_ai.tool.error.count",
		metric.WithDescription("Number of tool execution errors"),
		metric.WithUnit("{error}"),
	)

	return m
}

// RecordTokenUsage records input and output token counts.
func (m *Metrics) RecordTokenUsage(ctx context.Context, model, provider string, inputTokens, outputTokens int64) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String(AttrModelName, model),
		attribute.String(AttrModelProvider, provider),
	)
	if m.InputTokenCounter != nil {
		m.InputTokenCounter.Add(ctx, inputTokens, attrs)
	}
	if m.OutputTokenCounter != nil {
		m.OutputTokenCounter.Add(ctx, outputTokens, attrs)
	}
}

// RecordLLMCall records an LLM call with duration.
func (m *Metrics) RecordLLMCall(ctx context.Context, model, provider string, durationSec float64, err error) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String(AttrModelName, model),
		attribute.String(AttrModelProvider, provider),
	)
	if m.LLMCallCounter != nil {
		m.LLMCallCounter.Add(ctx, 1, attrs)
	}
	if m.LLMCallDuration != nil {
		m.LLMCallDuration.Record(ctx, durationSec, attrs)
	}
	if err != nil && m.LLMErrorCounter != nil {
		m.LLMErrorCounter.Add(ctx, 1, attrs)
	}
}

// RecordToolCall records a tool execution with duration.
func (m *Metrics) RecordToolCall(ctx context.Context, toolName string, durationSec float64, err error) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String(AttrToolName, toolName),
	)
	if m.ToolCallCounter != nil {
		m.ToolCallCounter.Add(ctx, 1, attrs)
	}
	if m.ToolCallDuration != nil {
		m.ToolCallDuration.Record(ctx, durationSec, attrs)
	}
	if err != nil && m.ToolErrorCounter != nil {
		m.ToolErrorCounter.Add(ctx, 1, attrs)
	}
}

// RecordAgentRun records an agent run with duration.
func (m *Metrics) RecordAgentRun(ctx context.Context, agentName string, durationSec float64) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(
		attribute.String(GenAIAgentName, agentName),
	)
	if m.AgentRunCounter != nil {
		m.AgentRunCounter.Add(ctx, 1, attrs)
	}
	if m.AgentRunDuration != nil {
		m.AgentRunDuration.Record(ctx, durationSec, attrs)
	}
}

// DefaultMetrics returns a global Metrics instance.
var DefaultMetrics = NewMetrics()
