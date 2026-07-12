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
	"fmt"

	"github.com/inferglow/action"
	"github.com/inferglow/model"
)

// ImageGenerateActionID is the registered Action name for image generation.
const ImageGenerateActionID = "image_generate"

// ImageGenerateSpec is the ActionSpec for image_generate.
var ImageGenerateSpec = &action.ActionSpec{
	ActionID:         ImageGenerateActionID,
	Name:             "ImageGenerate",
	Description:      "Generate an image from a text prompt using DALL-E, Stable Diffusion, or compatible API.",
	SideEffectLevel:  action.SideEffectWrite,
	ApprovalRequired: false,
	SandboxRequired:  false,
	ReplaySafe:       true,
	ExposeToModel:    true,
	Tags:             []string{"multimodal", "image", "generation", "builtin"},
	Kwargs: map[string]any{
		"prompt": map[string]any{"type": "string", "required": true, "description": "Text description of the image to generate"},
		"size":   map[string]any{"type": "string", "required": false, "description": "Image size, e.g. '1024x1024', '1792x1024'"},
		"model":  map[string]any{"type": "string", "required": false, "description": "Model name, e.g. 'dall-e-3', 'stable-diffusion-xl'"},
	},
}

// ImageGenerator is the abstraction for image generation backends.
// Implementations may wrap OpenAI DALL-E, Stability AI, Replicate, etc.
type ImageGenerator interface {
	Generate(ctx context.Context, prompt string, opts ImageGenerateOptions) (*ImageGenerateResult, error)
}

// ImageGenerateOptions carries optional parameters for image generation.
type ImageGenerateOptions struct {
	Size  string // e.g. "1024x1024"
	Model string // e.g. "dall-e-3"
}

// ImageGenerateResult is the output of an image generation call.
type ImageGenerateResult struct {
	ImageData []byte // raw image bytes (PNG/JPEG)
	MIMEType  string // e.g. "image/png"
	URL       string // remote URL (if provider returns URL instead of bytes)
}

// MockImageGenerator is a deterministic mock for testing.
type MockImageGenerator struct {
	Err error
}

func (m *MockImageGenerator) Generate(ctx context.Context, prompt string, opts ImageGenerateOptions) (*ImageGenerateResult, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	// Return a tiny 1x1 PNG as placeholder
	return &ImageGenerateResult{
		ImageData: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, // PNG header
		MIMEType:  "image/png",
	}, nil
}

// imageGenerateExecutor binds an ImageGenerator to the ActionExecutor contract.
type imageGenerateExecutor struct {
	generator ImageGenerator
}

func (e *imageGenerateExecutor) Execute(ctx context.Context, input map[string]any) (*action.ActionResult, error) {
	if e == nil || e.generator == nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "image_generate: no image generator configured",
		}, nil
	}
	prompt, _ := input["prompt"].(string)
	if prompt == "" {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  "image_generate: prompt is required",
		}, nil
	}
	opts := ImageGenerateOptions{}
	if s, ok := input["size"].(string); ok {
		opts.Size = s
	}
	if m, ok := input["model"].(string); ok {
		opts.Model = m
	}
	result, err := e.generator.Generate(ctx, prompt, opts)
	if err != nil {
		return &action.ActionResult{
			OK:     false,
			Status: "error",
			Error:  fmt.Sprintf("image_generate: %s", err.Error()),
		}, nil
	}
	// Build ContentBlock for the generated image
	var blocks []model.ContentBlock
	if len(result.ImageData) > 0 {
		blocks = append(blocks, model.ImageBlock(result.MIMEType, result.ImageData))
	} else if result.URL != "" {
		blocks = append(blocks, model.ImageURLBlock(result.URL))
	}
	return &action.ActionResult{
		OK:            true,
		Status:        "success",
		Result:        map[string]any{"prompt": prompt, "mime_type": result.MIMEType},
		ContentBlocks: blocks,
	}, nil
}

// NewImageGenerateAction builds an Action for image generation.
// If generator is nil, a MockImageGenerator is used.
func NewImageGenerateAction(generator ImageGenerator) *action.Action {
	if generator == nil {
		generator = &MockImageGenerator{}
	}
	return &action.Action{
		Name:        ImageGenerateActionID,
		Description: "Generate an image from a text prompt.",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{"type": "string", "description": "Text description of the image"},
				"size":   map[string]any{"type": "string", "description": "Image size (e.g. 1024x1024)"},
				"model":  map[string]any{"type": "string", "description": "Model name (e.g. dall-e-3)"},
			},
			"required": []string{"prompt"},
		},
		Executor: &imageGenerateExecutor{generator: generator},
		Tags:     []string{"multimodal", "image", "generation", "builtin"},
	}
}
