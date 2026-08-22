package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/inferglow/model"
)

func TestBuildImageBlocks_ReadsAndClassifies(t *testing.T) {
	p := filepath.Join(t.TempDir(), "snap.png")
	if err := os.WriteFile(p, []byte{0x89, 0x50, 0x4e, 0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	blocks, err := buildImageBlocks(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	if blocks[0].Type != model.ContentImage || blocks[0].MIMEType != "image/png" {
		t.Errorf("unexpected block: type=%s mime=%s", blocks[0].Type, blocks[0].MIMEType)
	}
}

func TestBuildImageBlocks_MissingFile(t *testing.T) {
	if _, err := buildImageBlocks(filepath.Join(t.TempDir(), "nope.png")); err == nil {
		t.Fatal("want error for missing image")
	}
}

func TestMimeForPath(t *testing.T) {
	cases := map[string]string{
		"a.png":  "image/png",
		"a.jpeg": "image/jpeg",
		"a.gif":  "image/gif",
		"a.webp": "image/webp",
		"a.bin":  "image/png", // default fallback
	}
	for in, want := range cases {
		if got := mimeForPath(in); got != want {
			t.Errorf("mimeForPath(%q) = %q, want %q", in, got, want)
		}
	}
}