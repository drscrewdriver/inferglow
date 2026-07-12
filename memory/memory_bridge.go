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

package memory

import (
	"fmt"
	"strings"
	"time"
)

// MemoryBridge provides automated memory extraction from execution snapshots.
// It analyzes flow execution results and extracts key information into
// long-term memories, closing the loop between execution and learning.
//
// Usage:
//
//	bridge := NewMemoryBridge(store)
//	bridge.ExtractFromSnapshot(snapshot)
//
// The bridge is designed to be called after flow completion to automatically
// capture important patterns, user preferences, and project insights.
type MemoryBridge struct {
	store  Store
	config MemoryBridgeConfig
}

// MemoryBridgeConfig controls the behavior of memory extraction.
type MemoryBridgeConfig struct {
	// Enabled controls whether extraction is active.
	Enabled bool
	// MinStepsForExtraction is the minimum number of steps executed
	// before attempting extraction (avoids noise from trivial flows).
	MinStepsForExtraction int
	// ExtractUserPreferences enables extraction of user preference patterns.
	ExtractUserPreferences bool
	// ExtractProjectInsights enables extraction of project-level insights.
	ExtractProjectInsights bool
	// ExtractFeedback enables extraction of corrective feedback.
	ExtractFeedback bool
}

// DefaultMemoryBridgeConfig returns a sensible default configuration.
func DefaultMemoryBridgeConfig() MemoryBridgeConfig {
	return MemoryBridgeConfig{
		Enabled:                true,
		MinStepsForExtraction:  3,
		ExtractUserPreferences: true,
		ExtractProjectInsights: true,
		ExtractFeedback:        true,
	}
}

// NewMemoryBridge creates a MemoryBridge with the given store and default config.
func NewMemoryBridge(store Store) *MemoryBridge {
	return &MemoryBridge{
		store:  store,
		config: DefaultMemoryBridgeConfig(),
	}
}

// NewMemoryBridgeWithConfig creates a MemoryBridge with custom config.
func NewMemoryBridgeWithConfig(store Store, config MemoryBridgeConfig) *MemoryBridge {
	return &MemoryBridge{
		store:  store,
		config: config,
	}
}

// ExtractionResult summarizes what was extracted from a snapshot.
type ExtractionResult struct {
	ExtractedCount int
	Memories       []Memory
	Errors         []error
}

// ExtractFromSnapshot analyzes an ExecutionSnapshot and extracts memories.
// This is the main entry point for automated memory extraction.
//
// The snapshot parameter is expected to be a map with the following keys:
//   - "execution_id": string
//   - "flow_name": string
//   - "status": string ("completed", "failed", "paused")
//   - "step_log": map of step results
//   - "result": any (the final result)
//   - "interventions": []InterventionState (user corrections)
func (b *MemoryBridge) ExtractFromSnapshot(snapshot map[string]any) (*ExtractionResult, error) {
	if !b.config.Enabled {
		return &ExtractionResult{}, nil
	}

	result := &ExtractionResult{}

	// Check minimum step threshold
	stepLog, _ := snapshot["step_log"].(map[string]any)
	if len(stepLog) < b.config.MinStepsForExtraction {
		return result, nil // Skip trivial executions
	}

	// Extract user preferences from interventions
	if b.config.ExtractFeedback {
		if interventions, ok := snapshot["interventions"].([]any); ok && len(interventions) > 0 {
			memories, err := b.extractFromInterventions(interventions, snapshot)
			if err != nil {
				result.Errors = append(result.Errors, err)
			} else {
				result.Memories = append(result.Memories, memories...)
				result.ExtractedCount += len(memories)
			}
		}
	}

	// Extract project insights from successful executions
	if b.config.ExtractProjectInsights {
		status, _ := snapshot["status"].(string)
		if status == "completed" {
			memories, err := b.extractProjectInsights(snapshot)
			if err != nil {
				result.Errors = append(result.Errors, err)
			} else {
				result.Memories = append(result.Memories, memories...)
				result.ExtractedCount += len(memories)
			}
		}
	}

	// Persist extracted memories
	for _, m := range result.Memories {
		if _, err := b.store.Save(m); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("save memory %q: %w", m.Name, err))
		}
	}

	return result, nil
}

// extractFromInterventions extracts feedback memories from user interventions.
// Interventions represent moments where the user corrected or guided the agent.
func (b *MemoryBridge) extractFromInterventions(interventions []any, snapshot map[string]any) ([]Memory, error) {
	var memories []Memory
	flowName, _ := snapshot["flow_name"].(string)

	for i, iv := range interventions {
		intervention, ok := iv.(map[string]any)
		if !ok {
			continue
		}
		userInput, _ := intervention["user_input"].(string)
		if userInput == "" {
			continue
		}
		// Create a feedback memory from the intervention
		m := Memory{
			Type:        TypeFeedback,
			Name:        fmt.Sprintf("feedback-%s-%d", slugify(flowName), i),
			Title:       fmt.Sprintf("User feedback on %s", flowName),
			Description: truncate(userInput, 200),
			Body:        userInput,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		memories = append(memories, m)
	}
	return memories, nil
}

// extractProjectInsights extracts project-level memories from successful executions.
func (b *MemoryBridge) extractProjectInsights(snapshot map[string]any) ([]Memory, error) {
	var memories []Memory
	flowName, _ := snapshot["flow_name"].(string)
	result, _ := snapshot["result"].(map[string]any)

	// Extract key patterns from the result
	if patterns := extractPatterns(result); len(patterns) > 0 {
		m := Memory{
			Type:        TypeProject,
			Name:        fmt.Sprintf("insight-%s", slugify(flowName)),
			Title:       fmt.Sprintf("Project insight from %s", flowName),
			Description: strings.Join(patterns, "; "),
			Body:        formatPatterns(patterns),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		memories = append(memories, m)
	}

	return memories, nil
}

// extractPatterns identifies key patterns in execution results.
func extractPatterns(result map[string]any) []string {
	if result == nil {
		return nil
	}
	var patterns []string

	// Look for common insight indicators
	if tools, ok := result["tools_used"].([]any); ok && len(tools) > 0 {
		patterns = append(patterns, fmt.Sprintf("Tools used: %v", tools))
	}
	if duration, ok := result["duration_seconds"].(float64); ok && duration > 0 {
		patterns = append(patterns, fmt.Sprintf("Duration: %.1fs", duration))
	}
	if steps, ok := result["steps_executed"].(int); ok && steps > 0 {
		patterns = append(patterns, fmt.Sprintf("Steps: %d", steps))
	}

	return patterns
}

// formatPatterns joins patterns into a readable content string.
func formatPatterns(patterns []string) string {
	var b strings.Builder
	b.WriteString("## Execution Patterns\n\n")
	for _, p := range patterns {
		b.WriteString("- ")
		b.WriteString(p)
		b.WriteString("\n")
	}
	return b.String()
}

// truncate shortens a string to maxLen characters.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// SetEnabled enables or disables memory extraction.
func (b *MemoryBridge) SetEnabled(enabled bool) {
	b.config.Enabled = enabled
}

// SetMinSteps sets the minimum step threshold for extraction.
func (b *MemoryBridge) SetMinSteps(n int) {
	b.config.MinStepsForExtraction = n
}
