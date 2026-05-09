package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestWorkspace 在 t.TempDir() 下创建 Workspace。
func newTestWorkspace(t *testing.T) *Workspace {
	t.Helper()
	w, err := New(Config{RootDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return w
}

// ============================================================================
// 构造与配置
// ============================================================================

func TestNewEmptyRoot(t *testing.T) {
	_, err := New(Config{RootDir: ""})
	if !errors.Is(err, ErrEmptyRoot) {
		t.Errorf("expected ErrEmptyRoot, got %v", err)
	}
}

func TestNewCreatesRootIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "ws")
	w, err := New(Config{RootDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if info, err := os.Stat(w.Root()); err != nil || !info.IsDir() {
		t.Errorf("Root() = %q not created or not a dir: %v", w.Root(), err)
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	w := newTestWorkspace(t)
	if w.Config().MaxFileSize != DefaultMaxFileSize {
		t.Errorf("MaxFileSize = %d, want %d", w.Config().MaxFileSize, DefaultMaxFileSize)
	}
	if w.Config().MaxFileCount != DefaultMaxFileCount {
		t.Errorf("MaxFileCount = %d, want %d", w.Config().MaxFileCount, DefaultMaxFileCount)
	}
}

func TestNewCustomLimits(t *testing.T) {
	w, err := New(Config{RootDir: t.TempDir(), MaxFileSize: 100, MaxFileCount: 5})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if w.Config().MaxFileSize != 100 {
		t.Errorf("MaxFileSize = %d, want 100", w.Config().MaxFileSize)
	}
	if w.Config().MaxFileCount != 5 {
		t.Errorf("MaxFileCount = %d, want 5", w.Config().MaxFileCount)
	}
}

func TestRootIsCleanedAbs(t *testing.T) {
	dir := t.TempDir()
	w, err := New(Config{RootDir: filepath.Join(dir, "..", filepath.Base(dir))})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Root 应等于 dir（Clean 后）。
	if w.Root() != filepath.Clean(dir) {
		t.Errorf("Root = %q, want %q", w.Root(), filepath.Clean(dir))
	}
}

// ============================================================================
// SafePath 路径穿越防护
// ============================================================================

func TestSafePathEmptyReturnsRoot(t *testing.T) {
	w := newTestWorkspace(t)
	abs, err := w.SafePath("")
	if err != nil {
		t.Fatalf("SafePath(\"\"): %v", err)
	}
	if abs != w.Root() {
		t.Errorf("SafePath(\"\") = %q, want %q", abs, w.Root())
	}
}

func TestSafePathNormalRelative(t *testing.T) {
	w := newTestWorkspace(t)
	abs, err := w.SafePath("foo/bar.txt")
	if err != nil {
		t.Fatalf("SafePath: %v", err)
	}
	want := filepath.Join(w.Root(), "foo", "bar.txt")
	if abs != want {
		t.Errorf("SafePath = %q, want %q", abs, want)
	}
}

func TestSafePathTraversalBlocked(t *testing.T) {
	w := newTestWorkspace(t)
	cases := []string{
		"../outside",
		"../../etc/passwd",
		"foo/../../../outside",
		"../",
	}
	for _, p := range cases {
		_, err := w.SafePath(p)
		if !errors.Is(err, ErrPathOutsideRoot) {
			t.Errorf("SafePath(%q) expected ErrPathOutsideRoot, got %v", p, err)
		}
	}
}

func TestSafePathNullByteBlocked(t *testing.T) {
	w := newTestWorkspace(t)
	_, err := w.SafePath("foo\x00bar")
	if !errors.Is(err, ErrPathOutsideRoot) {
		t.Errorf("SafePath with null byte expected ErrPathOutsideRoot, got %v", err)
	}
}

func TestSafePathDotAndDotDot(t *testing.T) {
	w := newTestWorkspace(t)
	// "." 应解析为 root。
	abs, err := w.SafePath(".")
	if err != nil {
		t.Fatalf("SafePath(\".\"): %v", err)
	}
	if abs != w.Root() {
		t.Errorf("SafePath(\".\") = %q, want %q", abs, w.Root())
	}
	// ".." 应被拒绝。
	_, err = w.SafePath("..")
	if !errors.Is(err, ErrPathOutsideRoot) {
		t.Errorf("SafePath(\"..\") expected ErrPathOutsideRoot, got %v", err)
	}
}

// ============================================================================
// 文件读写
// ============================================================================

func TestWriteAndReadFile(t *testing.T) {
	w := newTestWorkspace(t)
	content := []byte("hello workspace")
	if err := w.WriteFile("test.txt", content); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := w.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("ReadFile = %q, want %q", data, content)
	}
}

func TestWriteFileCreatesParentDirs(t *testing.T) {
	w := newTestWorkspace(t)
	if err := w.WriteFile("a/b/c/d.txt", []byte("deep")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := w.ReadFile("a/b/c/d.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "deep" {
		t.Errorf("data = %q, want %q", data, "deep")
	}
}

func TestWriteFileOverwriteExisting(t *testing.T) {
	w := newTestWorkspace(t)
	_ = w.WriteFile("f.txt", []byte("old"))
	if err := w.WriteFile("f.txt", []byte("new")); err != nil {
		t.Fatalf("WriteFile overwrite: %v", err)
	}
	data, _ := w.ReadFile("f.txt")
	if string(data) != "new" {
		t.Errorf("data = %q, want %q", data, "new")
	}
}

func TestWriteFileTooLarge(t *testing.T) {
	w, err := New(Config{RootDir: t.TempDir(), MaxFileSize: 10})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = w.WriteFile("big.txt", make([]byte, 11))
	if !errors.Is(err, ErrFileTooLarge) {
		t.Errorf("expected ErrFileTooLarge, got %v", err)
	}
}

func TestWriteFileReadOnly(t *testing.T) {
	w, err := New(Config{RootDir: t.TempDir(), ReadOnly: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = w.WriteFile("f.txt", []byte("x"))
	if !errors.Is(err, ErrReadOnly) {
		t.Errorf("expected ErrReadOnly, got %v", err)
	}
}

func TestReadFileOutsideRoot(t *testing.T) {
	w := newTestWorkspace(t)
	_, err := w.ReadFile("../outside")
	if !errors.Is(err, ErrPathOutsideRoot) {
		t.Errorf("expected ErrPathOutsideRoot, got %v", err)
	}
}

func TestReadFileNonExistent(t *testing.T) {
	w := newTestWorkspace(t)
	_, err := w.ReadFile("nope.txt")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist, got %v", err)
	}
}

// ============================================================================
// 文件数量限制
// ============================================================================

func TestWriteFileMaxFileCount(t *testing.T) {
	w, err := New(Config{RootDir: t.TempDir(), MaxFileCount: 3})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := w.WriteFile(string(rune('a'+i)), []byte("x")); err != nil {
			t.Fatalf("WriteFile %d: %v", i, err)
		}
	}
	// 第 4 个文件应被拒绝。
	err = w.WriteFile("d", []byte("x"))
	if !errors.Is(err, ErrTooManyFiles) {
		t.Errorf("expected ErrTooManyFiles, got %v", err)
	}
}

func TestOverwriteDoesNotCountAsNew(t *testing.T) {
	w, err := New(Config{RootDir: t.TempDir(), MaxFileCount: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = w.WriteFile("a.txt", []byte("1"))
	// 覆盖同名文件不应触发数量限制。
	if err := w.WriteFile("a.txt", []byte("2")); err != nil {
		t.Errorf("overwrite should not trigger MaxFileCount: %v", err)
	}
}

func TestFileCount(t *testing.T) {
	w := newTestWorkspace(t)
	_ = w.WriteFile("a.txt", []byte("x"))
	_ = w.WriteFile("b.txt", []byte("x"))
	_ = w.MkdirAll("subdir")
	_ = w.WriteFile("subdir/c.txt", []byte("x"))
	count, err := w.FileCount()
	if err != nil {
		t.Fatalf("FileCount: %v", err)
	}
	if count != 3 {
		t.Errorf("FileCount = %d, want 3", count)
	}
}

// ============================================================================
// 目录操作
// ============================================================================

func TestMkdirAll(t *testing.T) {
	w := newTestWorkspace(t)
	if err := w.MkdirAll("a/b/c"); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	exists, _ := w.Exists("a/b/c")
	if !exists {
		t.Error("MkdirAll did not create a/b/c")
	}
}

func TestMkdirAllReadOnly(t *testing.T) {
	w, _ := New(Config{RootDir: t.TempDir(), ReadOnly: true})
	err := w.MkdirAll("a")
	if !errors.Is(err, ErrReadOnly) {
		t.Errorf("expected ErrReadOnly, got %v", err)
	}
}

func TestRemoveAll(t *testing.T) {
	w := newTestWorkspace(t)
	_ = w.WriteFile("dir/a.txt", []byte("x"))
	if err := w.RemoveAll("dir"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	exists, _ := w.Exists("dir")
	if exists {
		t.Error("RemoveAll did not remove dir")
	}
}

func TestRemoveAllRootBlocked(t *testing.T) {
	w := newTestWorkspace(t)
	err := w.RemoveAll("")
	if !errors.Is(err, ErrPathOutsideRoot) {
		t.Errorf("expected ErrPathOutsideRoot for root removal, got %v", err)
	}
	err = w.RemoveAll(".")
	if !errors.Is(err, ErrPathOutsideRoot) {
		t.Errorf("expected ErrPathOutsideRoot for root removal via \".\", got %v", err)
	}
}

func TestRemoveAllReadOnly(t *testing.T) {
	w, _ := New(Config{RootDir: t.TempDir(), ReadOnly: true})
	err := w.RemoveAll("a")
	if !errors.Is(err, ErrReadOnly) {
		t.Errorf("expected ErrReadOnly, got %v", err)
	}
}

func TestRemoveAllOutsideRoot(t *testing.T) {
	w := newTestWorkspace(t)
	err := w.RemoveAll("../outside")
	if !errors.Is(err, ErrPathOutsideRoot) {
		t.Errorf("expected ErrPathOutsideRoot, got %v", err)
	}
}

// ============================================================================
// ListDir
// ============================================================================

func TestListDir(t *testing.T) {
	w := newTestWorkspace(t)
	_ = w.WriteFile("b.txt", []byte("x"))
	_ = w.WriteFile("a.txt", []byte("x"))
	_ = w.MkdirAll("subdir")

	names, err := w.ListDir("")
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	// 应包含 a.txt, b.txt, subdir，且按字典序。
	want := []string{"a.txt", "b.txt", "subdir"}
	if len(names) != len(want) {
		t.Fatalf("ListDir = %v, want %v", names, want)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("ListDir[%d] = %q, want %q", i, n, want[i])
		}
	}
}

func TestListDirSubdirectory(t *testing.T) {
	w := newTestWorkspace(t)
	_ = w.WriteFile("sub/x.txt", []byte("x"))
	_ = w.WriteFile("sub/y.txt", []byte("x"))

	names, err := w.ListDir("sub")
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	want := []string{"x.txt", "y.txt"}
	if len(names) != len(want) {
		t.Fatalf("ListDir = %v, want %v", names, want)
	}
}

func TestListDirNonExistent(t *testing.T) {
	w := newTestWorkspace(t)
	_, err := w.ListDir("nope")
	if err == nil {
		t.Error("expected error for non-existent dir")
	}
}

// ============================================================================
// Exists / Stat
// ============================================================================

func TestExists(t *testing.T) {
	w := newTestWorkspace(t)
	_ = w.WriteFile("f.txt", []byte("x"))

	exists, _ := w.Exists("f.txt")
	if !exists {
		t.Error("Exists should return true for existing file")
	}
	exists, _ = w.Exists("nope.txt")
	if exists {
		t.Error("Exists should return false for non-existent file")
	}
}

func TestStat(t *testing.T) {
	w := newTestWorkspace(t)
	_ = w.WriteFile("f.txt", []byte("hello"))
	info, err := w.Stat("f.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() != 5 {
		t.Errorf("Size = %d, want 5", info.Size())
	}
	if info.IsDir() {
		t.Error("IsDir = true, want false")
	}
}

// ============================================================================
// 边界情况
// ============================================================================

func TestWriteFileEmptyContent(t *testing.T) {
	w := newTestWorkspace(t)
	if err := w.WriteFile("empty.txt", []byte{}); err != nil {
		t.Fatalf("WriteFile empty: %v", err)
	}
	data, err := w.ReadFile("empty.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("data len = %d, want 0", len(data))
	}
}

func TestWriteFileAtRootLevel(t *testing.T) {
	w := newTestWorkspace(t)
	if err := w.WriteFile("root.txt", []byte("x")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	exists, _ := w.Exists("root.txt")
	if !exists {
		t.Error("root-level file not created")
	}
}

func TestSafePathAbsoluteBlocked(t *testing.T) {
	w := newTestWorkspace(t)
	// 绝对路径应被拒绝。
	// 在 Windows 上使用 "C:\Windows" 形式；在 Unix 上使用 "/etc/passwd"。
	var absPath string
	if filepath.Separator == '\\' {
		// Windows：使用一个不存在的盘符的绝对路径。
		absPath = "Z:\\Windows\\system32"
	} else {
		absPath = "/etc/passwd"
	}
	_, err := w.SafePath(absPath)
	if err == nil {
		t.Errorf("SafePath(%q) expected error for absolute path, got nil", absPath)
	}
}

func TestConcurrentWriteDifferentFiles(t *testing.T) {
	w := newTestWorkspace(t)
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			name := string(rune('a'+i)) + ".txt"
			done <- w.WriteFile(name, []byte("x"))
		}(i)
	}
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent write %d: %v", i, err)
		}
	}
	count, _ := w.FileCount()
	if count != 10 {
		t.Errorf("FileCount = %d, want 10", count)
	}
}

// ============================================================================
// Round-trip 集成测试
// ============================================================================

func TestIntegrationWriteReadListRemove(t *testing.T) {
	w := newTestWorkspace(t)

	// 写入多个文件。
	files := map[string]string{
		"README.md":    "# Project",
		"src/main.go":  "package main",
		"src/util.go":  "package main",
		"docs/intro.md": "# Intro",
	}
	for name, content := range files {
		if err := w.WriteFile(name, []byte(content)); err != nil {
			t.Fatalf("WriteFile(%q): %v", name, err)
		}
	}

	// 读取验证。
	for name, content := range files {
		data, err := w.ReadFile(name)
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", name, err)
		}
		if string(data) != content {
			t.Errorf("ReadFile(%q) = %q, want %q", name, data, content)
		}
	}

	// 列出 root。
	rootEntries, _ := w.ListDir("")
	if !contains(rootEntries, "README.md") || !contains(rootEntries, "src") || !contains(rootEntries, "docs") {
		t.Errorf("root entries = %v, want README.md/src/docs", rootEntries)
	}

	// 列出 src/。
	srcEntries, _ := w.ListDir("src")
	if !contains(srcEntries, "main.go") || !contains(srcEntries, "util.go") {
		t.Errorf("src entries = %v, want main.go/util.go", srcEntries)
	}

	// 删除 src/。
	if err := w.RemoveAll("src"); err != nil {
		t.Fatalf("RemoveAll(src): %v", err)
	}
	exists, _ := w.Exists("src/main.go")
	if exists {
		t.Error("src/main.go still exists after RemoveAll(src)")
	}

	// 文件计数应为剩余 2 个（README.md + docs/intro.md）。
	count, _ := w.FileCount()
	if count != 2 {
		t.Errorf("FileCount after delete = %d, want 2", count)
	}
}

func contains(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}

// 确保 ErrFileTooLarge 的错误消息包含限制信息。
func TestErrFileTooLargeMessage(t *testing.T) {
	w, _ := New(Config{RootDir: t.TempDir(), MaxFileSize: 5})
	err := w.WriteFile("big.txt", make([]byte, 10))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("error should mention limit, got: %v", err)
	}
}
