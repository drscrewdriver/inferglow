package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ============================================================================
// 静态值源测试
// ============================================================================

func TestStaticValueSourceImplementsInputSource(t *testing.T) {
	var _ InputSource = (*StaticValueSource)(nil)
}

func TestStaticValueSourceResolve(t *testing.T) {
	src := NewStaticValueSource("static", map[string]any{"a": 1, "b": "x"})
	out, err := src.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if len(out) != 2 || out["a"] != 1 || out["b"] != "x" {
		t.Errorf("out = %v", out)
	}
}

func TestStaticValueSourceReturnsCopy(t *testing.T) {
	original := map[string]any{"k": "v"}
	src := NewStaticValueSource("static", original)
	out1, _ := src.Resolve(context.Background())
	out1["k"] = "modified"
	out2, _ := src.Resolve(context.Background())
	if out2["k"] != "v" {
		t.Errorf("internal state mutated: %v", out2["k"])
	}
	// 修改原 map 不应影响 source（构造时已拷贝）。
	original["k"] = "external"
	out3, _ := src.Resolve(context.Background())
	if out3["k"] != "v" {
		t.Errorf("external mutation affected source: %v", out3["k"])
	}
}

func TestStaticValueSourceNilReceiver(t *testing.T) {
	var src *StaticValueSource
	_, err := src.Resolve(context.Background())
	if err == nil {
		t.Error("expected error for nil receiver")
	}
}

// ============================================================================
// 环境变量源测试
// ============================================================================

func TestEnvSourceImplementsInputSource(t *testing.T) {
	var _ InputSource = (*EnvSource)(nil)
}

func TestEnvSourceExplicitKeys(t *testing.T) {
	os.Setenv("INPUTSOURCE_TEST_A", "1")
	os.Setenv("INPUTSOURCE_TEST_B", "two")
	defer os.Unsetenv("INPUTSOURCE_TEST_A")
	defer os.Unsetenv("INPUTSOURCE_TEST_B")

	src := NewEnvSource("env", "INPUTSOURCE_TEST_A", "INPUTSOURCE_TEST_B", "INPUTSOURCE_TEST_MISSING")
	out, err := src.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if out["INPUTSOURCE_TEST_A"] != "1" {
		t.Errorf("A = %v", out["INPUTSOURCE_TEST_A"])
	}
	if out["INPUTSOURCE_TEST_B"] != "two" {
		t.Errorf("B = %v", out["INPUTSOURCE_TEST_B"])
	}
	if _, ok := out["INPUTSOURCE_TEST_MISSING"]; ok {
		t.Error("missing key should not be present")
	}
}

func TestEnvSourceRequiredMissing(t *testing.T) {
	src := NewEnvSource("env", "INPUTSOURCE_REQUIRED_MISSING").
		WithRequired("INPUTSOURCE_REQUIRED_MISSING")
	_, err := src.Resolve(context.Background())
	if err == nil {
		t.Error("expected error for missing required env")
	}
	if !strings.Contains(err.Error(), "INPUTSOURCE_REQUIRED_MISSING") {
		t.Errorf("error should mention key, got: %v", err)
	}
}

func TestEnvSourceRequiredPresent(t *testing.T) {
	os.Setenv("INPUTSOURCE_REQUIRED_OK", "yes")
	defer os.Unsetenv("INPUTSOURCE_REQUIRED_OK")
	src := NewEnvSource("env").
		WithRequired("INPUTSOURCE_REQUIRED_OK")
	out, err := src.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if out["INPUTSOURCE_REQUIRED_OK"] != "yes" {
		t.Errorf("got %v", out["INPUTSOURCE_REQUIRED_OK"])
	}
}

func TestEnvSourceWithPrefix(t *testing.T) {
	os.Setenv("MYAPP_KEY1", "v1")
	os.Setenv("MYAPP_KEY2", "v2")
	os.Setenv("OTHER_KEY", "ignore")
	defer os.Unsetenv("MYAPP_KEY1")
	defer os.Unsetenv("MYAPP_KEY2")
	defer os.Unsetenv("OTHER_KEY")

	src := NewEnvSource("env").WithPrefix("MYAPP_")
	out, err := src.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if out["KEY1"] != "v1" {
		t.Errorf("KEY1 = %v", out["KEY1"])
	}
	if out["KEY2"] != "v2" {
		t.Errorf("KEY2 = %v", out["KEY2"])
	}
	if _, ok := out["OTHER_KEY"]; ok {
		t.Error("OTHER_KEY should not be present")
	}
}

// ============================================================================
// 会话源测试
// ============================================================================

func TestSessionSourceImplementsInputSource(t *testing.T) {
	var _ InputSource = (*SessionSource)(nil)
}

func TestInMemorySessionStore(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()
	// 不存在的 session 返回空 map。
	out, err := store.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %v", out)
	}
	// 设置后能读出。
	if err := store.Set(ctx, "sess1", "k1", "v1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Set(ctx, "sess1", "k2", 42); err != nil {
		t.Fatalf("Set: %v", err)
	}
	out, _ = store.Get(ctx, "sess1")
	if out["k1"] != "v1" || out["k2"] != 42 {
		t.Errorf("got %v", out)
	}
	// 删除。
	if err := store.Delete(ctx, "sess1", "k1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	out, _ = store.Get(ctx, "sess1")
	if _, ok := out["k1"]; ok {
		t.Error("k1 should be deleted")
	}
	if out["k2"] != 42 {
		t.Errorf("k2 should still exist: %v", out)
	}
}

func TestSessionSourceResolve(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()
	_ = store.Set(ctx, "s1", "user", "alice")
	_ = store.Set(ctx, "s1", "count", 3)

	src := NewSessionSource("session", store, "s1")
	out, err := src.Resolve(ctx)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if out["user"] != "alice" || out["count"] != 3 {
		t.Errorf("got %v", out)
	}
}

func TestSessionSourceEmptyID(t *testing.T) {
	store := NewInMemorySessionStore()
	src := NewSessionSource("session", store, "")
	_, err := src.Resolve(context.Background())
	if err == nil {
		t.Error("expected error for empty session id")
	}
}

func TestSessionSourceNilStore(t *testing.T) {
	src := NewSessionSource("session", nil, "s1")
	_, err := src.Resolve(context.Background())
	if err == nil {
		t.Error("expected error for nil store")
	}
}

func TestInMemorySessionStoreConcurrent(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()
	var wg sync.WaitGroup
	// 并发写同一 session 不同 key。
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = store.Set(ctx, "concurrent", fmt.Sprintf("k%d", i), i)
		}(i)
	}
	wg.Wait()
	out, _ := store.Get(ctx, "concurrent")
	if len(out) != 50 {
		t.Errorf("got %d keys, want 50", len(out))
	}
}

// ============================================================================
// HTTP 源测试
// ============================================================================

func TestHTTPSourceImplementsInputSource(t *testing.T) {
	var _ InputSource = (*HTTPSource)(nil)
}

func TestHTTPSourceResolve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"alice","count":42}`))
	}))
	defer srv.Close()

	src := NewHTTPSource("http", srv.URL).WithTimeout(5 * time.Second)
	out, err := src.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if out["name"] != "alice" {
		t.Errorf("name = %v", out["name"])
	}
	if out["count"] != float64(42) {
		t.Errorf("count = %v (type %T)", out["count"], out["count"])
	}
}

func TestHTTPSourceEmptyURL(t *testing.T) {
	src := NewHTTPSource("http", "")
	_, err := src.Resolve(context.Background())
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestHTTPSourceNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	src := NewHTTPSource("http", srv.URL).WithTimeout(5 * time.Second)
	_, err := src.Resolve(context.Background())
	if err == nil {
		t.Error("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "status 500") {
		t.Errorf("error should mention status, got: %v", err)
	}
}

func TestHTTPSourceInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	src := NewHTTPSource("http", srv.URL).WithTimeout(5 * time.Second)
	_, err := src.Resolve(context.Background())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestHTTPSourceWithHeaders(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	src := NewHTTPSource("http", srv.URL).
		WithTimeout(5 * time.Second).
		WithHeader("Authorization", "Bearer test-token")
	_, _ = src.Resolve(context.Background())
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
}

func TestHTTPSourceJSONNonObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[1,2,3]`))
	}))
	defer srv.Close()

	src := NewHTTPSource("http", srv.URL).WithTimeout(5 * time.Second)
	_, err := src.Resolve(context.Background())
	if err == nil {
		t.Fatal("expected error for non-object JSON")
	}
	if !strings.Contains(err.Error(), "decode json") {
		t.Errorf("error should mention decode, got: %v", err)
	}
}

// ============================================================================
// MultiSource 测试
// ============================================================================

func TestMultiSourceImplementsInputSource(t *testing.T) {
	var _ InputSource = (*MultiSource)(nil)
}

func TestMultiSourceEmpty(t *testing.T) {
	m := NewMultiSource("multi")
	out, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %v", out)
	}
}

func TestMultiSourceNilFiltered(t *testing.T) {
	m := NewMultiSource("multi", nil, nil, nil)
	if len(m.Sources()) != 0 {
		t.Errorf("nil sources should be filtered")
	}
}

func TestMultiSourceLastWins(t *testing.T) {
	a := NewStaticValueSource("a", map[string]any{"k": "from-a", "only-a": 1})
	b := NewStaticValueSource("b", map[string]any{"k": "from-b", "only-b": 2})
	m := NewMultiSource("multi", a, b).WithStrategy(MergeLastWins)
	out, err := m.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if out["k"] != "from-b" {
		t.Errorf("k = %v, want from-b", out["k"])
	}
	if out["only-a"] != 1 {
		t.Errorf("only-a = %v", out["only-a"])
	}
	if out["only-b"] != 2 {
		t.Errorf("only-b = %v", out["only-b"])
	}
}

func TestMultiSourceFirstWins(t *testing.T) {
	a := NewStaticValueSource("a", map[string]any{"k": "from-a"})
	b := NewStaticValueSource("b", map[string]any{"k": "from-b"})
	m := NewMultiSource("multi", a, b).WithStrategy(MergeFirstWins)
	out, _ := m.Resolve(context.Background())
	if out["k"] != "from-a" {
		t.Errorf("k = %v, want from-a", out["k"])
	}
}

func TestMultiSourceError(t *testing.T) {
	a := NewStaticValueSource("a", map[string]any{"k": "from-a"})
	b := NewStaticValueSource("b", map[string]any{"k": "from-b"})
	m := NewMultiSource("multi", a, b).WithStrategy(MergeError)
	_, err := m.Resolve(context.Background())
	if err == nil {
		t.Error("expected conflict error")
	}
}

func TestMultiSourcePartialError(t *testing.T) {
	good := NewStaticValueSource("good", map[string]any{"k1": "v1"})
	bad := &errorSource{name: "bad", err: errors.New("boom")}
	m := NewMultiSource("multi", good, bad)
	out, err := m.Resolve(context.Background())
	if err == nil {
		t.Error("expected error from bad source")
	}
	if out["k1"] != "v1" {
		t.Errorf("good source values should be present: %v", out)
	}
}

func TestMultiSourceFailFast(t *testing.T) {
	// 慢源 + 立即失败的源。
	slow := &delayedSource{name: "slow", delay: 200 * time.Millisecond, value: map[string]any{"slow": 1}}
	bad := &errorSource{name: "bad", err: errors.New("immediate")}
	m := NewMultiSource("multi", slow, bad).WithFailFast(true)
	start := time.Now()
	_, err := m.Resolve(context.Background())
	elapsed := time.Since(start)
	if err == nil {
		t.Error("expected error")
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("failFast should return quickly, took %v", elapsed)
	}
}

func TestMultiSourceFailFastCancelsOthers(t *testing.T) {
	slow := &delayedSource{name: "slow", delay: 500 * time.Millisecond, value: map[string]any{"slow": 1}}
	bad := &errorSource{name: "bad", err: errors.New("now")}
	m := NewMultiSource("multi", slow, bad).WithFailFast(true)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := m.Resolve(ctx)
	if err == nil {
		t.Error("expected error")
	}
}

func TestMultiSourceConcurrent(t *testing.T) {
	// 并发执行 Resolve，验证无 race。
	var counter atomic.Int32
	srcs := make([]InputSource, 0, 10)
	for i := 0; i < 10; i++ {
		srcs = append(srcs, &countingSource{name: fmt.Sprintf("s%d", i), counter: &counter})
	}
	m := NewMultiSource("multi", srcs...)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = m.Resolve(context.Background())
		}()
	}
	wg.Wait()
}

// ============================================================================
// ResolveAll 便捷函数
// ============================================================================

func TestResolveAll(t *testing.T) {
	a := NewStaticValueSource("a", map[string]any{"a": 1})
	b := NewStaticValueSource("b", map[string]any{"b": 2})
	out, err := ResolveAll(context.Background(), a, b)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if out["a"] != 1 || out["b"] != 2 {
		t.Errorf("got %v", out)
	}
}

// ============================================================================
// 辅助 mock 类型
// ============================================================================

type errorSource struct {
	name string
	err  error
}

func (e *errorSource) Name() string                                          { return e.name }
func (e *errorSource) Resolve(ctx context.Context) (map[string]any, error) { return nil, e.err }

type delayedSource struct {
	name  string
	delay time.Duration
	value map[string]any
}

func (d *delayedSource) Name() string { return d.name }
func (d *delayedSource) Resolve(ctx context.Context) (map[string]any, error) {
	select {
	case <-time.After(d.delay):
		return d.value, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type countingSource struct {
	name    string
	counter *atomic.Int32
}

func (c *countingSource) Name() string { return c.name }
func (c *countingSource) Resolve(ctx context.Context) (map[string]any, error) {
	c.counter.Add(1)
	return map[string]any{c.name: c.counter.Load()}, nil
}

// 编译期断言
var _ InputSource = (*errorSource)(nil)
var _ InputSource = (*delayedSource)(nil)
var _ InputSource = (*countingSource)(nil)

// 确保 json 包被使用（在 HTTPSource 实现内已使用，此断言避免某些版本编译器警告）。
var _ = json.Decoder{}
