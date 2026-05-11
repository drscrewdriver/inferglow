package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// E2B Provider
//
// E2B (https://e2b.dev) 是基于 Firecracker 微虚拟机的远程代码执行平台。
// 通过 HTTPS API 创建、操作、销毁沙箱实例，所有真实隔离发生在 E2B 云端。
//
// 本实现不依赖 E2B 官方 Go SDK，而是直接调用其 REST API，以保持 sandbox
// 模块依赖最小化。SDK 用户可直接参考本文件自行替换为 SDK 调用。
//
// 鉴权：通过 E2B_API_KEY 环境变量或 cfg["api_key"] 提供。
// 缺失时 Provider 仍可构造，但 InspectAvailability 返回 false。
//
// 注：由于真实 E2B API 需要注册的 API Key，本实现的集成测试仅在
// E2B_API_KEY 存在或通过 mockHTTPClient 注入时才执行。
// ============================================================================

// e2bDefaultBaseURL 是 E2B REST API 的默认基址。
const e2bDefaultBaseURL = "https://api.e2b.dev/v1"

// E2BConfig 描述 E2B 沙箱的配置选项。
type E2BConfig struct {
	// APIKey 用于鉴权（必填）。
	APIKey string
	// BaseURL E2B API 基址，默认 https://api.e2b.dev/v1。
	BaseURL string
	// TemplateID 沙箱使用的模板（如 "base" 或自定义模板 ID）。
	TemplateID string
	// Timeout 单次 HTTP 请求超时。0 表示 30s。
	Timeout time.Duration
	// MemoryMB 沙箱内存（MB）。0 表示 E2B 默认。
	MemoryMB int
	// CPUs 沙箱 vCPU 数。0 表示 E2B 默认。
	CPUs int
	// EnvVars 沙箱启动时注入的环境变量。
	EnvVars map[string]string
	// Metadata 附加到沙箱的元数据（用户自定义）。
	Metadata map[string]string
}

// parseE2BConfig 从 cfg map[string]any 解析 E2BConfig。
//
// 已知字段（均字符串/数字，便于从 YAML/JSON 配置加载）：
//   - "api_key"     (string)
//   - "base_url"    (string)
//   - "template_id" (string)
//   - "timeout_seconds" (int/float)
//   - "memory_mb"   (int)
//   - "cpus"        (int)
//   - "env"         (map[string]string 或 map[string]any)
//   - "metadata"    (map[string]string 或 map[string]any)
func parseE2BConfig(cfg map[string]any) E2BConfig {
	out := E2BConfig{
		BaseURL: e2bDefaultBaseURL,
	}
	if cfg == nil {
		return out
	}
	if v, ok := cfg["api_key"].(string); ok {
		out.APIKey = v
	}
	if v, ok := cfg["base_url"].(string); ok && v != "" {
		out.BaseURL = v
	}
	if v, ok := cfg["template_id"].(string); ok {
		out.TemplateID = v
	}
	switch d := cfg["timeout_seconds"].(type) {
	case int:
		out.Timeout = time.Duration(d) * time.Second
	case int64:
		out.Timeout = time.Duration(d) * time.Second
	case float64:
		out.Timeout = time.Duration(d) * time.Second
	}
	if v, ok := cfg["memory_mb"].(int); ok {
		out.MemoryMB = v
	}
	if v, ok := cfg["cpus"].(int); ok {
		out.CPUs = v
	}
	if v, ok := cfg["env"]; ok {
		out.EnvVars = parseStringMap(v)
	}
	if v, ok := cfg["metadata"]; ok {
		out.Metadata = parseStringMap(v)
	}
	return out
}

// parseStringMap 接受 map[string]string 或 map[string]any，返回 map[string]string。
func parseStringMap(v any) map[string]string {
	if v == nil {
		return nil
	}
	out := map[string]string{}
	switch t := v.(type) {
	case map[string]string:
		for k, v := range t {
			out[k] = v
		}
		return out
	case map[string]any:
		for k, v := range t {
			switch s := v.(type) {
			case string:
				out[k] = s
			case fmt.Stringer:
				out[k] = s.String()
			default:
				out[k] = fmt.Sprintf("%v", v)
			}
		}
		return out
	}
	return nil
}

// e2bHTTPClient 是 E2BProvider 使用的 HTTP 客户端抽象，便于测试注入。
type e2bHTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// E2BProvider 是基于 E2B 云端 API 的沙箱 Provider。
type E2BProvider struct {
	mu        sync.RWMutex
	defaultCfg E2BConfig
	client    e2bHTTPClient
	available bool
}

// NewE2BProvider 构造 E2BProvider。
//   - apiKey 从 cfg["api_key"] 或环境变量 E2B_API_KEY 读取。
//   - baseURL 默认 e2bDefaultBaseURL，可通过 cfg["base_url"] 覆盖。
//   - httpClient 为 nil 时使用 http.DefaultClient。
//   - cfg 中的其他字段（template_id、memory_mb、cpus、env、metadata、timeout_seconds）
//     作为默认值，可在 CreateHandle 时被同名字段覆盖。
//
// 若 API key 缺失，Provider 仍可构造，但 InspectAvailability 返回 false。
func NewE2BProvider(cfg map[string]any, httpClient e2bHTTPClient) *E2BProvider {
	e2bCfg := parseE2BConfig(cfg)
	if e2bCfg.APIKey == "" {
		e2bCfg.APIKey = os.Getenv("E2B_API_KEY")
	}
	if e2bCfg.BaseURL == "" {
		e2bCfg.BaseURL = e2bDefaultBaseURL
	}
	e2bCfg.BaseURL = strings.TrimRight(e2bCfg.BaseURL, "/")
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &E2BProvider{
		defaultCfg: e2bCfg,
		client:     httpClient,
		available:  e2bCfg.APIKey != "",
	}
}

// Name 返回 "e2b"。
func (p *E2BProvider) Name() string { return "e2b" }

// Kind 返回 "remote"（远程沙箱）。
func (p *E2BProvider) Kind() string { return "remote" }

// InspectAvailability 报告 E2B Provider 是否配置了 API key。
// 实际网络连通性在第一次 CreateHandle / Start 时验证。
func (p *E2BProvider) InspectAvailability() (*AvailabilityResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	res := &AvailabilityResult{
		Available:  p.available,
		Platform:   "remote",
		BinaryPath: p.defaultCfg.BaseURL,
	}
	if !p.available {
		res.ErrorMessage = "E2B_API_KEY not set"
	}
	return res, nil
}

// CreateHandle 根据 cfg 和 policy 创建 E2BHandle。
// 若 cfg 未提供某字段，回退到 Provider 构造时的默认值。
func (p *E2BProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	if policy == nil {
		def := DefaultPolicy()
		policy = &def
	}
	p.mu.RLock()
	defaults := p.defaultCfg
	client := p.client
	p.mu.RUnlock()

	// 用 cfg 覆盖默认值。
	e2bCfg := parseE2BConfig(cfg)
	if e2bCfg.APIKey == "" {
		e2bCfg.APIKey = defaults.APIKey
	}
	if e2bCfg.BaseURL == "" || e2bCfg.BaseURL == e2bDefaultBaseURL {
		e2bCfg.BaseURL = defaults.BaseURL
	}
	if e2bCfg.TemplateID == "" {
		e2bCfg.TemplateID = defaults.TemplateID
	}
	if e2bCfg.Timeout == 0 {
		e2bCfg.Timeout = defaults.Timeout
	}
	if e2bCfg.MemoryMB == 0 {
		e2bCfg.MemoryMB = defaults.MemoryMB
	}
	if e2bCfg.CPUs == 0 {
		e2bCfg.CPUs = defaults.CPUs
	}
	if e2bCfg.EnvVars == nil {
		e2bCfg.EnvVars = defaults.EnvVars
	} else if len(defaults.EnvVars) > 0 {
		merged := make(map[string]string, len(defaults.EnvVars)+len(e2bCfg.EnvVars))
		for k, v := range defaults.EnvVars {
			merged[k] = v
		}
		for k, v := range e2bCfg.EnvVars {
			merged[k] = v
		}
		e2bCfg.EnvVars = merged
	}
	if e2bCfg.Metadata == nil {
		e2bCfg.Metadata = defaults.Metadata
	} else if len(defaults.Metadata) > 0 {
		merged := make(map[string]string, len(defaults.Metadata)+len(e2bCfg.Metadata))
		for k, v := range defaults.Metadata {
			merged[k] = v
		}
		for k, v := range e2bCfg.Metadata {
			merged[k] = v
		}
		e2bCfg.Metadata = merged
	}
	if e2bCfg.APIKey == "" {
		return nil, fmt.Errorf("%w: E2B_API_KEY not set", ErrProviderUnavailable)
	}
	if e2bCfg.TemplateID == "" {
		e2bCfg.TemplateID = "base"
	}
	return &E2BHandle{
		cfg:    e2bCfg,
		policy: policy,
		client: client,
		status: StatusCreated,
	}, nil
}

// 编译期断言
var _ Provider = (*E2BProvider)(nil)

// ============================================================================
// E2BHandle
// ============================================================================

// E2BHandle 包装一个 E2B 远程沙箱的生命周期。
//
// 状态流转：
//   - Start: POST /sandboxes 创建沙箱，记录 sandboxID
//   - Execute: POST /sandboxes/{id}/commands 运行命令
//   - Stop: DELETE /sandboxes/{id} 销毁沙箱
type E2BHandle struct {
	mu        sync.Mutex
	cfg       E2BConfig
	policy    *ExecutionPolicy
	client    e2bHTTPClient
	status    HandleStatus
	sandboxID string
}

// Start 通过 E2B API 创建一个新沙箱。
func (h *E2BHandle) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.status == StatusRunning {
		return nil
	}
	if h.cfg.APIKey == "" {
		return fmt.Errorf("e2b: missing API key")
	}

	body := map[string]any{
		"templateID": h.cfg.TemplateID,
	}
	if h.cfg.MemoryMB > 0 {
		body["memoryMB"] = h.cfg.MemoryMB
	}
	if h.cfg.CPUs > 0 {
		body["cpus"] = h.cfg.CPUs
	}
	if len(h.cfg.EnvVars) > 0 {
		body["envVars"] = h.cfg.EnvVars
	}
	if len(h.cfg.Metadata) > 0 {
		body["metadata"] = h.cfg.Metadata
	}

	var resp struct {
		SandboxID string `json:"sandboxID"`
		ID        string `json:"id"`
	}
	if err := h.doRequest(ctx, http.MethodPost, "/sandboxes", body, &resp); err != nil {
		h.status = StatusError
		return fmt.Errorf("e2b: create sandbox: %w", err)
	}
	id := resp.SandboxID
	if id == "" {
		id = resp.ID
	}
	if id == "" {
		h.status = StatusError
		return fmt.Errorf("e2b: create sandbox: response missing sandbox id")
	}
	h.sandboxID = id
	h.status = StatusRunning
	return nil
}

// Execute 在远程沙箱中执行命令。
// 调用者必须先调用 Start。
func (h *E2BHandle) Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error) {
	if cmd == nil || len(cmd.Argv) == 0 {
		return nil, fmt.Errorf("e2b: command is nil or empty")
	}
	h.mu.Lock()
	running := h.status == StatusRunning
	sandboxID := h.sandboxID
	h.mu.Unlock()
	if !running {
		return nil, fmt.Errorf("%w: e2b handle not running", ErrHandleNotRunning)
	}
	if sandboxID == "" {
		return nil, fmt.Errorf("e2b: sandbox id is empty")
	}

	// AllowedCommands 白名单（与其他 provider 一致）。
	if h.policy != nil && len(h.policy.AllowedCommands) > 0 {
		allowed := false
		for _, c := range h.policy.AllowedCommands {
			if c == cmd.Argv[0] {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("%w: %q", ErrCommandNotAllowed, cmd.Argv[0])
		}
	}

	// 构造命令请求。E2B 的 commands 端点接受 cmd 字符串。
	body := map[string]any{
		"cmd": strings.Join(cmd.Argv, " "),
	}
	if cmd.Workdir != "" {
		body["workdir"] = cmd.Workdir
	}
	if len(cmd.Env) > 0 {
		envMap := make(map[string]string, len(cmd.Env))
		for _, e := range cmd.Env {
			if i := strings.IndexByte(e, '='); i >= 0 {
				envMap[e[:i]] = e[i+1:]
			}
		}
		body["envs"] = envMap
	}

	// Timeout：cfg.Timeout 优先，其次 policy.Timeout，最后 30s。
	timeout := h.cfg.Timeout
	if timeout <= 0 && h.policy != nil && h.policy.Timeout > 0 {
		timeout = h.policy.Timeout
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	var resp struct {
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		Error    string `json:"error"`
	}
	path := fmt.Sprintf("/sandboxes/%s/commands", sandboxID)
	if err := h.doRequest(reqCtx, http.MethodPost, path, body, &resp); err != nil {
		return nil, fmt.Errorf("e2b: execute command: %w", err)
	}
	duration := time.Since(start)

	result := &ExecutionResult{
		ExitCode: resp.ExitCode,
		Stdout:   resp.Stdout,
		Stderr:   resp.Stderr,
		Duration: duration,
	}
	if resp.Error != "" {
		// 服务端报告错误，但仍返回结果（exitCode 通常非 0）。
		if result.Stderr == "" {
			result.Stderr = resp.Error
		}
	}
	return result, nil
}

// Stop 通过 E2B API 销毁远程沙箱。
// 多次调用幂等。未 Start 的 handle 调用 Stop 直接转为 Stopped。
func (h *E2BHandle) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.status == StatusStopped {
		return nil
	}
	if h.sandboxID != "" {
		path := fmt.Sprintf("/sandboxes/%s", h.sandboxID)
		_ = h.doRequest(ctx, http.MethodDelete, path, nil, nil)
		h.sandboxID = ""
	}
	h.status = StatusStopped
	return nil
}

// Status 返回当前生命周期状态。
func (h *E2BHandle) Status() HandleStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

// SandboxID 返回 E2B 沙箱 ID（Start 后有效）。
func (h *E2BHandle) SandboxID() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sandboxID
}

// 编译期断言
var _ Handle = (*E2BHandle)(nil)

// ============================================================================
// HTTP 请求辅助
// ============================================================================

// doRequest 发送 HTTP 请求并解析 JSON 响应。
//   - method: HTTP 方法
//   - path: 相对 baseURL 的路径（以 / 开头）
//   - body: 请求体（nil 表示无 body）
//   - out: 响应解码目标（nil 表示不解析）
func (h *E2BHandle) doRequest(ctx context.Context, method, path string, body any, out any) error {
	url := h.cfg.BaseURL + path

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+h.cfg.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// 读取最多 4KB 用于错误消息。
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(errBody))
	}

	if out != nil {
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(out); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("decode json: %w", err)
		}
	}
	return nil
}

// RegisterE2BProvider 将 E2BProvider 注册到 Manager（若 API key 可用）。
func RegisterE2BProvider(m *Manager, cfg map[string]any, httpClient e2bHTTPClient) error {
	provider := NewE2BProvider(cfg, httpClient)
	avail, err := provider.InspectAvailability()
	if err != nil {
		return err
	}
	if !avail.Available {
		return ErrProviderUnavailable
	}
	return m.Register(provider)
}
