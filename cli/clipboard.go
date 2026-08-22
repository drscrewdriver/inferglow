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
	"errors"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/atotto/clipboard"
)

// ErrClipboardUnavailable is returned when the system clipboard cannot be
// read or written (no platform tool, no image present, etc.).
var ErrClipboardUnavailable = errors.New("clipboard unavailable")

// pendingImageAttachment holds a clipboard image staged for attachment.
// ContentBlocks assembly and submission happen in a later task; here we only
// stage the path plus metadata so the Composer can show an attach line.
type pendingImageAttachment struct {
	Path     string
	MIMEType string
	Width    int
	Height   int
}

// ReadClipboardText reads text from the system clipboard. On WSL it falls
// back to powershell.exe Get-Clipboard when the native backend is missing.
func ReadClipboardText() (string, error) {
	if text, err := clipboard.ReadAll(); err == nil {
		return text, nil
	}
	if isWSL() {
		if text, err := wslGetClipboardText(); err == nil {
			return text, nil
		}
	}
	return "", ErrClipboardUnavailable
}

// WriteClipboardText writes text to the system clipboard using platform
// tools, and additionally emits an OSC 52 escape sequence for terminals that
// support it (e.g. over SSH). Empty text is a no-op.
func WriteClipboardText(text string) error {
	if text == "" {
		return nil
	}
	// OSC 52: best-effort terminal clipboard (works over SSH).
	osc52Copy(text)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("clip")
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		switch {
		case hasExec("xclip"):
			cmd = exec.Command("xclip", "-selection", "clipboard")
		case hasExec("xsel"):
			cmd = exec.Command("xsel", "--clipboard", "--input")
		case hasExec("wl-copy"):
			cmd = exec.Command("wl-copy")
		default:
			return ErrClipboardUnavailable
		}
	default:
		return ErrClipboardUnavailable
	}
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		return ErrClipboardUnavailable
	}
	return nil
}

// ReadClipboardImagePNG reads the current clipboard image as PNG-encoded
// bytes. It returns ErrClipboardUnavailable when no image is present or no
// platform tool is available.
func ReadClipboardImagePNG() ([]byte, error) {
	switch runtime.GOOS {
	case "windows":
		return readClipboardImagePngWindows()
	case "linux":
		return readClipboardImagePngLinux()
	case "darwin":
		return readClipboardImagePngDarwin()
	}
	return nil, ErrClipboardUnavailable
}

// writeTempPNG writes PNG bytes to a new temp file and returns its path.
func writeTempPNG(data []byte, nameHint string) (string, error) {
	pattern := "inferglow-img-*.png"
	if hint := sanitizeNameHint(nameHint); hint != "" {
		pattern = "inferglow-img-" + hint + "-*.png"
	}
	f, err := os.CreateTemp(os.TempDir(), pattern)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// imageSizeOf decodes the dimensions of an image (PNG/JPEG) from its bytes.
func imageSizeOf(data []byte) (w, h int, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

// isWSL reports whether the process runs inside Windows Subsystem for Linux.
func isWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}

// wslGetClipboardText reads text from the Windows clipboard via PowerShell.
func wslGetClipboardText() (string, error) {
	out, err := exec.Command("powershell.exe", "-NoProfile", "-Command", "Get-Clipboard -Raw").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// readClipboardImagePngWindows reads the clipboard image via PowerShell and
// streams its PNG encoding to stdout through an in-memory stream.
func readClipboardImagePngWindows() ([]byte, error) {
	const script = `
Add-Type -AssemblyName System.Drawing
$img = Get-Clipboard -Format Image
if ($null -ne $img) {
    $ms = New-Object System.IO.MemoryStream
    $img.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
    $bytes = $ms.ToArray()
    $out = [System.IO.Console]::OpenStandardOutput()
    $out.Write($bytes, 0, $bytes.Length)
    $out.Flush()
}
`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", script)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, ErrClipboardUnavailable
	}
	if buf.Len() == 0 {
		return nil, ErrClipboardUnavailable
	}
	return buf.Bytes(), nil
}

// readClipboardImagePngLinux reads the clipboard image via wl-paste
// (Wayland) or xclip (X11).
func readClipboardImagePngLinux() ([]byte, error) {
	if out, err := exec.Command("wl-paste", "--type", "image/png").Output(); err == nil && len(out) > 0 {
		return out, nil
	}
	if out, err := exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-o").Output(); err == nil && len(out) > 0 {
		return out, nil
	}
	return nil, ErrClipboardUnavailable
}

// readClipboardImagePngDarwin reads the clipboard image via pngpaste into a
// temp file, then returns the file's bytes.
func readClipboardImagePngDarwin() ([]byte, error) {
	if _, err := exec.LookPath("pngpaste"); err != nil {
		return nil, ErrClipboardUnavailable
	}
	f, err := os.CreateTemp(os.TempDir(), "inferglow-clip-*.png")
	if err != nil {
		return nil, ErrClipboardUnavailable
	}
	path := f.Name()
	f.Close()
	defer os.Remove(path)
	if err := exec.Command("pngpaste", path).Run(); err != nil {
		return nil, ErrClipboardUnavailable
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, ErrClipboardUnavailable
	}
	return data, nil
}

// hasExec reports whether the named executable is available on PATH.
func hasExec(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// sanitizeNameHint strips a name hint down to safe filename characters so it
// can be embedded in a temp-file pattern.
func sanitizeNameHint(nameHint string) string {
	nameHint = filepath.Base(nameHint)
	var b strings.Builder
	for _, r := range nameHint {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	return b.String()
}
