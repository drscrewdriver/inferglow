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

package flow

import (
	"reflect"
	"strings"
	"testing"
)

// sameHandler 比较两个 any 是否为同一 func 值（按指针）。
// 测试辅助函数，避免 func 值不能用 == 直接比较的问题。
func sameHandler(a, b any) bool {
	if a == nil || b == nil {
		return false
	}
	va := reflect.ValueOf(a)
	vb := reflect.ValueOf(b)
	if va.Kind() != reflect.Func || vb.Kind() != reflect.Func {
		return false
	}
	return va.Pointer() == vb.Pointer()
}

// ============================================================================
// HandlerRegistry 测试
// ============================================================================

// Check: NewHandlerRegistry 创建空注册表
func TestNewHandlerRegistry(t *testing.T) {
	r := NewHandlerRegistry()
	if r == nil {
		t.Fatal("NewHandlerRegistry returned nil")
	}
	if len(r.Names()) != 0 {
		t.Errorf("expected empty registry, got %d names", len(r.Names()))
	}
}

// Check: Register + Get 正常工作
func TestHandlerRegistryRegisterGet(t *testing.T) {
	r := NewHandlerRegistry()
	handler := func() {}
	if err := r.Register("h1", handler); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if got := r.Get("h1"); !sameHandler(got, handler) {
		t.Errorf("Get(h1) = %p, want %p", got, handler)
	}
}

// Check: Get 未找到返回 nil
func TestHandlerRegistryGetMissing(t *testing.T) {
	r := NewHandlerRegistry()
	if got := r.Get("nonexistent"); got != nil {
		t.Errorf("Get(nonexistent) = %v, want nil", got)
	}
}

// Check: Register 空 name 返回 error
func TestHandlerRegistryRegisterEmptyName(t *testing.T) {
	r := NewHandlerRegistry()
	err := r.Register("", func() {})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

// Check: Register nil handler 返回 error
func TestHandlerRegistryRegisterNil(t *testing.T) {
	r := NewHandlerRegistry()
	err := r.Register("h1", nil)
	if err == nil {
		t.Error("expected error for nil handler")
	}
}

// Check: Register 覆盖同名
func TestHandlerRegistryRegisterOverride(t *testing.T) {
	r := NewHandlerRegistry()
	h1 := func() {}
	h2 := func() {}
	if err := r.Register("h", h1); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}
	if err := r.Register("h", h2); err != nil {
		t.Fatalf("second Register failed: %v", err)
	}
	if got := r.Get("h"); !sameHandler(got, h2) {
		t.Error("override did not replace handler")
	}
}

// Check: Has 正确返回
func TestHandlerRegistryHas(t *testing.T) {
	r := NewHandlerRegistry()
	r.Register("h1", func() {})
	if !r.Has("h1") {
		t.Error("Has(h1) should be true")
	}
	if r.Has("h2") {
		t.Error("Has(h2) should be false")
	}
}

// Check: Unregister 正确移除
func TestHandlerRegistryUnregister(t *testing.T) {
	r := NewHandlerRegistry()
	r.Register("h1", func() {})
	if !r.Unregister("h1") {
		t.Error("Unregister(h1) should return true")
	}
	if r.Has("h1") {
		t.Error("Has(h1) should be false after Unregister")
	}
	if r.Unregister("h1") {
		t.Error("Unregister(h1) second time should return false")
	}
}

// Check: Names 返回所有名称
func TestHandlerRegistryNames(t *testing.T) {
	r := NewHandlerRegistry()
	r.Register("a", func() {})
	r.Register("b", func() {})
	r.Register("c", func() {})
	names := r.Names()
	if len(names) != 3 {
		t.Errorf("expected 3 names, got %d", len(names))
	}
}

// Check: nil registry 的方法安全返回
func TestHandlerRegistryNilSafe(t *testing.T) {
	var r *HandlerRegistry
	if r.Get("x") != nil {
		t.Error("nil.Get should return nil")
	}
	if r.Has("x") {
		t.Error("nil.Has should return false")
	}
	if r.Unregister("x") {
		t.Error("nil.Unregister should return false")
	}
	if r.Names() != nil {
		t.Error("nil.Names should return nil")
	}
}

// ============================================================================
// GlobalHandlerRegistry 测试
// ============================================================================

// Check: GlobalHandlerRegistry 返回非 nil
func TestGlobalHandlerRegistry(t *testing.T) {
	if GlobalHandlerRegistry() == nil {
		t.Fatal("GlobalHandlerRegistry returned nil")
	}
}

// Check: SetGlobalHandlerRegistry 替换全局注册表
func TestSetGlobalHandlerRegistry(t *testing.T) {
	original := GlobalHandlerRegistry()
	defer SetGlobalHandlerRegistry(original)

	newReg := NewHandlerRegistry()
	newReg.Register("test_handler", func() {})
	SetGlobalHandlerRegistry(newReg)

	if GlobalHandlerRegistry() != newReg {
		t.Error("global registry not replaced")
	}
	if !GlobalHandlerRegistry().Has("test_handler") {
		t.Error("test_handler should be in new registry")
	}
}

// Check: SetGlobalHandlerRegistry(nil) 重置为空注册表
func TestSetGlobalHandlerRegistryNil(t *testing.T) {
	original := GlobalHandlerRegistry()
	defer SetGlobalHandlerRegistry(original)

	GlobalHandlerRegistry().Register("temp", func() {})
	SetGlobalHandlerRegistry(nil)
	if GlobalHandlerRegistry().Has("temp") {
		t.Error("nil should reset to empty registry")
	}
}

// Check: RegisterGlobalHandler 注册到全局
func TestRegisterGlobalHandler(t *testing.T) {
	ResetGlobalHandlerRegistry()
	defer ResetGlobalHandlerRegistry()

	handler := func() {}
	if err := RegisterGlobalHandler("global_h", handler); err != nil {
		t.Fatalf("RegisterGlobalHandler failed: %v", err)
	}
	if !GlobalHandlerRegistry().Has("global_h") {
		t.Error("global_h should be registered")
	}
}

// ============================================================================
// CallableRef.ResolveHandler 测试
// ============================================================================

// Check 2.8: ResolveHandler "registered" 从 GlobalHandlerRegistry 查找
func TestCallableRefResolveRegistered(t *testing.T) {
	ResetGlobalHandlerRegistry()
	defer ResetGlobalHandlerRegistry()

	handler := func() {}
	RegisterGlobalHandler("reg_handler", handler)

	ref := &CallableRef{
		Kind: CallableRegistered,
		Name: "reg_handler",
	}
	resolved, err := ref.ResolveHandler()
	if err != nil {
		t.Fatalf("ResolveHandler failed: %v", err)
	}
	if !sameHandler(resolved, handler) {
		t.Errorf("resolved = %p, want %p", resolved, handler)
	}
}

// Check: ResolveHandler "registered" 未找到返回 error
func TestCallableRefResolveRegisteredMissing(t *testing.T) {
	ResetGlobalHandlerRegistry()
	defer ResetGlobalHandlerRegistry()

	ref := &CallableRef{
		Kind: CallableRegistered,
		Name: "nonexistent",
	}
	_, err := ref.ResolveHandler()
	if err == nil {
		t.Error("expected error for nonexistent handler")
	}
}

// Check 2.8: ResolveHandler "inspected" 通过 Qualname 在注册表查找
func TestCallableRefResolveInspected(t *testing.T) {
	ResetGlobalHandlerRegistry()
	defer ResetGlobalHandlerRegistry()

	handler := func() {}
	RegisterGlobalHandler("github.com/test/pkg.HandlerFunc", handler)

	ref := &CallableRef{
		Kind:     CallableInspected,
		Module:   "github.com/test/pkg",
		Qualname: "github.com/test/pkg.HandlerFunc",
	}
	resolved, err := ref.ResolveHandler()
	if err != nil {
		t.Fatalf("ResolveHandler failed: %v", err)
	}
	if !sameHandler(resolved, handler) {
		t.Errorf("resolved = %p, want %p", resolved, handler)
	}
}

// Check: ResolveHandler "inspected" 未注册返回 error
func TestCallableRefResolveInspectedMissing(t *testing.T) {
	ResetGlobalHandlerRegistry()
	defer ResetGlobalHandlerRegistry()

	ref := &CallableRef{
		Kind:     CallableInspected,
		Module:   "github.com/test/pkg",
		Qualname: "github.com/test/pkg.Nonexistent",
	}
	_, err := ref.ResolveHandler()
	if err == nil {
		t.Error("expected error for unresolvable inspected handler")
	}
}

// Check 2.8: ResolveHandler "anonymous" 返回 error
func TestCallableRefResolveAnonymous(t *testing.T) {
	ref := &CallableRef{
		Kind: CallableAnonymous,
	}
	_, err := ref.ResolveHandler()
	if err == nil {
		t.Error("expected error for anonymous handler")
	}
}

// Check: ResolveHandler 未知 Kind 返回 error
func TestCallableRefResolveUnknownKind(t *testing.T) {
	ref := &CallableRef{
		Kind: "bogus_kind",
	}
	_, err := ref.ResolveHandler()
	if err == nil {
		t.Error("expected error for unknown kind")
	}
}

// Check: ResolveHandler 对 nil ref 返回 error
func TestCallableRefResolveNil(t *testing.T) {
	var ref *CallableRef
	_, err := ref.ResolveHandler()
	if err == nil {
		t.Error("expected error for nil ref")
	}
}

// ============================================================================
// BuildCallableRef 测试
// ============================================================================

// Check 2.9: BuildCallableRef 从 handler 构建 CallableRef
func TestBuildCallableRef(t *testing.T) {
	ResetGlobalHandlerRegistry()
	defer ResetGlobalHandlerRegistry()

	// 测试用 handler
	testHandler := func(x int) int { return x * 2 }

	ref, err := BuildCallableRef(testHandler, "")
	if err != nil {
		t.Fatalf("BuildCallableRef failed: %v", err)
	}
	if ref == nil {
		t.Fatal("returned nil ref")
	}
	// 未注册的 handler 应该是 CallableInspected
	if ref.Kind != CallableInspected {
		t.Errorf("Kind = %q, want %q", ref.Kind, CallableInspected)
	}
	if ref.Qualname == "" {
		t.Error("Qualname should not be empty")
	}
	if ref.Line <= 0 {
		t.Errorf("Line = %d, should be > 0", ref.Line)
	}
}

// Check: BuildCallableRef 用 explicitName 设置 Name
func TestBuildCallableRefExplicitName(t *testing.T) {
	ResetGlobalHandlerRegistry()
	defer ResetGlobalHandlerRegistry()

	testHandler := func() {}
	ref, err := BuildCallableRef(testHandler, "my_handler")
	if err != nil {
		t.Fatalf("BuildCallableRef failed: %v", err)
	}
	if ref.Name != "my_handler" {
		t.Errorf("Name = %q, want my_handler", ref.Name)
	}
}

// Check: BuildCallableRef 对已注册的 handler 返回 CallableRegistered
func TestBuildCallableRefRegistered(t *testing.T) {
	ResetGlobalHandlerRegistry()
	defer ResetGlobalHandlerRegistry()

	testHandler := func() {}
	RegisterGlobalHandler("reg_h", testHandler)

	ref, err := BuildCallableRef(testHandler, "reg_h")
	if err != nil {
		t.Fatalf("BuildCallableRef failed: %v", err)
	}
	if ref.Kind != CallableRegistered {
		t.Errorf("Kind = %q, want %q", ref.Kind, CallableRegistered)
	}
}

// Check: BuildCallableRef 对 nil handler 返回 error
func TestBuildCallableRefNilHandler(t *testing.T) {
	_, err := BuildCallableRef(nil, "h")
	if err == nil {
		t.Error("expected error for nil handler")
	}
}

// Check: BuildCallableRef 对非 func 类型返回 error
func TestBuildCallableRefNonFunc(t *testing.T) {
	_, err := BuildCallableRef("not a func", "h")
	if err == nil {
		t.Error("expected error for non-func handler")
	}
}

// ============================================================================
// determineKind 测试
// ============================================================================

// Check: determineKind 对已注册 handler 返回 CallableRegistered
func TestDetermineKindRegistered(t *testing.T) {
	ResetGlobalHandlerRegistry()
	defer ResetGlobalHandlerRegistry()

	h := func() {}
	RegisterGlobalHandler("kind_test", h)
	if got := determineKind(h, "kind_test"); got != CallableRegistered {
		t.Errorf("determineKind = %q, want %q", got, CallableRegistered)
	}
}

// Check: determineKind 对未注册 handler 返回 CallableInspected
func TestDetermineKindInspected(t *testing.T) {
	ResetGlobalHandlerRegistry()
	defer ResetGlobalHandlerRegistry()

	h := func() {}
	if got := determineKind(h, "unregistered"); got != CallableInspected {
		t.Errorf("determineKind = %q, want %q", got, CallableInspected)
	}
}

// Check: determineKind 空 name 返回 CallableInspected
func TestDetermineKindEmptyName(t *testing.T) {
	h := func() {}
	if got := determineKind(h, ""); got != CallableInspected {
		t.Errorf("determineKind = %q, want %q", got, CallableInspected)
	}
}

// ============================================================================
// splitRuntimeFuncName 测试
// ============================================================================

// Check: splitRuntimeFuncName 正确解析
func TestSplitRuntimeFuncName(t *testing.T) {
	cases := []struct {
		fullName string
		pkgPath  string
		funcName string
	}{
		{"github.com/foo/bar.Baz", "github.com/foo/bar", "Baz"},
		{"github.com/foo/bar.(*Type).Method", "github.com/foo/bar", "(*Type).Method"},
		{"main.main", "main", "main"},
		{"", "", ""},
		{"NoDot", "", "NoDot"},
	}
	for _, c := range cases {
		t.Run(c.fullName, func(t *testing.T) {
			pkg, fn := splitRuntimeFuncName(c.fullName)
			if pkg != c.pkgPath {
				t.Errorf("pkgPath = %q, want %q", pkg, c.pkgPath)
			}
			if fn != c.funcName {
				t.Errorf("funcName = %q, want %q", fn, c.funcName)
			}
		})
	}
}

// ============================================================================
// 端到端：Build + Resolve 往返一致
// ============================================================================

// Check: Build → Resolve 往返（已注册）
func TestBuildAndResolveRoundTripRegistered(t *testing.T) {
	ResetGlobalHandlerRegistry()
	defer ResetGlobalHandlerRegistry()

	testHandler := func(x int) int { return x + 1 }
	RegisterGlobalHandler("rt_handler", testHandler)

	ref, err := BuildCallableRef(testHandler, "rt_handler")
	if err != nil {
		t.Fatalf("BuildCallableRef failed: %v", err)
	}
	resolved, err := ref.ResolveHandler()
	if err != nil {
		t.Fatalf("ResolveHandler failed: %v", err)
	}
	if !sameHandler(resolved, testHandler) {
		t.Error("resolved handler does not match original")
	}
}

// Check: Build → Resolve 往返（inspected）
func TestBuildAndResolveRoundTripInspected(t *testing.T) {
	ResetGlobalHandlerRegistry()
	defer ResetGlobalHandlerRegistry()

	testHandler := func(x int) int { return x + 1 }
	// 不注册 → inspected，但 inspected 需要在 ResolveHandler 时通过 Qualname 查找
	// 由于未注册，ResolveHandler 会失败。我们验证这个错误路径。
	ref, err := BuildCallableRef(testHandler, "")
	if err != nil {
		t.Fatalf("BuildCallableRef failed: %v", err)
	}
	if ref.Kind != CallableInspected {
		t.Errorf("Kind = %q, want %q", ref.Kind, CallableInspected)
	}
	_, err = ref.ResolveHandler()
	if err == nil {
		t.Error("expected error for unresolvable inspected handler")
	}
	if !strings.Contains(err.Error(), "not resolvable") {
		t.Errorf("error should mention 'not resolvable', got: %v", err)
	}
}
