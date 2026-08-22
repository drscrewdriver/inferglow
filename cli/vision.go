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

package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/inferglow/model"
	"github.com/inferglow/orchestrator/agent"
)

// buildImageBlocks reads an image file and returns a single Chat ContentBlock
// ready to be sent via agent.WithContentBlocks (the engine multimodal channel,
// see #2). MIME is inferred from the file extension; unknown types fall back
// to image/png so the block is still serializable.
func buildImageBlocks(path string) ([]model.ContentBlock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("vision: read image: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("vision: empty image file: %s", path)
	}
	return []model.ContentBlock{model.ImageBlock(mimeForPath(path), data)}, nil
}

// mimeForPath infers a MIME type from a file extension. Unknown extensions
// fall back to image/png so the content block stays serializable.
func mimeForPath(path string) string {
	switch {
	case hasExt(path, ".png"):
		return "image/png"
	case hasExt(path, ".jpg"), hasExt(path, ".jpeg"):
		return "image/jpeg"
	case hasExt(path, ".gif"):
		return "image/gif"
	case hasExt(path, ".webp"):
		return "image/webp"
	default:
		return "image/png"
	}
}

func hasExt(path, ext string) bool {
	lower := path
	if len(lower) >= len(ext) && lower[len(lower)-len(ext):] == ext {
		return true
	}
	return false
}

// visionPrompt returns the system prompt instructing the model to describe /
// answer about the attached image (the "看图"/read-screen agent behavior).
const visionPrompt = "You are looking at an image attached to this message. " +
	"Answer the user's question about it. Describe what you see concisely and " +
	"factually; do not claim to see elements that are not in the image."

// runVision executes a vision turn against the model: it attaches the image at
// path and asks the given question (defaulting to a description request), then
// commits the model's answer to the transcript. Runs asynchronously so the TUI
// keeps rendering.
func (m *chatTUI) runVision(path, question string) {
	if question == "" {
		question = "Describe this image in detail."
	}
	blocks, err := buildImageBlocks(path)
	if err != nil {
		m.commitLine(errorText(err.Error()))
		return
	}

	m.commitLine("")
	m.commitUserBubble(fmt.Sprintf("‹vision %s› %s", path, question))
	m.commitSystemNote(dim(fmt.Sprintf("Sending image to %s …", m.modelLabel)))

	go func() {
		resp, err := m.agent.Run(context.Background(), question,
			agent.WithSystemPrompt(visionPrompt),
			agent.WithContentBlocks(blocks),
		)
		if err != nil {
			m.commitLine(errorText(fmt.Sprintf("vision error: %v", err)))
			return
		}
		m.commitBlock(blockAssistant, resp, resp)
		m.transcriptDirty = true
	}()
}