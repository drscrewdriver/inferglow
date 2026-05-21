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

package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// newPopulatedStore 创建一个包含以下结构的 MemoryLineageStore：
//
//	root ── a ── b ── c
//	  └── d
//	(b 同时以 root 与 a 作为 parent)
func newPopulatedStore(t *testing.T) *MemoryLineageStore {
	t.Helper()
	s := NewMemoryLineageStore()
	nodes := []LineageNode{
		{Path: "root", Operation: "init", CreatedBy: "agent"},
		{Path: "a", Parents: []string{"root"}, Operation: "transform"},
		{Path: "b", Parents: []string{"root", "a"}, Operation: "merge"},
		{Path: "c", Parents: []string{"b"}, Operation: "transform"},
		{Path: "d", Parents: []string{"root"}, Operation: "copy"},
	}
	for _, n := range nodes {
		if err := s.Record(n); err != nil {
			t.Fatalf("Record %q: %v", n.Path, err)
		}
	}
	return s
}

// ============================================================================
// 构造与基本属性
// ============================================================================

func TestNewMemoryLineageStoreEmpty(t *testing.T) {
	s := NewMemoryLineageStore()
	if s == nil {
		t.Fatal("NewMemoryLineageStore returned nil")
	}
	if s.Size() != 0 {
		t.Errorf("Size = %d, want 0", s.Size())
	}
	if all := s.All(); len(all) != 0 {
		t.Errorf("All() = %v, want empty", all)
	}
}

// ============================================================================
// Record / Get
// ============================================================================

func TestRecordRejectsEmptyPath(t *testing.T) {
	s := NewMemoryLineageStore()
	err := s.Record(LineageNode{Path: ""})
	if !errors.Is(err, ErrEmptyLineagePath) {
		t.Errorf("expected ErrEmptyLineagePath, got %v", err)
	}
}

func TestRecordAutoFillsCreatedAt(t *testing.T) {
	s := NewMemoryLineageStore()
	before := time.Now().UTC().Add(-time.Millisecond)
	if err := s.Record(LineageNode{Path: "x"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	n, err := s.Get("x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n.CreatedAt.Before(before) {
		t.Errorf("CreatedAt = %v, want >= %v", n.CreatedAt, before)
	}
	if n.CreatedAt.IsZero() {
		t.Errorf("CreatedAt not filled")
	}
}

func TestRecordPreservesProvidedCreatedAt(t *testing.T) {
	s := NewMemoryLineageStore()
	ts := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := s.Record(LineageNode{Path: "x", CreatedAt: ts}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	n, err := s.Get("x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !n.CreatedAt.Equal(ts) {
		t.Errorf("CreatedAt = %v, want %v", n.CreatedAt, ts)
	}
}

func TestRecordUpdatesExisting(t *testing.T) {
	s := NewMemoryLineageStore()
	if err := s.Record(LineageNode{Path: "x", Operation: "v1"}); err != nil {
		t.Fatalf("Record v1: %v", err)
	}
	if err := s.Record(LineageNode{Path: "x", Operation: "v2"}); err != nil {
		t.Fatalf("Record v2: %v", err)
	}
	n, err := s.Get("x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n.Operation != "v2" {
		t.Errorf("Operation = %q, want %q", n.Operation, "v2")
	}
	if s.Size() != 1 {
		t.Errorf("Size = %d, want 1", s.Size())
	}
}

func TestRecordDedupsParents(t *testing.T) {
	s := NewMemoryLineageStore()
	if err := s.Record(LineageNode{Path: "p"}); err != nil {
		t.Fatalf("Record p: %v", err)
	}
	if err := s.Record(LineageNode{Path: "c", Parents: []string{"p", "p", "", "p"}}); err != nil {
		t.Fatalf("Record c: %v", err)
	}
	n, err := s.Get("c")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(n.Parents) != 1 || n.Parents[0] != "p" {
		t.Errorf("Parents = %v, want [p]", n.Parents)
	}
}

func TestRecordDefensiveCopies(t *testing.T) {
	s := NewMemoryLineageStore()
	original := LineageNode{
		Path:     "x",
		Parents:  []string{"p"},
		Metadata: map[string]any{"k": "v"},
	}
	if err := s.Record(original); err != nil {
		t.Fatalf("Record: %v", err)
	}
	// 修改 original，store 不应受影响。
	original.Parents[0] = "tampered"
	original.Metadata["k"] = "tampered"

	n, err := s.Get("x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n.Parents[0] != "p" {
		t.Errorf("Parents[0] = %q, want %q (防御性拷贝失败)", n.Parents[0], "p")
	}
	if n.Metadata["k"] != "v" {
		t.Errorf("Metadata[k] = %v, want %v (防御性拷贝失败)", n.Metadata["k"], "v")
	}

	// 修改 Get 返回的拷贝，store 不应受影响。
	n.Parents[0] = "tampered2"
	n.Metadata["k"] = "tampered2"
	n2, err := s.Get("x")
	if err != nil {
		t.Fatalf("Get second: %v", err)
	}
	if n2.Parents[0] != "p" {
		t.Errorf("Parents[0] = %q, want %q (Get 返回值应防御性拷贝)", n2.Parents[0], "p")
	}
	if n2.Metadata["k"] != "v" {
		t.Errorf("Metadata[k] = %v, want %v (Get 返回值应防御性拷贝)", n2.Metadata["k"], "v")
	}
}

func TestGetNotFound(t *testing.T) {
	s := NewMemoryLineageStore()
	_, err := s.Get("nope")
	if !errors.Is(err, ErrLineageNotFound) {
		t.Errorf("expected ErrLineageNotFound, got %v", err)
	}
}

// ============================================================================
// Parents / Children
// ============================================================================

func TestParentsAndChildren(t *testing.T) {
	s := newPopulatedStore(t)

	parents, err := s.Parents("b")
	if err != nil {
		t.Fatalf("Parents b: %v", err)
	}
	if len(parents) != 2 {
		t.Fatalf("Parents(b) = %v, want 2 entries", parents)
	}
	want := map[string]bool{"root": false, "a": false}
	for _, p := range parents {
		if _, ok := want[p]; !ok {
			t.Errorf("unexpected parent %q", p)
		}
		want[p] = true
	}
	for k, found := range want {
		if !found {
			t.Errorf("missing parent %q", k)
		}
	}

	children, err := s.Children("root")
	if err != nil {
		t.Fatalf("Children root: %v", err)
	}
	// root 的直接子节点：a, b, d
	wantChildren := map[string]bool{"a": false, "b": false, "d": false}
	for _, c := range children {
		if _, ok := wantChildren[c]; !ok {
			t.Errorf("unexpected child %q", c)
		}
		wantChildren[c] = true
	}
	for k, found := range wantChildren {
		if !found {
			t.Errorf("missing child %q of root (got %v)", k, children)
		}
	}
}

func TestParentsNoParentsReturnsEmpty(t *testing.T) {
	s := newPopulatedStore(t)
	p, err := s.Parents("root")
	if err != nil {
		t.Fatalf("Parents root: %v", err)
	}
	if len(p) != 0 {
		t.Errorf("Parents(root) = %v, want empty", p)
	}
}

func TestParentsNotFound(t *testing.T) {
	s := NewMemoryLineageStore()
	_, err := s.Parents("nope")
	if !errors.Is(err, ErrLineageNotFound) {
		t.Errorf("expected ErrLineageNotFound, got %v", err)
	}
}

func TestChildrenNotFound(t *testing.T) {
	s := NewMemoryLineageStore()
	_, err := s.Children("nope")
	if !errors.Is(err, ErrLineageNotFound) {
		t.Errorf("expected ErrLineageNotFound, got %v", err)
	}
}

func TestChildrenNoChildrenReturnsEmpty(t *testing.T) {
	s := newPopulatedStore(t)
	c, err := s.Children("c")
	if err != nil {
		t.Fatalf("Children c: %v", err)
	}
	if len(c) != 0 {
		t.Errorf("Children(c) = %v, want empty", c)
	}
}

// ============================================================================
// Ancestors / Descendants
// ============================================================================

func TestAncestorsBFSOrder(t *testing.T) {
	s := newPopulatedStore(t)
	// c 的祖先链：b -> {root, a}
	anc, err := s.Ancestors("c")
	if err != nil {
		t.Fatalf("Ancestors c: %v", err)
	}
	if len(anc) != 3 {
		t.Fatalf("Ancestors(c) = %v, want 3 entries", anc)
	}
	if anc[0] != "b" {
		t.Errorf("first ancestor = %q, want %q", anc[0], "b")
	}
	want := map[string]bool{"root": false, "a": false, "b": false}
	for _, p := range anc {
		want[p] = true
	}
	for k, found := range want {
		if !found {
			t.Errorf("missing ancestor %q (got %v)", k, anc)
		}
	}
}

func TestAncestorsRootReturnsEmpty(t *testing.T) {
	s := newPopulatedStore(t)
	anc, err := s.Ancestors("root")
	if err != nil {
		t.Fatalf("Ancestors root: %v", err)
	}
	if len(anc) != 0 {
		t.Errorf("Ancestors(root) = %v, want empty", anc)
	}
}

func TestDescendantsBFSOrder(t *testing.T) {
	s := newPopulatedStore(t)
	// root 的后代：a, b, d（第一层）+ c（第二层，从 b）
	desc, err := s.Descendants("root")
	if err != nil {
		t.Fatalf("Descendants root: %v", err)
	}
	if len(desc) != 4 {
		t.Fatalf("Descendants(root) = %v, want 4 entries", desc)
	}
	want := map[string]bool{"a": false, "b": false, "c": false, "d": false}
	for _, p := range desc {
		want[p] = true
	}
	for k, found := range want {
		if !found {
			t.Errorf("missing descendant %q (got %v)", k, desc)
		}
	}
}

func TestDescendantsLeafReturnsEmpty(t *testing.T) {
	s := newPopulatedStore(t)
	desc, err := s.Descendants("c")
	if err != nil {
		t.Fatalf("Descendants c: %v", err)
	}
	if len(desc) != 0 {
		t.Errorf("Descendants(c) = %v, want empty", desc)
	}
}

func TestAncestorsNotFound(t *testing.T) {
	s := NewMemoryLineageStore()
	_, err := s.Ancestors("nope")
	if !errors.Is(err, ErrLineageNotFound) {
		t.Errorf("expected ErrLineageNotFound, got %v", err)
	}
}

func TestDescendantsNotFound(t *testing.T) {
	s := NewMemoryLineageStore()
	_, err := s.Descendants("nope")
	if !errors.Is(err, ErrLineageNotFound) {
		t.Errorf("expected ErrLineageNotFound, got %v", err)
	}
}

// ============================================================================
// 环检测
// ============================================================================

func TestRecordRejectsSelfParent(t *testing.T) {
	s := NewMemoryLineageStore()
	err := s.Record(LineageNode{Path: "x", Parents: []string{"x"}})
	if !errors.Is(err, ErrLineageCycle) {
		t.Errorf("expected ErrLineageCycle, got %v", err)
	}
}

func TestRecordRejectsIndirectCycle(t *testing.T) {
	s := NewMemoryLineageStore()
	if err := s.Record(LineageNode{Path: "a", Parents: []string{"b"}}); err != nil {
		t.Fatalf("Record a: %v", err)
	}
	if err := s.Record(LineageNode{Path: "b", Parents: []string{"a"}}); err == nil {
		t.Errorf("Record b should fail with cycle, got nil")
	} else if !errors.Is(err, ErrLineageCycle) {
		t.Errorf("expected ErrLineageCycle, got %v", err)
	}
}

func TestRecordRejectsThreeNodeCycle(t *testing.T) {
	s := NewMemoryLineageStore()
	if err := s.Record(LineageNode{Path: "a", Parents: []string{"b"}}); err != nil {
		t.Fatalf("Record a: %v", err)
	}
	if err := s.Record(LineageNode{Path: "b", Parents: []string{"c"}}); err != nil {
		t.Fatalf("Record b: %v", err)
	}
	err := s.Record(LineageNode{Path: "c", Parents: []string{"a"}})
	if !errors.Is(err, ErrLineageCycle) {
		t.Errorf("expected ErrLineageCycle for 3-node cycle, got %v", err)
	}
}

func TestAncestorsHandlesCorruptedCycleGracefully(t *testing.T) {
	// 模拟数据被外部直接写入形成的环（绕过 Record 的环检测）。
	s := NewMemoryLineageStore()
	s.nodes["x"] = &LineageNode{Path: "x", Parents: []string{"y"}}
	s.nodes["y"] = &LineageNode{Path: "y", Parents: []string{"x"}}

	// Ancestors 不应死循环；visited 守卫防止再次访问 x。
	// x 的直接祖先是 y，y 的祖先是 x（已访问，跳过）。
	// 因此 Ancestors(x) = [y]（len=1，不含自身）。
	anc, err := s.Ancestors("x")
	if err != nil {
		t.Fatalf("Ancestors: %v", err)
	}
	if len(anc) != 1 || anc[0] != "y" {
		t.Errorf("Ancestors(x) = %v, want [y]", anc)
	}
	// 反方向同理：Ancestors(y) = [x]。
	anc, err = s.Ancestors("y")
	if err != nil {
		t.Fatalf("Ancestors y: %v", err)
	}
	if len(anc) != 1 || anc[0] != "x" {
		t.Errorf("Ancestors(y) = %v, want [x]", anc)
	}
}

func TestDescendantsHandlesCorruptedCycleGracefully(t *testing.T) {
	s := NewMemoryLineageStore()
	s.nodes["x"] = &LineageNode{Path: "x", Parents: []string{"y"}}
	s.nodes["y"] = &LineageNode{Path: "y", Parents: []string{"x"}}

	// Descendants(x)：y 是 x 的子（因为 y.Parents 含 x），x 又是 y 的子但已访问。
	// 因此 Descendants(x) = [y]。
	desc, err := s.Descendants("x")
	if err != nil {
		t.Fatalf("Descendants: %v", err)
	}
	if len(desc) != 1 || desc[0] != "y" {
		t.Errorf("Descendants(x) = %v, want [y]", desc)
	}
	// 反方向：Descendants(y) = [x]。
	desc, err = s.Descendants("y")
	if err != nil {
		t.Fatalf("Descendants y: %v", err)
	}
	if len(desc) != 1 || desc[0] != "x" {
		t.Errorf("Descendants(y) = %v, want [x]", desc)
	}
}

// ============================================================================
// All / Remove / Size
// ============================================================================

func TestAllReturnsSortedCopy(t *testing.T) {
	s := newPopulatedStore(t)
	all := s.All()
	if len(all) != 5 {
		t.Fatalf("len(All) = %d, want 5", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].Path > all[i].Path {
			t.Errorf("All() not sorted: %q > %q", all[i-1].Path, all[i].Path)
		}
	}
	// 验证是拷贝。
	all[0].Operation = "tampered"
	n, err := s.Get(all[0].Path)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n.Operation == "tampered" {
		t.Errorf("All() returned reference, not copy")
	}
}

func TestRemove(t *testing.T) {
	s := newPopulatedStore(t)
	before := s.Size()
	if err := s.Remove("c"); err != nil {
		t.Fatalf("Remove c: %v", err)
	}
	if s.Size() != before-1 {
		t.Errorf("Size after Remove = %d, want %d", s.Size(), before-1)
	}
	if _, err := s.Get("c"); !errors.Is(err, ErrLineageNotFound) {
		t.Errorf("Get after Remove: expected ErrLineageNotFound, got %v", err)
	}
}

func TestRemovePreservesReferencesInOtherNodes(t *testing.T) {
	s := newPopulatedStore(t)
	// 删除 a，但 b 的 Parents 仍应包含 "a"（审计链保留）。
	if err := s.Remove("a"); err != nil {
		t.Fatalf("Remove a: %v", err)
	}
	b, err := s.Get("b")
	if err != nil {
		t.Fatalf("Get b: %v", err)
	}
	found := false
	for _, p := range b.Parents {
		if p == "a" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("b.Parents = %v, should still contain 'a' (审计链保留)", b.Parents)
	}
}

func TestRemoveNotFound(t *testing.T) {
	s := NewMemoryLineageStore()
	err := s.Remove("nope")
	if !errors.Is(err, ErrLineageNotFound) {
		t.Errorf("expected ErrLineageNotFound, got %v", err)
	}
}

// ============================================================================
// 并发测试
// ============================================================================

func TestConcurrentRecordAndRead(t *testing.T) {
	s := NewMemoryLineageStore()
	const N = 100
	var wg sync.WaitGroup

	// 先写入 root（同步完成，避免下面 child 写入时 root 不存在的环检测歧义）。
	if err := s.Record(LineageNode{Path: "root"}); err != nil {
		t.Fatalf("Record root: %v", err)
	}

	// 并发写入：每个 goroutine 写一个唯一 child 节点，parent 都是 root。
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			path := fmt.Sprintf("child-%d", i)
			if err := s.Record(LineageNode{
				Path:      path,
				Parents:   []string{"root"},
				Operation: "gen",
			}); err != nil {
				t.Errorf("Record %s: %v", path, err)
			}
		}(i)
	}

	// 并发读。
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.Ancestors("root")
			_, _ = s.Descendants("root")
			_ = s.All()
			_ = s.Size()
		}()
	}

	wg.Wait()

	if s.Size() != N+1 {
		t.Errorf("Size = %d, want %d", s.Size(), N+1)
	}
	desc, err := s.Descendants("root")
	if err != nil {
		t.Fatalf("Descendants: %v", err)
	}
	if len(desc) != N {
		t.Errorf("Descendants(root) = %d entries, want %d", len(desc), N)
	}
}

// ============================================================================
// JSON 持久化
// ============================================================================

func TestSaveLoadRoundTrip(t *testing.T) {
	s := newPopulatedStore(t)
	path := filepath.Join(t.TempDir(), "lineage.json")
	if err := SaveLineageToFile(s, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadLineageFromFile(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Size() != s.Size() {
		t.Errorf("loaded.Size = %d, want %d", loaded.Size(), s.Size())
	}

	// 验证每个节点内容一致。
	orig := s.All()
	for _, n := range orig {
		got, err := loaded.Get(n.Path)
		if err != nil {
			t.Errorf("loaded.Get(%q): %v", n.Path, err)
			continue
		}
		if got.Operation != n.Operation {
			t.Errorf("Operation mismatch for %q: %q vs %q", n.Path, got.Operation, n.Operation)
		}
		if !got.CreatedAt.Equal(n.CreatedAt) {
			t.Errorf("CreatedAt mismatch for %q: %v vs %v", n.Path, got.CreatedAt, n.CreatedAt)
		}
		if len(got.Parents) != len(n.Parents) {
			t.Errorf("Parents len mismatch for %q: %d vs %d", n.Path, len(got.Parents), len(n.Parents))
			continue
		}
		for i, p := range n.Parents {
			if got.Parents[i] != p {
				t.Errorf("Parents[%d] mismatch for %q: %q vs %q", i, n.Path, got.Parents[i], p)
			}
		}
	}
}

func TestSaveWithMetadata(t *testing.T) {
	s := NewMemoryLineageStore()
	if err := s.Record(LineageNode{
		Path:     "x",
		Metadata: map[string]any{"k1": "v1", "k2": float64(42)},
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	path := filepath.Join(t.TempDir(), "lineage.json")
	if err := SaveLineageToFile(s, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadLineageFromFile(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	n, err := loaded.Get("x")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v, ok := n.Metadata["k1"].(string); !ok || v != "v1" {
		t.Errorf("Metadata[k1] = %v, want \"v1\"", n.Metadata["k1"])
	}
	// JSON 反序列化数字为 float64。
	if v, ok := n.Metadata["k2"].(float64); !ok || v != 42 {
		t.Errorf("Metadata[k2] = %v, want 42", n.Metadata["k2"])
	}
}

func TestSaveRejectsUnsupportedStoreType(t *testing.T) {
	var fake fakeStore
	err := SaveLineageToFile(&fake, filepath.Join(t.TempDir(), "x.json"))
	if err == nil {
		t.Errorf("expected error for unsupported type")
	}
}

func TestSaveRejectsNilStore(t *testing.T) {
	err := SaveLineageToFile(nil, filepath.Join(t.TempDir(), "x.json"))
	if err == nil {
		t.Errorf("expected error for nil store")
	}
}

func TestLoadFileNotExist(t *testing.T) {
	_, err := LoadLineageFromFile(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil {
		t.Errorf("expected error for missing file")
	}
}

func TestLoadRejectsUnknownVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lineage.json")
	if err := os.WriteFile(path, []byte(`{"version":999,"nodes":[]}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := LoadLineageFromFile(path)
	if err == nil {
		t.Errorf("expected error for unknown version")
	}
}

// fakeStore 仅用于测试 SaveLineageToFile 拒绝非 *MemoryLineageStore 类型。
type fakeStore struct{}

func (f *fakeStore) Record(LineageNode) error          { return nil }
func (f *fakeStore) Get(string) (*LineageNode, error)  { return nil, nil }
func (f *fakeStore) Parents(string) ([]string, error)  { return nil, nil }
func (f *fakeStore) Children(string) ([]string, error) { return nil, nil }
func (f *fakeStore) Ancestors(string) ([]string, error) {
	return nil, nil
}
func (f *fakeStore) Descendants(string) ([]string, error) {
	return nil, nil
}
func (f *fakeStore) All() []LineageNode  { return nil }
func (f *fakeStore) Remove(string) error { return nil }
func (f *fakeStore) Size() int           { return 0 }

var _ LineageStore = (*fakeStore)(nil)
