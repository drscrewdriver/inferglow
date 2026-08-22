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
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO
// EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES
// OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package cli

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// buildTestPNG constructs a small 3x2 in-memory PNG.
func buildTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestWriteTempPNG(t *testing.T) {
	data := buildTestPNG(t)
	path, err := writeTempPNG(data, "unit-test")
	if err != nil {
		t.Fatalf("writeTempPNG: %v", err)
	}
	defer os.Remove(path)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("round-trip mismatch: got %d bytes, want %d", len(got), len(data))
	}
}

func TestImageSizeOf(t *testing.T) {
	data := buildTestPNG(t)
	w, h, err := imageSizeOf(data)
	if err != nil {
		t.Fatalf("imageSizeOf: %v", err)
	}
	if w != 3 || h != 2 {
		t.Fatalf("imageSizeOf = (%d, %d), want (3, 2)", w, h)
	}
	if _, _, err := imageSizeOf([]byte("not an image")); err == nil {
		t.Fatal("imageSizeOf should fail on invalid data")
	}
}

func TestWriteClipboardTextEmpty(t *testing.T) {
	if err := WriteClipboardText(""); err != nil {
		t.Fatalf("empty text should be a no-op, got: %v", err)
	}
}

func TestReadClipboardImagePNGNoPanic(t *testing.T) {
	// Must not panic on this machine regardless of which platform tools are
	// available; the result may be a real image or ErrClipboardUnavailable.
	_, _ = ReadClipboardImagePNG()
}
