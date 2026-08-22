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
	"context"
	"errors"
	"testing"
)

func TestGateMultimodal_RejectsKnownNonVision(t *testing.T) {
	// gpt-4 在注册表中 Vision=false → 图片应被拒绝。
	err := gateMultimodal("gpt-4", []ContentBlock{ImageBlock("image/png", []byte{1, 2, 3})})
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Fatalf("want ErrUnsupportedContent, got %v", err)
	}
}

func TestGateMultimodal_AllowsKnownVision(t *testing.T) {
	// gpt-4o 在注册表中 Vision=true → 图片放行。
	if err := gateMultimodal("gpt-4o", []ContentBlock{ImageBlock("image/png", []byte{1})}); err != nil {
		t.Fatalf("gpt-4o should accept image, got %v", err)
	}
	// 纯文本永不触发门控。
	if err := gateMultimodal("gpt-4", []ContentBlock{TextBlock("hi")}); err != nil {
		t.Fatalf("text should never gate, got %v", err)
	}
}

func TestGateMultimodal_AllowsUnknownModel(t *testing.T) {
	// 未知/自定义模型（本地或聚合平台）不应被保守误伤，一律放行。
	if err := gateMultimodal("my-local-vllm-mixture-8x7b", []ContentBlock{ImageBlock("image/png", []byte{1})}); err != nil {
		t.Fatalf("unknown model should pass through, got %v", err)
	}
}

func TestGateMultimodal_RejectsKnownNotAudio(t *testing.T) {
	// gpt-4 无 Audio 标记 → 音频应拒绝。
	err := gateMultimodal("gpt-4", []ContentBlock{AudioBlock("audio/mp3", []byte{1})})
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Fatalf("want ErrUnsupportedContent for audio, got %v", err)
	}
}

// TestOpenAIGenerateRequestData_ImageGate 验证门控穿透到发起 OpenAI 兼容请求的
// OpenAICompatibleProvider：非 vision 模型收到图片在拼接请求前即被拦截。
func TestOpenAIGenerateRequestData_ImageGate(t *testing.T) {
	p := &OpenAICompatibleProvider{Model: "gpt-4"} // Vision=false
	req := &ModelRequest{Input: "describe this", ContentBlocks: []ContentBlock{ImageBlock("image/png", []byte{1})}}
	_, err := p.GenerateRequestData(context.Background(), req)
	if !errors.Is(err, ErrUnsupportedContent) {
		t.Fatalf("want ErrUnsupportedContent, got %v", err)
	}
}

// TestAnthropicGenerateRequestData_ImageGate 验证 Anthropic 路径同样被穿透。
func TestAnthropicGenerateRequestData_ImageGate(t *testing.T) {
	p := &AnthropicCompatibleProvider{Model: "claude-3-opus"} // Vision=true
	req := &ModelRequest{Input: "describe this", ContentBlocks: []ContentBlock{ImageBlock("image/png", []byte{1})}}
	if _, err := p.GenerateRequestData(context.Background(), req); err != nil {
		t.Fatalf("vision claude should accept image, got %v", err)
	}
}