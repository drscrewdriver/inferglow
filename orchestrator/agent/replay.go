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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/inferglow/session"
)

// ReplayConfig configures a deterministic replay test.
type ReplayConfig struct {
	// SessionFile is the path to the golden session JSON file.
	SessionFile string

	// PromptVersion is the expected prompt template version.
	// If non-empty, the golden session's PromptVersion must match.
	PromptVersion string

	// ToolInterceptor intercepts tool calls during replay.
	// Given the tool name and arguments, it returns the recorded response.
	// If nil, tool calls are not intercepted (real execution occurs).
	ToolInterceptor func(toolName string, args map[string]any) (any, error)
}

// ReplayResult captures the outcome of a replay test.
type ReplayResult struct {
	// Match indicates whether the replay matched all expectations.
	Match bool

	// GoldenPromptVersion is the prompt version from the golden session.
	GoldenPromptVersion string

	// GoldenMessages is the list of messages from the golden session.
	GoldenMessages []session.ChatMessage

	// UserMessages are the user messages extracted from the golden session.
	UserMessages []string

	// Diffs describes any differences found.
	Diffs []string
}

// ToolCallRecord records a single tool call during replay.
type ToolCallRecord struct {
	ToolName string
	Args     map[string]any
	Response any
	Error    error
}

// Replay loads a golden session, validates the prompt version, and prepares
// the replay context. The caller is responsible for executing Agent.Run with
// the extracted user messages and comparing results.
//
// This is the primary entry point for deterministic replay testing.
// It validates that:
//   - The golden session file can be loaded
//   - The prompt version matches (if configured)
//   - User messages can be extracted in order
//
// The actual agent execution is left to the caller because Agent.Run
// requires a fully configured Agent with model, tools, etc.
func Replay(ctx context.Context, cfg ReplayConfig) (*ReplayResult, error) {
	result := &ReplayResult{}

	// Load the golden session.
	data, err := os.ReadFile(cfg.SessionFile)
	if err != nil {
		return nil, fmt.Errorf("replay: read session file: %w", err)
	}

	var sessionData session.SessionData
	if err := json.Unmarshal(data, &sessionData); err != nil {
		return nil, fmt.Errorf("replay: parse session JSON: %w", err)
	}

	result.GoldenPromptVersion = sessionData.PromptVersion
	result.GoldenMessages = sessionData.FullContext

	// Validate prompt version.
	if cfg.PromptVersion != "" && sessionData.PromptVersion != cfg.PromptVersion {
		result.Diffs = append(result.Diffs, fmt.Sprintf(
			"prompt version mismatch: golden=%q, expected=%q",
			sessionData.PromptVersion, cfg.PromptVersion))
	}

	// Extract user messages in order.
	for _, msg := range sessionData.FullContext {
		if msg.Role == "user" {
			text := extractText(msg.Content)
			result.UserMessages = append(result.UserMessages, text)
		}
	}

	// Determine match status.
	result.Match = len(result.Diffs) == 0

	return result, nil
}

// ReplayCompare compares an actual response against the golden session's
// assistant responses. Returns true if the actual response matches any
// golden assistant response (using exact string comparison).
func ReplayCompare(goldenMessages []session.ChatMessage, actual string) bool {
	for _, msg := range goldenMessages {
		if msg.Role == "assistant" {
			goldenText := extractText(msg.Content)
			if goldenText == actual {
				return true
			}
		}
	}
	return false
}

// ReplayToolCallSequence compares the actual tool call sequence against
// the expected sequence. Both are slices of tool names in execution order.
func ReplayToolCallSequence(expected, actual []string) (bool, []string) {
	var diffs []string
	if len(expected) != len(actual) {
		diffs = append(diffs, fmt.Sprintf(
			"tool call count: expected %d, got %d", len(expected), len(actual)))
	}
	minLen := len(expected)
	if len(actual) < minLen {
		minLen = len(actual)
	}
	for i := 0; i < minLen; i++ {
		if expected[i] != actual[i] {
			diffs = append(diffs, fmt.Sprintf(
				"tool call[%d]: expected %q, got %q", i, expected[i], actual[i]))
		}
	}
	return len(diffs) == 0, diffs
}

// extractText converts a ChatMessage's Content (which can be string or []ContentBlock)
// to a plain string.
func extractText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, block := range v {
			if m, ok := block.(map[string]any); ok {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	default:
		return fmt.Sprintf("%v", content)
	}
}
