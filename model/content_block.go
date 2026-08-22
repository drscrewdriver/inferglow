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
	"fmt"
)

// ContentType classifies the kind of content carried by a ContentBlock.
type ContentType string

const (
	// ContentText is plain text content.
	ContentText ContentType = "text"
	// ContentImage is image content (PNG, JPEG, WebP, GIF).
	ContentImage ContentType = "image"
	// ContentAudio is audio content (MP3, WAV, OGG, FLAC).
	ContentAudio ContentType = "audio"
	// ContentVideo is video content (MP4, WebM).
	ContentVideo ContentType = "video"
	// ContentFile is generic file content (PDF, DOCX, etc.).
	ContentFile ContentType = "file"
)

// ContentBlock is the unified multimodal content type used across
// ModelRequest (input), StreamChunk (streaming output), and
// ActionResult (tool output).
//
// It replaces the legacy Attachment{Type string, Data any} stub with
// strongly-typed content classification and explicit MIME type.
//
// A ContentBlock carries content in exactly one of two forms:
//   - Inline: Data is non-nil (raw bytes)
//   - Remote: URL is non-empty (provider fetches or references)
//
// Meta is an optional side-channel for provider-specific hints
// (e.g. image dimensions, audio duration, file name).
type ContentBlock struct {
	// Type classifies the content (text, image, audio, video, file).
	Type ContentType `json:"type"`
	// MIMEType is the IANA media type (e.g. "image/png", "audio/mp3").
	// May be empty for text content.
	MIMEType string `json:"mime_type,omitempty"`
	// Data carries inline content bytes. Mutually exclusive with URL.
	Data []byte `json:"data,omitempty"`
	// URL references remote content. Mutually exclusive with Data.
	URL string `json:"url,omitempty"`
	// Meta is an optional side-channel for provider-specific hints.
	Meta map[string]any `json:"meta,omitempty"`
}

// IsInline reports whether the block carries inline data.
func (b *ContentBlock) IsInline() bool {
	return len(b.Data) > 0
}

// IsRemote reports whether the block references a remote URL.
func (b *ContentBlock) IsRemote() bool {
	return b.URL != ""
}

// --- Convenience constructors ---

// TextBlock creates a text ContentBlock.
func TextBlock(text string) ContentBlock {
	return ContentBlock{
		Type: ContentText,
		Data: []byte(text),
	}
}

// ImageBlock creates an image ContentBlock from inline bytes.
func ImageBlock(mimeType string, data []byte) ContentBlock {
	return ContentBlock{
		Type:     ContentImage,
		MIMEType: mimeType,
		Data:     data,
	}
}

// ImageURLBlock creates an image ContentBlock from a remote URL.
func ImageURLBlock(url string) ContentBlock {
	return ContentBlock{
		Type: ContentImage,
		URL:  url,
	}
}

// AudioBlock creates an audio ContentBlock from inline bytes.
func AudioBlock(mimeType string, data []byte) ContentBlock {
	return ContentBlock{
		Type:     ContentAudio,
		MIMEType: mimeType,
		Data:     data,
	}
}

// AudioURLBlock creates an audio ContentBlock from a remote URL.
func AudioURLBlock(url string) ContentBlock {
	return ContentBlock{
		Type: ContentAudio,
		URL:  url,
	}
}

// FileBlock creates a generic file ContentBlock.
func FileBlock(mimeType string, data []byte, meta map[string]any) ContentBlock {
	return ContentBlock{
		Type:     ContentFile,
		MIMEType: mimeType,
		Data:     data,
		Meta:     meta,
	}
}

// HasContentBlocks reports whether the slice contains any blocks of the
// given type. Useful for quickly checking multimodal presence.
func HasContentBlocks(blocks []ContentBlock, ct ContentType) bool {
	for i := range blocks {
		if blocks[i].Type == ct {
			return true
		}
	}
	return false
}

// FilterContentBlocks returns a new slice containing only blocks of the
// specified type.
func FilterContentBlocks(blocks []ContentBlock, ct ContentType) []ContentBlock {
	var out []ContentBlock
	for i := range blocks {
		if blocks[i].Type == ct {
			out = append(out, blocks[i])
		}
	}
	return out
}

// ExtractText joins all text ContentBlocks into a single string.
func ExtractText(blocks []ContentBlock) string {
	var s string
	for i := range blocks {
		if blocks[i].Type == ContentText && len(blocks[i].Data) > 0 {
			if s != "" {
				s += "\n"
			}
			s += string(blocks[i].Data)
		}
	}
	return s
}

// ErrUnsupportedContent 表示当前模型不支持请求携带的某种媒体内容块
//（如非 vision 模型收到图片输入）。
var ErrUnsupportedContent = errors.New("model does not support requested content")

// gateMultimodal 在模型层对媒体 ContentBlock 做**已知能力**门控。
//
// 背景：模型能力注册表（ModelCapabilityRegistry）只覆盖已知模型。若对未知模型
// 一律保守拒绝，会误伤本地部署 / 聚合平台（OpenRouter、SiliconFlow）/ 企业网关
// 上传的任意自定义模型名。因此策略为：
//
//   - 模型**已知**且明确缺少对应能力（如 Vision=false）→ 拒绝并给出可读错误；
//   - 模型**未知**或能力标记为 true → 放行，交由 provider 序列化（未知模型交由
//     上游自行裁决），避免在接入侧浪费 token 前就被错误拦截。
//
// 各 provider 的 GenerateRequestData 在拼接 ContentBlocks 之前调用此函数。
func gateMultimodal(modelName string, blocks []ContentBlock) error {
	cap, found := LookupModelCapability(modelName)
	if !found {
		return nil
	}
	for i := range blocks {
		switch blocks[i].Type {
		case ContentImage:
			if !cap.Vision {
				return fmt.Errorf("%w: %s does not accept image input (Vision=false)", ErrUnsupportedContent, modelName)
			}
		case ContentAudio:
			if !cap.Audio {
				return fmt.Errorf("%w: %s does not accept audio input (Audio=false)", ErrUnsupportedContent, modelName)
			}
		case ContentVideo:
			if !cap.Video {
				return fmt.Errorf("%w: %s does not accept video input (Video=false)", ErrUnsupportedContent, modelName)
			}
		}
	}
	return nil
}
