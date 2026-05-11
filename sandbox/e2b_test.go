package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// ============================================================================
// Provider 基础测试
// ============================================================================

func TestE2BProviderNameKind(t *testing.T) {
	p := NewE2BProvider(map[string]any{"api_key": "test"}, nil)
	if p.Name() != "e2b" {
		t.Errorf("Name() = %q, want e2b", p.Name())
	}
	if p.Kind() != "remote" {
		t.Errorf("Kind() = %q, want remote", p.Kind())
	}
}

func TestE2BProviderImplementsProvider(t *testing.T) {
	var _ Provider = (*E2BProvider)(nil)
}

func TestE2BHandleImplementsHandle(t *testing.T) {
	var _ Handle = (*E2BHandle)(nil)
}

func TestE2BProviderNoAPIKey(t *testing.T) {
	// 清空 env 以确保不被外部环境污染。
	t.Setenv("E2B_API_KEY", "")
	p := NewE2BProvider(nil, nil)
	avail, err := p.InspectAvailability()
	if err != nil {
		t.Fatalf("InspectAvailability error: %v", err)
	}
	if avail.Available {
		t.Error("expected Available=false without API key")
	}
	if !strings.Contains(avail.ErrorMessage, "E2B_API_KEY") {
		t.Errorf("ErrorMessage = %q", avail.ErrorMessage)
	}
}

func TestE2BProviderWithAPIKeyFromConfig(t *testing.T) {
	t.Setenv("E2B_API_KEY", "")
	p := NewE2BProvider(map[string]any{"api_key": "from-cfg"}, nil)
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Error("expected Available=true with cfg api_key")
	}
}

func TestE2BProviderWithAPIKeyFromEnv(t *testing.T) {
	t.Setenv("E2B_API_KEY", "from-env")
	p := NewE2BProvider(nil, nil)
	avail, _ := p.InspectAvailability()
	if !avail.Available {
		t.Error("expected Available=true with env API key")
	}
}

func TestE2BProviderCreateHandleNoKey(t *testing.T) {
	t.Setenv("E2B_API_KEY", "")
	p := NewE2BProvider(nil, nil)
	policy := DefaultPolicy()
	_, err := p.CreateHandle(nil, &policy)
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable, got %v", err)
	}
}

// ============================================================================
// Config 解析测试
// ============================================================================

func TestParseE2BConfigDefaults(t *testing.T) {
	cfg := parseE2BConfig(nil)
	if cfg.BaseURL != e2bDefaultBaseURL {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
}

func TestParseE2BConfigFull(t *testing.T) {
	cfg := parseE2BConfig(map[string]any{
		"api_key":         "secret",
		"base_url":        "https://custom.example.com",
		"template_id":     "my-template",
		"timeout_seconds": 10,
		"memory_mb":       1024,
		"cpus":            2,
		"env":             map[string]any{"FOO": "bar"},
		"metadata":        map[string]any{"team": "dev"},
	})
	if cfg.APIKey != "secret" {
		t.Errorf("APIKey = %q", cfg.APIKey)
	}
	if cfg.BaseURL != "https://custom.example.com" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.TemplateID != "my-template" {
		t.Errorf("TemplateID = %q", cfg.TemplateID)
	}
	if cfg.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v", cfg.Timeout)
	}
	if cfg.MemoryMB != 1024 {
		t.Errorf("MemoryMB = %d", cfg.MemoryMB)
	}
	if cfg.CPUs != 2 {
		t.Errorf("CPUs = %d", cfg.CPUs)
	}
	if cfg.EnvVars["FOO"] != "bar" {
		t.Errorf("EnvVars = %v", cfg.EnvVars)
	}
	if cfg.Metadata["team"] != "dev" {
		t.Errorf("Metadata = %v", cfg.Metadata)
	}
}

func TestParseStringMapStringType(t *testing.T) {
	out := parseStringMap(map[string]string{"a": "b"})
	if out["a"] != "b" {
		t.Errorf("got %v", out)
	}
}

func TestParseStringMapUnknownType(t *testing.T) {
	out := parseStringMap(42)
	if out != nil {
		t.Errorf("expected nil, got %v", out)
	}
}

// ============================================================================
// 集成测试（使用 mock HTTP server 模拟 E2B API）
// ============================================================================

// newMockE2BServer 创建一个模拟 E2B API 的 httptest.Server。
// 返回 server 和一个收到的请求记录切片（线程安全）。
func newMockE2BServer(t *testing.T, sandboxID string, execResult *struct {
	ExitCode int
	Stdout   string
	Stderr   string
}) (*httptest.Server, *[]map[string]any) {
	var mu sync.Mutex
	var requests []map[string]any

	mux := http.NewServeMux()

	// POST /sandboxes
	mux.HandleFunc("/sandboxes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		requests = append(requests, body)
		mu.Unlock()

		// 校验 Authorization header。
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"sandboxID":%q}`, sandboxID)
	})

	// POST /sandboxes/{id}/commands
	mux.HandleFunc("/sandboxes/"+sandboxID+"/commands", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		requests = append(requests, body)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		ec, out, errStr := 0, "", ""
		if execResult != nil {
			ec, out, errStr = execResult.ExitCode, execResult.Stdout, execResult.Stderr
		}
		_, _ = fmt.Fprintf(w, `{"exitCode":%d,"stdout":%q,"stderr":%q}`, ec, out, errStr)
	})

	// DELETE /sandboxes/{id}
	mux.HandleFunc("/sandboxes/"+sandboxID, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		mu.Lock()
		requests = append(requests, map[string]any{"_action": "delete"})
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &requests
}

func TestE2BHandleFullLifecycle(t *testing.T) {
	const sbID = "sb-test-123"
	execResult := &struct {
		ExitCode int
		Stdout   string
		Stderr   string
	}{ExitCode: 0, Stdout: "hello-e2b", Stderr: ""}

	srv, _ := newMockE2BServer(t, sbID, execResult)

	p := NewE2BProvider(map[string]any{
		"api_key":     "test-key",
		"base_url":    srv.URL,
		"template_id": "base",
	}, nil)

	policy := DefaultPolicy()
	h, err := p.CreateHandle(nil, &policy)
	if err != nil {
		t.Fatalf("CreateHandle: %v", err)
	}
	if h.Status() != StatusCreated {
		t.Errorf("Status = %v, want created", h.Status())
	}

	ctx := context.Background()
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if h.Status() != StatusRunning {
		t.Errorf("Status = %v, want running", h.Status())
	}
	if h.(*E2BHandle).SandboxID() != sbID {
		t.Errorf("SandboxID = %q, want %q", h.(*E2BHandle).SandboxID(), sbID)
	}

	res, err := h.Execute(ctx, &Command{Argv: []string{"echo", "hello-e2b"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Stdout != "hello-e2b" {
		t.Errorf("Stdout = %q", res.Stdout)
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d", res.ExitCode)
	}

	if err := h.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if h.Status() != StatusStopped {
		t.Errorf("Status = %v, want stopped", h.Status())
	}
}

func TestE2BHandleStartRequestShape(t *testing.T) {
	const sbID = "sb-shape-001"
	srv, reqPtr := newMockE2BServer(t, sbID, nil)

	p := NewE2BProvider(map[string]any{
		"api_key":     "test-key",
		"base_url":    srv.URL,
		"template_id": "my-tmpl",
		"memory_mb":   512,
		"cpus":        1,
		"env":         map[string]any{"KEY": "VAL"},
	}, nil)
	h, _ := p.CreateHandle(nil, nil)
	ctx := context.Background()
	if err := h.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 校验 create sandbox 请求体包含我们设置的参数。
	if len(*reqPtr) == 0 {
		t.Fatal("no requests recorded")
	}
	first := (*reqPtr)[0]
	if first["templateID"] != "my-tmpl" {
		t.Errorf("templateID = %v", first["templateID"])
	}
	// JSON 解码后数字为 float64。
	if memMB, ok := first["memoryMB"].(float64); !ok || int(memMB) != 512 {
		t.Errorf("memoryMB = %v (%T)", first["memoryMB"], first["memoryMB"])
	}
	if cpus, ok := first["cpus"].(float64); !ok || int(cpus) != 1 {
		t.Errorf("cpus = %v (%T)", first["cpus"], first["cpus"])
	}
	envMap, ok := first["envVars"].(map[string]any)
	if !ok || envMap["KEY"] != "VAL" {
		t.Errorf("envVars = %v", first["envVars"])
	}
}

func TestE2BHandleExecuteWithoutStart(t *testing.T) {
	srv, _ := newMockE2BServer(t, "sb-x", nil)
	p := NewE2BProvider(map[string]any{"api_key": "k", "base_url": srv.URL}, nil)
	h, _ := p.CreateHandle(nil, nil)
	_, err := h.Execute(context.Background(), &Command{Argv: []string{"echo"}})
	if !errors.Is(err, ErrHandleNotRunning) {
		t.Fatalf("expected ErrHandleNotRunning, got %v", err)
	}
}

func TestE2BHandleExecuteNilCommand(t *testing.T) {
	srv, _ := newMockE2BServer(t, "sb-x", nil)
	p := NewE2BProvider(map[string]any{"api_key": "k", "base_url": srv.URL}, nil)
	h, _ := p.CreateHandle(nil, nil)
	ctx := context.Background()
	_ = h.Start(ctx)
	defer h.Stop(ctx)
	_, err := h.Execute(ctx, nil)
	if err == nil {
		t.Error("expected error for nil command")
	}
	_, err = h.Execute(ctx, &Command{Argv: nil})
	if err == nil {
		t.Error("expected error for empty argv")
	}
}

func TestE2BHandleAllowedCommands(t *testing.T) {
	const sbID = "sb-allowed"
	srv, _ := newMockE2BServer(t, sbID, &struct {
		ExitCode int
		Stdout   string
		Stderr   string
	}{0, "ok", ""})

	p := NewE2BProvider(map[string]any{
		"api_key":  "k",
		"base_url": srv.URL,
	}, nil)
	policy := DefaultPolicy()
	policy.AllowedCommands = []string{"ls", "echo"}
	h, _ := p.CreateHandle(nil, &policy)
	ctx := context.Background()
	_ = h.Start(ctx)
	defer h.Stop(ctx)

	// 允许的命令。
	_, err := h.Execute(ctx, &Command{Argv: []string{"echo", "hi"}})
	if err != nil {
		t.Errorf("echo should be allowed: %v", err)
	}
	// 禁止的命令。
	_, err = h.Execute(ctx, &Command{Argv: []string{"rm", "-rf", "/"}})
	if !errors.Is(err, ErrCommandNotAllowed) {
		t.Errorf("expected ErrCommandNotAllowed, got %v", err)
	}
}

func TestE2BHandleStartServerDown(t *testing.T) {
	// 使用一个会立即拒绝连接的 URL。
	p := NewE2BProvider(map[string]any{
		"api_key":  "k",
		"base_url": "http://127.0.0.1:0", // 0 端口通常不可用
	}, &http.Client{Timeout: 1 * time.Second})
	h, _ := p.CreateHandle(nil, nil)
	err := h.Start(context.Background())
	if err == nil {
		t.Error("expected error when server unreachable")
	}
	if h.Status() != StatusError {
		t.Errorf("Status = %v, want error", h.Status())
	}
}

func TestE2BHandleStopIdempotent(t *testing.T) {
	const sbID = "sb-stop"
	srv, _ := newMockE2BServer(t, sbID, nil)
	p := NewE2BProvider(map[string]any{"api_key": "k", "base_url": srv.URL}, nil)
	h, _ := p.CreateHandle(nil, nil)
	ctx := context.Background()
	_ = h.Start(ctx)
	if err := h.Stop(ctx); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	// 再次 Stop 应幂等。
	if err := h.Stop(ctx); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestE2BHandleStopWithoutStart(t *testing.T) {
	srv, _ := newMockE2BServer(t, "sb-x", nil)
	p := NewE2BProvider(map[string]any{"api_key": "k", "base_url": srv.URL}, nil)
	h, _ := p.CreateHandle(nil, nil)
	// 未 Start 直接 Stop 应安全返回。
	if err := h.Stop(context.Background()); err != nil {
		t.Errorf("Stop without Start: %v", err)
	}
	if h.Status() != StatusStopped {
		t.Errorf("Status = %v, want stopped", h.Status())
	}
}

func TestE2BHandleCreateHandlePolicyNil(t *testing.T) {
	srv, _ := newMockE2BServer(t, "sb-x", nil)
	p := NewE2BProvider(map[string]any{"api_key": "k", "base_url": srv.URL}, nil)
	h, err := p.CreateHandle(nil, nil)
	if err != nil {
		t.Fatalf("CreateHandle(nil, nil): %v", err)
	}
	if h == nil {
		t.Fatal("handle is nil")
	}
}

func TestE2BHandleDefaultTemplate(t *testing.T) {
	const sbID = "sb-tmpl"
	srv, reqPtr := newMockE2BServer(t, sbID, nil)
	p := NewE2BProvider(map[string]any{
		"api_key":  "k",
		"base_url": srv.URL,
	}, nil)
	h, _ := p.CreateHandle(nil, nil)
	ctx := context.Background()
	_ = h.Start(ctx)
	// 默认 templateID 应为 "base"。
	if len(*reqPtr) == 0 {
		t.Fatal("no requests recorded")
	}
	if (*reqPtr)[0]["templateID"] != "base" {
		t.Errorf("templateID = %v, want base", (*reqPtr)[0]["templateID"])
	}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestE2BProviderConcurrentInspect(t *testing.T) {
	t.Setenv("E2B_API_KEY", "")
	p := NewE2BProvider(nil, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			_, _ = p.InspectAvailability()
		}
	}()
	for i := 0; i < 50; i++ {
		_, _ = p.InspectAvailability()
	}
	<-done
}

func TestE2BHandleConcurrentStatus(t *testing.T) {
	const sbID = "sb-conc"
	srv, _ := newMockE2BServer(t, sbID, nil)
	p := NewE2BProvider(map[string]any{"api_key": "k", "base_url": srv.URL}, nil)
	h, _ := p.CreateHandle(nil, nil)
	ctx := context.Background()
	_ = h.Start(ctx)
	defer h.Stop(ctx)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			_ = h.Status()
		}
	}()
	for i := 0; i < 50; i++ {
		_ = h.Status()
	}
	<-done
}
