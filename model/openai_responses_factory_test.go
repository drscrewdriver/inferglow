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

package model

import (
	"errors"
	"strings"
	"testing"
)

// TestNewOpenAIResponsesProviderFromConfig verifies the factory:
//   - DEFAULT_SETTINGS provides BaseURL + Model defaults
//   - <prefix>.api_key / .full_url / .model override defaults
//   - missing api_key returns ErrMissingRequiredConfig
func TestNewOpenAIResponsesProviderFromConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"openai_responses": map[string]any{
				"api_key": "sk-test",
			},
		}}
		p, err := NewOpenAIResponsesProviderFromConfig(cp, "openai_responses")
		if err != nil {
			t.Fatalf("NewOpenAIResponsesProviderFromConfig failed: %v", err)
		}
		if p.BaseURL != "https://api.openai.com/v1" {
			t.Errorf("BaseURL = %q, want https://api.openai.com/v1 (DEFAULT_SETTINGS)", p.BaseURL)
		}
		if p.Model != "gpt-4o" {
			t.Errorf("Model = %q, want gpt-4o (DEFAULT_SETTINGS)", p.Model)
		}
		if p.APIKey != "sk-test" {
			t.Errorf("APIKey = %q, want sk-test", p.APIKey)
		}
		if p.Name() != "openai-responses" {
			t.Errorf("Name() = %q, want openai-responses", p.Name())
		}
	})

	t.Run("override_full_url_and_model", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{
			"openai_responses": map[string]any{
				"api_key":  "sk-test",
				"full_url": "https://gateway.example.com/responses",
				"model":    "o1-mini",
			},
		}}
		p, err := NewOpenAIResponsesProviderFromConfig(cp, "openai_responses")
		if err != nil {
			t.Fatalf("NewOpenAIResponsesProviderFromConfig failed: %v", err)
		}
		if p.FullURL != "https://gateway.example.com/responses" {
			t.Errorf("FullURL = %q, want https://gateway.example.com/responses", p.FullURL)
		}
		if p.Model != "o1-mini" {
			t.Errorf("Model = %q, want o1-mini", p.Model)
		}
	})

	t.Run("missing_api_key_returns_error", func(t *testing.T) {
		cp := &StaticConfigProvider{Values: map[string]any{}}
		_, err := NewOpenAIResponsesProviderFromConfig(cp, "openai_responses")
		if err == nil {
			t.Fatal("expected error for missing api_key")
		}
		if !errors.Is(err, ErrMissingRequiredConfig) {
			t.Errorf("expected ErrMissingRequiredConfig, got %v", err)
		}
		if !strings.Contains(err.Error(), "openai_responses") {
			t.Errorf("error should contain prefix 'openai_responses', got %v", err)
		}
	})
}
