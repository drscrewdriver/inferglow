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
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// R2：会话 Fork 与 Ephemeral 的行为测试。
// - Fork：深拷贝当前会话状态生成新 ID，原会话不受影响；
// - NewEphemeralSession：进程内存会话，全程不产生持久化文件。

package session

import (
	"os"
	"path/filepath"
	"testing"
)

// TestForkDeepCopyIndependence 验证 Fork 的深拷贝独立性：
// 构造带消息/上下文/memo 的会话 A → A.Fork() 得 B（新 ID）→
// 向 B 追加消息后 A 的历史不变，且嵌套结构（ContentBlock/Meta/Memo）
// 上改 B 不影响 A。
func TestForkDeepCopyIndependence(t *testing.T) {
	a := NewSession("origin", 4096)

	// 构造带嵌套结构的会话状态：多模态消息、Meta、memo、resize 配置
	a.AddMessage("system", "You are helpful.", "")
	a.AddMessageWithMeta("user", "hello", "alice", map[string]any{"tool_call_id": "tc-1"})
	a.AddMessage("assistant", []ContentBlock{
		{Type: "text", Data: "answer", Meta: map[string]any{"lang": "en"}},
		{Type: "image", Data: "http://example.com/img.png"},
	}, "")
	a.Memo["topic"] = "fork-test"
	a.AutoResize = true
	a.PromptVersion = "v9"
	a.RegisterResizeHandler("simple_cut", SimpleCutResizeHandler)
	if err := a.SetDefaultResizeHandler("simple_cut"); err != nil {
		t.Fatal(err)
	}

	fcBefore := len(a.GetFullContext())

	b := a.Fork()

	// 1) 生成新 ID 且非空
	if b.ID == "" {
		t.Fatal("fork 会话 ID 为空")
	}
	if b.ID == a.ID {
		t.Fatalf("fork 会话应生成新 ID，得到与原会话相同的 %q", b.ID)
	}

	// 2) 副本状态与原会话一致
	if got := len(b.GetFullContext()); got != fcBefore {
		t.Fatalf("fork FullContext 长度 = %d, want %d", got, fcBefore)
	}
	if got := len(b.GetContextWindow()); got != fcBefore {
		t.Fatalf("fork ContextWindow 长度 = %d, want %d", got, fcBefore)
	}
	if b.MaxLength != a.MaxLength {
		t.Fatalf("fork MaxLength = %d, want %d", b.MaxLength, a.MaxLength)
	}
	if !b.AutoResize {
		t.Fatal("fork 未继承 AutoResize 配置")
	}
	if b.PromptVersion != a.PromptVersion {
		t.Fatalf("fork PromptVersion = %q, want %q", b.PromptVersion, a.PromptVersion)
	}
	if b.Memo["topic"] != "fork-test" {
		t.Fatalf("fork Memo 缺少原会话内容: %+v", b.Memo)
	}
	if _, ok := b.resizeHandlers["simple_cut"]; !ok {
		t.Fatal("fork 未继承 resize 策略注册表")
	}
	if b.defaultResizeName != "simple_cut" {
		t.Fatalf("fork defaultResizeName = %q, want simple_cut", b.defaultResizeName)
	}

	// 3) 向 B 追加消息：A 的历史不变
	b.AddMessage("user", "only-for-fork", "")
	if got := len(a.GetFullContext()); got != fcBefore {
		t.Fatalf("向 fork 追加消息后原会话历史被改动: %d != %d", got, fcBefore)
	}
	if got := len(b.GetFullContext()); got != fcBefore+1 {
		t.Fatalf("fork 会话追加消息失败: %d != %d", got, fcBefore+1)
	}

	// 4) 嵌套结构改 B 不影响 A
	// 4a. Memo 加键
	b.Memo["extra"] = "b-only"
	if _, ok := a.Memo["extra"]; ok {
		t.Fatal("向 fork 的 Memo 加键泄漏到了原会话")
	}
	// 4b. 消息 ContentBlock 内容修改
	bBlocks, ok := b.ContextWindow[2].Content.([]ContentBlock)
	if !ok || len(bBlocks) == 0 {
		t.Fatal("fork 消息的 Content 应为 []ContentBlock")
	}
	bBlocks[0].Data = "mutated-by-fork"
	aBlocks, ok := a.ContextWindow[2].Content.([]ContentBlock)
	if !ok || len(aBlocks) == 0 {
		t.Fatal("原会话消息的 Content 应为 []ContentBlock")
	}
	if aBlocks[0].Data != "answer" {
		t.Fatalf("修改 fork 的 ContentBlock 泄漏到了原会话: %v", aBlocks[0].Data)
	}
	// 4c. 消息 Meta 加键
	b.ContextWindow[1].Meta["new"] = true
	if _, ok := a.ContextWindow[1].Meta["new"]; ok {
		t.Fatal("向 fork 消息的 Meta 加键泄漏到了原会话")
	}
}

// TestForkMultipleUniqueIDs 验证同一会话多次 Fork 生成的 ID 互不相同。
func TestForkMultipleUniqueIDs(t *testing.T) {
	a := NewSession("multi", 100)
	a.AddMessage("user", "hi", "")
	b1 := a.Fork()
	b2 := a.Fork()
	if b1.ID == b2.ID {
		t.Fatalf("两次 Fork 生成了相同 ID: %q", b1.ID)
	}
}

// TestNewEphemeralSessionNoPersistenceFiles 验证 ephemeral 会话
// 运行/追加消息全程不产生任何持久化文件：即使上层显式调用
// SaveJSON/SaveYAML（模拟误挂的持久化路径）也不会在磁盘上留下
// JSONL/JSON/YAML 文件；对比普通 NewSession 会话的 SaveJSON
// 会真实落盘。
func TestNewEphemeralSessionNoPersistenceFiles(t *testing.T) {
	dir := t.TempDir()
	sessDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}

	eph := NewEphemeralSession("eph-1", 4096)
	if !eph.IsEphemeral() {
		t.Fatal("NewEphemeralSession 构造的会话应标记为 ephemeral")
	}

	// 运行/追加消息（含带 Meta 与多模态内容的消息）
	eph.AddMessage("user", "hello", "")
	eph.AddMessageWithMeta("assistant", "hi", "", map[string]any{"k": "v"})
	eph.AddMessage("user", []ContentBlock{{Type: "text", Data: "block"}}, "")

	// 模拟上层挂接持久化：显式请求落盘也不应产生文件
	jsonlPath := filepath.Join(sessDir, "eph-1.jsonl")
	if err := eph.SaveJSON(jsonlPath); err != nil {
		t.Fatalf("ephemeral SaveJSON 应为 no-op 且不报错: %v", err)
	}
	if err := eph.SaveYAML(filepath.Join(sessDir, "eph-1.yaml")); err != nil {
		t.Fatalf("ephemeral SaveYAML 应为 no-op 且不报错: %v", err)
	}
	assertDirNoFiles(t, dir)

	// 消息确实保留在内存态会话中
	if got := len(eph.GetFullContext()); got != 3 {
		t.Fatalf("ephemeral 会话内存历史长度 = %d, want 3", got)
	}

	// 对比：普通 NewSession 会话同样的 SaveJSON 调用会真实落盘
	normal := NewSession("normal-1", 4096)
	normal.AddMessage("user", "hello", "")
	normalPath := filepath.Join(sessDir, "normal-1.jsonl")
	if err := normal.SaveJSON(normalPath); err != nil {
		t.Fatalf("普通会话 SaveJSON: %v", err)
	}
	if _, err := os.Stat(normalPath); err != nil {
		t.Fatalf("普通会话应产生持久化文件: %v", err)
	}

	// ephemeral 会话的 Fork 仍保持内存态（不落盘）
	fork := eph.Fork()
	if !fork.IsEphemeral() {
		t.Fatal("ephemeral 会话的 Fork 也应保持 ephemeral")
	}
	if err := fork.SaveJSON(filepath.Join(sessDir, "eph-fork.jsonl")); err != nil {
		t.Fatalf("ephemeral fork 的 SaveJSON 应为 no-op: %v", err)
	}

	// 除普通会话的对照文件外，不应有任何 ephemeral 产生的文件
	assertDirHasExactly(t, dir, []string{normalPath})
}

// assertDirNoFiles 断言 dir（递归）下没有任何文件。
func assertDirNoFiles(t *testing.T, dir string) {
	t.Helper()
	if files := walkFiles(t, dir); len(files) > 0 {
		t.Fatalf("目录下不应产生任何文件, got %v", files)
	}
}

// assertDirHasExactly 断言 dir（递归）下的文件集合恰好等于 want（忽略顺序）。
func assertDirHasExactly(t *testing.T, dir string, want []string) {
	t.Helper()
	got := walkFiles(t, dir)
	if len(got) != len(want) {
		t.Fatalf("目录文件集合 = %v, want %v", got, want)
	}
	wantSet := make(map[string]struct{}, len(want))
	for _, p := range want {
		wantSet[p] = struct{}{}
	}
	for _, p := range got {
		if _, ok := wantSet[p]; !ok {
			t.Fatalf("出现多余文件 %q, want %v", p, want)
		}
	}
}

// walkFiles 递归收集 dir 下的所有文件路径。
func walkFiles(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
