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

// Package workspace 提供 Agent 工作空间的核心能力：安全的文件读写、目录操作、
// 路径管理（防止路径穿越）以及文件大小/数量限制。
//
// Workspace 以一个根目录（rootDir）为沙箱边界，所有操作路径必须解析到 rootDir
// 内部。通过 filepath.Clean + filepath.Join + 前缀校验三重防护阻止路径穿越攻击。
//
// 注意：血缘追踪（LineageStore）为独立可选组件，需调用方显式集成，不自动嵌入文件操作。
package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// 默认限制。
const (
	DefaultMaxFileSize    int64 = 10 * 1024 * 1024 // 10 MB
	DefaultMaxFileCount   int   = 10000
	DefaultMaxFileNameLen int   = 255
)

// 错误哨兵。
var (
	ErrPathOutsideRoot = errors.New("path outside workspace root")
	ErrFileTooLarge    = errors.New("file exceeds size limit")
	ErrTooManyFiles    = errors.New("file count exceeds limit")
	ErrFileNameTooLong = errors.New("file name too long")
	ErrEmptyRoot       = errors.New("workspace root is empty")
	ErrReadOnly        = errors.New("workspace is read-only")
)

// Config 控制 Workspace 的行为限制。
type Config struct {
	// RootDir 是工作空间的根目录（绝对路径）。所有操作路径必须解析到其内部。
	RootDir string
	// MaxFileSize 单个文件最大字节数；0 表示使用 DefaultMaxFileSize。
	MaxFileSize int64
	// MaxFileCount 工作空间内文件总数上限；0 表示使用 DefaultMaxFileCount。
	MaxFileCount int
	// ReadOnly 为 true 时禁止所有写操作（WriteFile/MkdirAll/RemoveAll）。
	ReadOnly bool
}

// Workspace 提供沙箱化的文件/目录操作。
//
// 所有公开方法接收的 path 参数均为相对路径（相对于 RootDir）。
// 路径穿越（如 "../../etc/passwd"）会被 SafePath 拦截并返回 ErrPathOutsideRoot。
type Workspace struct {
	mu   sync.RWMutex
	cfg  Config
	root string // 已 Clean 的绝对路径
}

// New 创建并初始化 Workspace。若 RootDir 不存在则自动创建。
func New(cfg Config) (*Workspace, error) {
	if cfg.RootDir == "" {
		return nil, ErrEmptyRoot
	}
	root, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve root dir: %w", err)
	}
	root = filepath.Clean(root)
	if cfg.MaxFileSize <= 0 {
		cfg.MaxFileSize = DefaultMaxFileSize
	}
	if cfg.MaxFileCount <= 0 {
		cfg.MaxFileCount = DefaultMaxFileCount
	}
	// 确保根目录存在。
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create root dir: %w", err)
	}
	return &Workspace{cfg: cfg, root: root}, nil
}

// Root 返回工作空间的绝对根路径（已 Clean）。
func (w *Workspace) Root() string { return w.root }

// Config 返回当前配置的副本。
func (w *Workspace) Config() Config { return w.cfg }

// SafePath 将相对路径解析为绝对路径，并校验其位于 root 内部。
//
// 规则：
//   - 空路径返回 root 本身
//   - 绝对路径（含 Windows 盘符路径如 "C:\..." 或 UNC 路径）被拒绝
//   - 路径先 Clean 再 Join 到 root
//   - 清洗后的绝对路径必须等于 root 或以 root + 分隔符 开头
//   - 路径各段不允许包含空字节
//
// 返回的绝对路径始终已 filepath.Clean。
func (w *Workspace) SafePath(relative string) (string, error) {
	if strings.ContainsRune(relative, 0) {
		return "", fmt.Errorf("%w: path contains null byte", ErrPathOutsideRoot)
	}
	if relative == "" {
		return w.root, nil
	}
	// 拒绝绝对路径（含 Windows 盘符路径 "C:\..." 和 Unix "/..."）。
	if filepath.IsAbs(relative) {
		return "", fmt.Errorf("%w: absolute path not allowed: %q", ErrPathOutsideRoot, relative)
	}
	// 先 Clean 相对路径，再 Join 到 root。
	cleaned := filepath.Clean(relative)
	abs := filepath.Join(w.root, cleaned)
	abs = filepath.Clean(abs)
	if !w.isInsideRoot(abs) {
		return "", fmt.Errorf("%w: %q resolves to %q", ErrPathOutsideRoot, relative, abs)
	}
	return abs, nil
}

// isInsideRoot 判断 abs 是否等于 root 或位于 root 内部。
func (w *Workspace) isInsideRoot(abs string) bool {
	if abs == w.root {
		return true
	}
	prefix := w.root + string(filepath.Separator)
	return strings.HasPrefix(abs, prefix)
}

// ReadFile 读取工作空间内的文件。
func (w *Workspace) ReadFile(path string) ([]byte, error) {
	abs, err := w.SafePath(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > w.cfg.MaxFileSize {
		return nil, fmt.Errorf("%w: %d bytes (limit %d)", ErrFileTooLarge, len(data), w.cfg.MaxFileSize)
	}
	return data, nil
}

// WriteFile 向工作空间写入文件。文件大小不得超过 MaxFileSize。
// 若工作空间内文件总数超过 MaxFileCount，拒绝写入。
func (w *Workspace) WriteFile(path string, data []byte) error {
	if w.cfg.ReadOnly {
		return fmt.Errorf("%w: WriteFile %q", ErrReadOnly, path)
	}
	if int64(len(data)) > w.cfg.MaxFileSize {
		return fmt.Errorf("%w: %d bytes (limit %d)", ErrFileTooLarge, len(data), w.cfg.MaxFileSize)
	}
	abs, err := w.SafePath(path)
	if err != nil {
		return err
	}
	// 校验文件名长度。
	if name := filepath.Base(abs); len(name) > DefaultMaxFileNameLen {
		return fmt.Errorf("%w: %d (limit %d)", ErrFileNameTooLong, len(name), DefaultMaxFileNameLen)
	}
	// 确保父目录存在。
	parent := filepath.Dir(abs)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	// 若是新文件，校验文件数量限制。
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		count, err := w.countFilesLocked()
		if err != nil {
			return fmt.Errorf("count files: %w", err)
		}
		if count >= w.cfg.MaxFileCount {
			return fmt.Errorf("%w: %d (limit %d)", ErrTooManyFiles, count, w.cfg.MaxFileCount)
		}
	}
	// 写入：0o644 权限。
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		return err
	}
	return nil
}

// MkdirAll 在工作空间内递归创建目录。
func (w *Workspace) MkdirAll(path string) error {
	if w.cfg.ReadOnly {
		return fmt.Errorf("%w: MkdirAll %q", ErrReadOnly, path)
	}
	abs, err := w.SafePath(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(abs, 0o755)
}

// RemoveAll 删除工作空间内的文件或目录树。
// 拒绝删除 root 本身（path 为空或 "." 或 "/"）。
func (w *Workspace) RemoveAll(path string) error {
	if w.cfg.ReadOnly {
		return fmt.Errorf("%w: RemoveAll %q", ErrReadOnly, path)
	}
	abs, err := w.SafePath(path)
	if err != nil {
		return err
	}
	if abs == w.root {
		return fmt.Errorf("%w: cannot remove workspace root", ErrPathOutsideRoot)
	}
	return os.RemoveAll(abs)
}

// ListDir 列出工作空间内指定目录下的条目名称（不含路径前缀）。
// 若 path 为空，列出 root。
// 不递归。返回的名称按字典序排序。
func (w *Workspace) ListDir(path string) ([]string, error) {
	abs, err := w.SafePath(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	// 简单插入排序，保证稳定且无额外依赖。
	for i := 1; i < len(names); i++ {
		j := i
		for j > 0 && names[j] < names[j-1] {
			names[j], names[j-1] = names[j-1], names[j]
			j--
		}
	}
	return names, nil
}

// Stat 返回工作空间内文件/目录的信息。
func (w *Workspace) Stat(path string) (os.FileInfo, error) {
	abs, err := w.SafePath(path)
	if err != nil {
		return nil, err
	}
	return os.Stat(abs)
}

// Exists 判断工作空间内路径是否存在。
func (w *Workspace) Exists(path string) (bool, error) {
	abs, err := w.SafePath(path)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(abs)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// countFilesLocked 统计工作空间内的文件总数（不含目录）。
// 调用者必须持有 w.mu。
func (w *Workspace) countFilesLocked() (int, error) {
	count := 0
	err := filepath.WalkDir(w.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d != nil && !d.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

// FileCount 返回工作空间内的文件总数（不含目录）。
func (w *Workspace) FileCount() (int, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.countFilesLocked()
}
