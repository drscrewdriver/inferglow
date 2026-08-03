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
// AVERROR OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package compress

import (
	"context"
	"strings"
	"testing"
)

func TestMaskHeaderRegex_Valid(t *testing.T) {
	validCases := []string{
		"[掩码 step_1|原5t|tool|params]",
		"[掩码 step_42|原128t|read_file|path=main.go]",
		"[掩码 step_0|原0t|tool|p]",
		"[掩码 step_999|原1000t|write_file|path=test.go,content=hello]",
	}

	for _, c := range validCases {
		if !maskHeaderRegex.MatchString(c) {
			t.Errorf("expected valid mask header %q to match regex", c)
		}
	}
}

func TestMaskHeaderRegex_Invalid(t *testing.T) {
	invalidCases := []string{
		"",
		"普通文本 without mask",
		"[掩码 step_|原5t|tool|params]",       // missing step number
		"[掩码 step_1|原t|tool|params]",       // missing token count
		"[掩码 step_1||tool|params]",         // empty token count field
		"掩码 step_1|原5t|tool|params]",       // missing opening bracket
		"[掩码 step_1|原5t|tool|params",       // missing closing bracket
		"[Mask step_1|原5t|tool|params]",    // wrong prefix
		"先有文本 [掩码 step_1|原5t|tool|params]", // text before mask
	}

	for _, c := range invalidCases {
		if maskHeaderRegex.MatchString(c) {
			t.Errorf("expected invalid mask header %q to NOT match regex", c)
		}
	}
}

// mockCompressClient implements CompressModelClient for testing.
type mockCompressClient struct {
	compressFn func(ctx context.Context, level int, prompt string) (string, error)
	available  bool
}

func (m *mockCompressClient) Compress(ctx context.Context, level int, prompt string) (string, error) {
	if m.compressFn != nil {
		return m.compressFn(ctx, level, prompt)
	}
	return "", nil
}

func (m *mockCompressClient) Available() bool {
	return m.available
}

func TestCompressModelChain_Interface(t *testing.T) {
	// Test 1: small model succeeds
	small := &mockCompressClient{
		available: true,
		compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
			return "small result", nil
		},
	}
	main := &mockCompressClient{
		available: true,
		compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
			return "main result", nil
		},
	}
	chain := NewCompressModelChain(small, main, 0, 0)
	ctx := context.Background()
	result, err := chain.Compress(ctx, 1, "test prompt with enough length to pass validation")
	if err != nil {
		t.Fatalf("Compress returned error: %v", err)
	}
	if result != "small result" {
		t.Errorf("expected small model result, got %q", result)
	}

	// Test 2: small model unavailable, main model succeeds
	small2 := &mockCompressClient{available: false}
	main2 := &mockCompressClient{
		available: true,
		compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
			return "main result", nil
		},
	}
	chain2 := NewCompressModelChain(small2, main2, 0, 0)
	result2, err2 := chain2.Compress(ctx, 1, "test prompt")
	if err2 != nil {
		t.Fatalf("Compress returned error: %v", err2)
	}
	if result2 != "main result" {
		t.Errorf("expected main model result, got %q", result2)
	}

	// Test 3: both models unavailable, falls back to mechanical
	small3 := &mockCompressClient{available: false}
	main3 := &mockCompressClient{available: false}
	chain3 := NewCompressModelChain(small3, main3, 0, 0)
	result3, err3 := chain3.Compress(ctx, 1, "test prompt with some content")
	if err3 != nil {
		t.Fatalf("Compress returned error: %v", err3)
	}
	if !strings.Contains(result3, "test prompt") {
		t.Errorf("expected mechanical fallback output, got %q", result3)
	}

	// Test 4: validate L2/L3 requires mask header, falls back if invalid
	small4 := &mockCompressClient{
		available: true,
		compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
			return "invalid output without mask header", nil
		},
	}
	main4 := &mockCompressClient{
		available: true,
		compressFn: func(ctx context.Context, level int, prompt string) (string, error) {
			return "also invalid", nil
		},
	}
	chain4 := NewCompressModelChain(small4, main4, 0, 0)
	result4, err4 := chain4.Compress(ctx, 2, "some content")
	if err4 != nil {
		t.Fatalf("Compress returned error: %v", err4)
	}
	// Should fall back to mechanical L2 which doesn't produce mask header from raw content
	// but still returns something
	if result4 == "" {
		t.Errorf("expected non-empty result from mechanical fallback")
	}
}
