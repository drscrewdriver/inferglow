// Package builtin provides built-in stage functions for common agent workflows.
//
// These stages use the flow.Context interface to call LLM (via GenerateModel)
// or run multi-turn agent loops (via RunAgent). They are designed to be registered
// in a stage.Registry and referenced by name in YAML flow definitions.
//
// Each stage function follows the stage.StageFunc signature:
//
//	func(ctx context.Context, in stage.Inputs, fctx flow.Context) (stage.Outputs, error)
//
// When fctx is nil, stages fall back to simple pass-through or return an error.
package builtin

import (
	"context"
	"fmt"
	"strings"

	"github.com/inferglow/flow"
	"github.com/inferglow/flow/stage"
)

// System prompts for each built-in stage.
const (
	TriageSystemPrompt = `You are an issue triage agent. Analyze the given issue and return a structured assessment.
Return JSON with these fields:
- "category": one of "bug", "feature", "refactor", "question"
- "priority": one of "critical", "high", "medium", "low"
- "summary": brief summary of the issue
- "labels": array of suggested labels`

	PlanSystemPrompt = `You are a planning agent. Given a triaged issue and its context, create an implementation plan.
Return JSON with these fields:
- "title": plan title
- "steps": array of implementation steps, each with "description" and "files" (array of file paths)
- "risks": array of potential risks
- "estimated_complexity": one of "low", "medium", "high"`

	CoderSystemPrompt = `You are a coding agent. Given an implementation plan (or specific task), write the code changes.
Return JSON with these fields:
- "files": array of objects with "path" and "content" fields
- "summary": brief description of changes made
- "tests": array of test file paths that should be updated`

	ReviewerSystemPrompt = `You are a code review agent. Review the proposed code changes and provide feedback.
Return JSON with these fields:
- "approved": boolean
- "comments": array of review comments, each with "file", "line", "severity" (info/warning/error), and "body"
- "summary": overall review summary
- "suggestions": array of improvement suggestions`
)

// RegisterAll registers all built-in stages into the given registry.
func RegisterAll(reg *stage.Registry) {
	reg.Register("triage", Triage)
	reg.Register("plan", Plan)
	reg.Register("coder", Coder)
	reg.Register("reviewer", Reviewer)
}

// Triage analyzes an issue and categorizes it.
//
// Inputs:
//   - "issue_title" (string): the issue title
//   - "issue_body" (string): the issue description
//   - "_system_prompt" (string, optional): overrides the default system prompt
//
// Outputs:
//   - "category", "priority", "summary", "labels"
func Triage(ctx context.Context, in stage.Inputs, fctx flow.Context) (stage.Outputs, error) {
	if fctx == nil {
		return stage.Outputs{"category": "unknown", "priority": "medium", "summary": "no Context", "labels": []string{}}, nil
	}

	issueTitle := getString(in, "issue_title")
	issueBody := getString(in, "issue_body")
	systemPrompt := getStringOr(in, "_system_prompt", TriageSystemPrompt)

	userMsg := fmt.Sprintf("Issue Title: %s\n\nIssue Body:\n%s", issueTitle, issueBody)
	resp, err := fctx.GenerateModel(ctx, systemPrompt, userMsg)
	if err != nil {
		return nil, fmt.Errorf("triage stage: %w", err)
	}

	return stage.Outputs{
		"category": extractJSONField(resp, "category", "unknown"),
		"priority": extractJSONField(resp, "priority", "medium"),
		"summary":  extractJSONField(resp, "summary", resp),
		"labels":   []string{},
		"raw":      resp,
	}, nil
}

// Plan creates an implementation plan based on the triaged issue.
//
// Inputs:
//   - "issue_title" (string): the issue title
//   - "category" (string): from triage
//   - "priority" (string): from triage
//   - "summary" (string): from triage
//   - "_system_prompt" (string, optional): overrides the default system prompt
//
// Outputs:
//   - "title", "steps", "risks", "estimated_complexity"
func Plan(ctx context.Context, in stage.Inputs, fctx flow.Context) (stage.Outputs, error) {
	if fctx == nil {
		return stage.Outputs{"title": "no-op plan", "steps": "[]", "risks": "[]", "estimated_complexity": "low"}, nil
	}

	systemPrompt := getStringOr(in, "_system_prompt", PlanSystemPrompt)
	userMsg := fmt.Sprintf(
		"Issue: %s\nCategory: %s\nPriority: %s\nSummary: %s\n\nCreate an implementation plan.",
		getString(in, "issue_title"),
		getString(in, "category"),
		getString(in, "priority"),
		getString(in, "summary"),
	)

	resp, err := fctx.GenerateModel(ctx, systemPrompt, userMsg)
	if err != nil {
		return nil, fmt.Errorf("plan stage: %w", err)
	}

	return stage.Outputs{
		"title":                extractJSONField(resp, "title", "Implementation Plan"),
		"steps":                extractJSONField(resp, "steps", "[]"),
		"risks":                extractJSONField(resp, "risks", "[]"),
		"estimated_complexity": extractJSONField(resp, "estimated_complexity", "medium"),
		"raw":                  resp,
	}, nil
}

// Coder writes code based on the plan.
//
// Inputs:
//   - "title" (string): plan title
//   - "steps" (string): plan steps (JSON string)
//   - "issue_title" (string): original issue
//   - "_system_prompt" (string, optional): overrides the default system prompt
//
// Outputs:
//   - "files", "summary", "tests"
func Coder(ctx context.Context, in stage.Inputs, fctx flow.Context) (stage.Outputs, error) {
	if fctx == nil {
		return stage.Outputs{"files": "[]", "summary": "no-op", "tests": "[]"}, nil
	}

	systemPrompt := getStringOr(in, "_system_prompt", CoderSystemPrompt)
	userMsg := fmt.Sprintf(
		"Plan: %s\nSteps: %s\nOriginal Issue: %s\n\nWrite the code changes.",
		getString(in, "title"),
		getString(in, "steps"),
		getString(in, "issue_title"),
	)

	resp, err := fctx.GenerateModel(ctx, systemPrompt, userMsg)
	if err != nil {
		return nil, fmt.Errorf("coder stage: %w", err)
	}

	return stage.Outputs{
		"files":   extractJSONField(resp, "files", "[]"),
		"summary": extractJSONField(resp, "summary", resp),
		"tests":   extractJSONField(resp, "tests", "[]"),
		"raw":     resp,
	}, nil
}

// Reviewer reviews the code changes.
//
// Inputs:
//   - "files" (string): code changes (JSON string)
//   - "summary" (string): coder summary
//   - "issue_title" (string): original issue
//   - "_system_prompt" (string, optional): overrides the default system prompt
//
// Outputs:
//   - "approved", "comments", "summary", "suggestions"
func Reviewer(ctx context.Context, in stage.Inputs, fctx flow.Context) (stage.Outputs, error) {
	if fctx == nil {
		return stage.Outputs{"approved": "true", "comments": "[]", "summary": "no-op review", "suggestions": "[]"}, nil
	}

	systemPrompt := getStringOr(in, "_system_prompt", ReviewerSystemPrompt)
	userMsg := fmt.Sprintf(
		"Code Changes:\n%s\n\nCoder Summary: %s\n\nOriginal Issue: %s\n\nReview the changes.",
		getString(in, "files"),
		getString(in, "summary"),
		getString(in, "issue_title"),
	)

	resp, err := fctx.GenerateModel(ctx, systemPrompt, userMsg)
	if err != nil {
		return nil, fmt.Errorf("reviewer stage: %w", err)
	}

	return stage.Outputs{
		"approved":    extractJSONField(resp, "approved", "true"),
		"comments":    extractJSONField(resp, "comments", "[]"),
		"summary":     extractJSONField(resp, "summary", resp),
		"suggestions": extractJSONField(resp, "suggestions", "[]"),
		"raw":         resp,
	}, nil
}

// --- helpers ---

func getString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func getStringOr(m map[string]any, key, fallback string) string {
	if v := getString(m, key); v != "" {
		return v
	}
	return fallback
}

// extractJSONField does a best-effort extraction of a JSON field value from
// a string that may contain JSON. It uses simple string search rather than
// full JSON parsing to be tolerant of LLM output formatting variations.
func extractJSONField(s, field, fallback string) string {
	// Try to find "field": "value" or "field": value patterns
	patterns := []string{
		fmt.Sprintf(`"%s"`, field),
	}
	for _, pattern := range patterns {
		idx := strings.Index(s, pattern)
		if idx < 0 {
			continue
		}
		// Find the colon after the key
		rest := s[idx+len(pattern):]
		colonIdx := strings.Index(rest, ":")
		if colonIdx < 0 {
			continue
		}
		value := strings.TrimSpace(rest[colonIdx+1:])
		// Extract the value (string or other)
		if strings.HasPrefix(value, `"`) {
			// Quoted string
			end := strings.Index(value[1:], `"`)
			if end >= 0 {
				return value[1 : end+1]
			}
		}
		// Unquoted value (number, bool, array, object)
		end := strings.IndexAny(value, ",}\n")
		if end >= 0 {
			return strings.TrimSpace(value[:end])
		}
		return strings.TrimSpace(value)
	}
	return fallback
}
