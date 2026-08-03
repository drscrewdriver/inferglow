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

package actions

import (
	"context"
	"errors"
	"testing"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
)

func TestImageGenerateActionSpec(t *testing.T) {
	a := NewImageGenerateAction(nil)
	if a.Name != ImageGenerateActionID {
		t.Errorf("Name = %q, want %q", a.Name, ImageGenerateActionID)
	}
	if a.Executor == nil {
		t.Error("Executor should not be nil")
	}
}

func TestImageGenerateActionRegistry(t *testing.T) {
	r := action.NewRegistry()
	if err := r.Register(NewImageGenerateAction(nil)); err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if !r.Has(ImageGenerateActionID) {
		t.Errorf("registry missing %q", ImageGenerateActionID)
	}
}

func TestImageGenerateExecutorSuccess(t *testing.T) {
	a := NewImageGenerateAction(nil)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"prompt": "a cute cat",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if res.Status != "success" {
		t.Errorf("Status = %q, want success", res.Status)
	}
	result, ok := res.Result.(map[string]any)
	if !ok {
		t.Fatalf("Result not map[string]any: %T", res.Result)
	}
	if result["prompt"] != "a cute cat" {
		t.Errorf("prompt = %v", result["prompt"])
	}
	if result["mime_type"] != "image/png" {
		t.Errorf("mime_type = %v", result["mime_type"])
	}
	if len(res.ContentBlocks) != 1 {
		t.Fatalf("expected 1 ContentBlock, got %d", len(res.ContentBlocks))
	}
	if res.ContentBlocks[0].Type != model.ContentImage {
		t.Errorf("ContentBlock type = %q, want image", res.ContentBlocks[0].Type)
	}
}

func TestImageGenerateExecutorSuccessWithOptions(t *testing.T) {
	a := NewImageGenerateAction(nil)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"prompt": "a cute cat",
		"size":   "1024x1024",
		"model":  "dall-e-3",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
}

func TestImageGenerateExecutorMissingPrompt(t *testing.T) {
	a := NewImageGenerateAction(nil)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{})
	if res.OK {
		t.Errorf("expected OK=false for missing prompt")
	}
	if res.Error != "image_generate: prompt is required" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestImageGenerateExecutorGeneratorError(t *testing.T) {
	mock := &MockImageGenerator{Err: errors.New("api limit exceeded")}
	a := NewImageGenerateAction(mock)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"prompt": "a dog",
	})
	if res.OK {
		t.Errorf("expected OK=false when generator errors")
	}
	if res.Error != "image_generate: api limit exceeded" {
		t.Errorf("Error = %q", res.Error)
	}
}

func TestImageGenerateExecutorNilGenerator(t *testing.T) {
	// Passing nil should use MockImageGenerator internally.
	a := NewImageGenerateAction(nil)
	res, _ := a.Executor.Execute(context.Background(), map[string]any{
		"prompt": "test prompt",
	})
	if !res.OK {
		t.Errorf("expected OK=true with nil generator (uses MockImageGenerator)")
	}
}

func TestImageGenerateExecutorURLResult(t *testing.T) {
	// Custom generator that returns a URL instead of inline bytes.
	urlGen := &imageURLGenerator{}
	a := NewImageGenerateAction(urlGen)
	res, err := a.Executor.Execute(context.Background(), map[string]any{
		"prompt": "remote image",
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if len(res.ContentBlocks) != 1 {
		t.Fatalf("expected 1 ContentBlock, got %d", len(res.ContentBlocks))
	}
	if res.ContentBlocks[0].Type != model.ContentImage {
		t.Errorf("ContentBlock type = %q, want image", res.ContentBlocks[0].Type)
	}
	if res.ContentBlocks[0].URL != "https://example.com/image.png" {
		t.Errorf("URL = %q", res.ContentBlocks[0].URL)
	}
}

// imageURLGenerator returns a result with a URL and no inline data.
type imageURLGenerator struct{}

func (g *imageURLGenerator) Generate(ctx context.Context, prompt string, opts ImageGenerateOptions) (*ImageGenerateResult, error) {
	return &ImageGenerateResult{
		URL: "https://example.com/image.png",
	}, nil
}