package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ============================================================================
// InputSource - 多输入源抽象
//
// InputSource 抽象了 "从某个来源获取一组命名值" 的过程。
// 典型用途：Flow 的 Step 在执行前需要从多个来源收集参数（环境变量、
// 上游 Session、HTTP 接口、静态配置等），InputSource 把这些来源统一
// 为 Resolve(ctx) 调用，便于组合与并发获取。
//
// 设计原则：
//   - 每个 Source 是独立的、无状态的（或自包含状态）。
//   - Resolve 返回的 map[string]any 的所有权转交给调用方。
//   - 多个 Source 通过 MultiSource 组合，使用 sync.WaitGroup 并发解析，
//     结果按键合并；冲突时后追加的覆盖先前的（可通过 MergeStrategy 调整）。
//   - 不引入 golang.org/x/sync 依赖，保持模块零外部依赖。
// ============================================================================

// InputSource 是所有输入源的统一契约。
//
// 调用方约定：
//   - Resolve 应支持并发安全调用（即同一 Source 实例可被多个 goroutine 同时调用）。
//   - ctx 取消时应尽快返回 context.Canceled 或其包装。
//   - 返回的 map 可能为空（无数据但不视为错误）。
type InputSource interface {
	// Name 返回源名称，用于日志和错误定位。
	Name() string
	// Resolve 从源解析出键值对。
	Resolve(ctx context.Context) (map[string]any, error)
}

// ============================================================================
// StaticValueSource - 静态值
// ============================================================================

// StaticValueSource 返回构造时预定义的静态 map。
// 用于在 Flow 配置中直接嵌入常量参数。
type StaticValueSource struct {
	name  string
	value map[string]any
}

// NewStaticValueSource 创建一个静态值源。
// value 会被浅拷贝以避免外部修改影响内部状态。
func NewStaticValueSource(name string, value map[string]any) *StaticValueSource {
	out := make(map[string]any, len(value))
	for k, v := range value {
		out[k] = v
	}
	return &StaticValueSource{name: name, value: out}
}

// Name 返回源名称。
func (s *StaticValueSource) Name() string { return s.name }

// Resolve 返回静态 map 的副本。
func (s *StaticValueSource) Resolve(ctx context.Context) (map[string]any, error) {
	if s == nil {
		return nil, fmt.Errorf("static source: nil receiver")
	}
	out := make(map[string]any, len(s.value))
	for k, v := range s.value {
		out[k] = v
	}
	return out, nil
}

// 编译期断言
var _ InputSource = (*StaticValueSource)(nil)

// ============================================================================
// EnvSource - 环境变量
// ============================================================================

// EnvSource 从进程环境变量中读取指定键集合。
//
// 行为：
//   - 若 keys 为空，则读取所有环境变量（os.Environ）。
//   - 若 prefix 非空，仅读取以 prefix 开头的键（并去掉前缀）。
//   - 缺失的 key 不视为错误（除非 Required=true）。
type EnvSource struct {
	name     string
	keys     []string
	prefix   string
	required []string
}

// NewEnvSource 创建一个环境变量源。
//   - name: 源名称
//   - keys: 显式要读取的键列表（可为空，表示读全部）
func NewEnvSource(name string, keys ...string) *EnvSource {
	return &EnvSource{name: name, keys: keys}
}

// WithPrefix 设置键前缀过滤。设置后只返回以 prefix 开头的键，并在结果中去除前缀。
// 链式调用，返回 Source 自身。
func (s *EnvSource) WithPrefix(prefix string) *EnvSource {
	s.prefix = prefix
	return s
}

// WithRequired 标记某些键为必需：若缺失则返回错误。
// 链式调用。keys 不在 s.keys 中时会自动追加。
func (s *EnvSource) WithRequired(keys ...string) *EnvSource {
	s.required = append(s.required, keys...)
	// 同步到 keys，避免遗漏。
	for _, r := range keys {
		found := false
		for _, k := range s.keys {
			if k == r {
				found = true
				break
			}
		}
		if !found {
			s.keys = append(s.keys, r)
		}
	}
	return s
}

// Name 返回源名称。
func (s *EnvSource) Name() string { return s.name }

// Resolve 读取环境变量。
func (s *EnvSource) Resolve(ctx context.Context) (map[string]any, error) {
	if s == nil {
		return nil, fmt.Errorf("env source: nil receiver")
	}
	out := make(map[string]any)

	if len(s.keys) == 0 {
		// 读取所有环境变量。
		for _, env := range os.Environ() {
			eq := strings.IndexByte(env, '=')
			if eq < 0 {
				continue
			}
			k := env[:eq]
			v := env[eq+1:]
			if s.prefix != "" {
				if !strings.HasPrefix(k, s.prefix) {
					continue
				}
				k = strings.TrimPrefix(k, s.prefix)
			}
			out[k] = v
		}
	} else {
		// 仅读取指定 keys。
		for _, k := range s.keys {
			v, ok := os.LookupEnv(k)
			if !ok {
				if s.containsRequired(k) {
					return nil, fmt.Errorf("env source %q: required env %q not set", s.name, k)
				}
				continue
			}
			key := k
			if s.prefix != "" {
				key = strings.TrimPrefix(k, s.prefix)
			}
			out[key] = v
		}
	}
	return out, nil
}

// containsRequired 判断 k 是否在 required 列表中。
func (s *EnvSource) containsRequired(k string) bool {
	for _, r := range s.required {
		if r == k {
			return true
		}
	}
	return false
}

// 编译期断言
var _ InputSource = (*EnvSource)(nil)

// ============================================================================
// SessionSource - 会话内存存储
// ============================================================================

// SessionStore 是 SessionSource 依赖的会话存储接口。
// 典型实现：基于 sync.RWMutex 的 map（SessionStore 的默认实现）。
type SessionStore interface {
	// Get 返回指定 sessionID 下的所有键值对（副本）。
	Get(ctx context.Context, sessionID string) (map[string]any, error)
	// Set 在指定 sessionID 下设置 key=value。
	Set(ctx context.Context, sessionID string, key string, value any) error
	// Delete 删除指定 sessionID 下的 key。
	Delete(ctx context.Context, sessionID, key string) error
}

// InMemorySessionStore 是基于 sync.RWMutex 的 SessionStore 默认实现。
type InMemorySessionStore struct {
	mu       sync.RWMutex
	sessions map[string]map[string]any
}

// NewInMemorySessionStore 创建空会话存储。
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		sessions: make(map[string]map[string]any),
	}
}

// Get 返回 sessionID 下的所有键值对（深拷贝顶层 map）。
// 不存在的 sessionID 返回空 map（非错误）。
func (s *InMemorySessionStore) Get(ctx context.Context, sessionID string) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src, ok := s.sessions[sessionID]
	if !ok {
		return map[string]any{}, nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out, nil
}

// Set 在 sessionID 下设置 key=value。若 session 不存在则创建。
func (s *InMemorySessionStore) Set(ctx context.Context, sessionID, key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[sessionID]; !ok {
		s.sessions[sessionID] = make(map[string]any)
	}
	s.sessions[sessionID][key] = value
	return nil
}

// Delete 删除 sessionID 下的 key。键不存在不算错误。
func (s *InMemorySessionStore) Delete(ctx context.Context, sessionID, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[sessionID]; ok {
		delete(sess, key)
	}
	return nil
}

// 编译期断言
var _ SessionStore = (*InMemorySessionStore)(nil)

// SessionSource 从 SessionStore 读取指定 sessionID 下的所有键值对。
type SessionSource struct {
	name      string
	store     SessionStore
	sessionID string
}

// NewSessionSource 创建一个会话源。
//   - store 不能为 nil，否则 Resolve 返回错误。
func NewSessionSource(name string, store SessionStore, sessionID string) *SessionSource {
	return &SessionSource{name: name, store: store, sessionID: sessionID}
}

// Name 返回源名称。
func (s *SessionSource) Name() string { return s.name }

// Resolve 从 store 读取 sessionID 下的所有键值对。
func (s *SessionSource) Resolve(ctx context.Context) (map[string]any, error) {
	if s == nil {
		return nil, fmt.Errorf("session source: nil receiver")
	}
	if s.store == nil {
		return nil, fmt.Errorf("session source %q: store is nil", s.name)
	}
	if s.sessionID == "" {
		return nil, fmt.Errorf("session source %q: empty session id", s.name)
	}
	return s.store.Get(ctx, s.sessionID)
}

// 编译期断言
var _ InputSource = (*SessionSource)(nil)

// ============================================================================
// HTTPSource - HTTP JSON 接口
// ============================================================================

// HTTPSource 通过 HTTP GET 拉取 JSON 响应并解码为 map[string]any。
//
// 默认行为：
//   - 使用 http.DefaultClient。
//   - Timeout 0 表示无超时（不推荐，建议显式设置）。
//   - 响应 Content-Type 不强制校验。
//   - 响应体大小不限（流式解码）。
type HTTPSource struct {
	name     string
	url      string
	client   *http.Client
	timeout  time.Duration
	headers  map[string]string
}

// NewHTTPSource 创建一个 HTTP 源。
//   - url 必须是合法的 http/https URL。
func NewHTTPSource(name, url string) *HTTPSource {
	return &HTTPSource{
		name:   name,
		url:    url,
		client: http.DefaultClient,
	}
}

// WithClient 注入自定义 http.Client（用于测试或定制 transport）。
func (s *HTTPSource) WithClient(c *http.Client) *HTTPSource {
	s.client = c
	return s
}

// WithTimeout 设置单次请求超时。
func (s *HTTPSource) WithTimeout(d time.Duration) *HTTPSource {
	s.timeout = d
	return s
}

// WithHeader 添加一个 HTTP header（链式）。
func (s *HTTPSource) WithHeader(key, value string) *HTTPSource {
	if s.headers == nil {
		s.headers = make(map[string]string)
	}
	s.headers[key] = value
	return s
}

// Name 返回源名称。
func (s *HTTPSource) Name() string { return s.name }

// Resolve 发起 HTTP GET，解析 JSON 响应为 map。
// 仅当响应体为 JSON 对象（顶层为 { ... }）时返回非空 map。
func (s *HTTPSource) Resolve(ctx context.Context) (map[string]any, error) {
	if s == nil {
		return nil, fmt.Errorf("http source: nil receiver")
	}
	if s.url == "" {
		return nil, fmt.Errorf("http source %q: empty url", s.name)
	}

	reqCtx, cancel := context.WithTimeout(ctx, s.effectiveTimeout())
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, s.url, nil)
	if err != nil {
		return nil, fmt.Errorf("http source %q: build request: %w", s.name, err)
	}
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http source %q: request: %w", s.name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("http source %q: status %d: %s", s.name, resp.StatusCode, string(body))
	}

	var out map[string]any
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("http source %q: decode json: %w", s.name, err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// effectiveTimeout 返回实际超时；0 时回退到 30s。
func (s *HTTPSource) effectiveTimeout() time.Duration {
	if s.timeout > 0 {
		return s.timeout
	}
	return 30 * time.Second
}

// 编译期断言
var _ InputSource = (*HTTPSource)(nil)

// ============================================================================
// MultiSource - 多源并发合并
// ============================================================================

// MergeStrategy 描述 MultiSource 在合并多个源结果时如何处理键冲突。
type MergeStrategy int

const (
	// MergeLastWins 默认策略：后追加的源覆盖先前的。
	MergeLastWins MergeStrategy = iota
	// MergeFirstWins 先到先得：若键已存在则保留首次设置的值。
	MergeFirstWins
	// MergeError 冲突时返回错误。
	MergeError
)

// MultiSource 将多个 InputSource 的解析结果合并为一个 map。
//
// 并发模型：所有源通过 sync.WaitGroup 并发解析，任一源出错都会被收集
// （取决于 FailFast 设置）。若 FailFast=true，第一个错误返回时立即取消
// 其他源（通过 ctx cancel）。
type MultiSource struct {
	name      string
	sources   []InputSource
	strategy  MergeStrategy
	failFast  bool
}

// NewMultiSource 创建一个 MultiSource。
func NewMultiSource(name string, sources ...InputSource) *MultiSource {
	filtered := make([]InputSource, 0, len(sources))
	for _, s := range sources {
		if s != nil {
			filtered = append(filtered, s)
		}
	}
	return &MultiSource{
		name:     name,
		sources:  filtered,
		strategy: MergeLastWins,
	}
}

// WithStrategy 设置合并策略。链式。
func (m *MultiSource) WithStrategy(s MergeStrategy) *MultiSource {
	m.strategy = s
	return m
}

// WithFailFast 设置是否快速失败。链式。
// true: 任一源出错立即取消其他源并返回该错误。
// false: 等待所有源完成，收集所有错误后返回第一个。
func (m *MultiSource) WithFailFast(b bool) *MultiSource {
	m.failFast = b
	return m
}

// Name 返回源名称。
func (m *MultiSource) Name() string { return m.name }

// Resolve 并发解析所有子源并合并结果。
func (m *MultiSource) Resolve(ctx context.Context) (map[string]any, error) {
	if m == nil {
		return nil, fmt.Errorf("multi source: nil receiver")
	}
	if len(m.sources) == 0 {
		return map[string]any{}, nil
	}

	// 准备可取消的 ctx（用于 failFast）。
	resolvableCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		idx    int
		values map[string]any
		err    error
	}

	var wg sync.WaitGroup
	results := make([]result, len(m.sources))

	for i, src := range m.sources {
		wg.Add(1)
		go func(idx int, s InputSource) {
			defer wg.Done()
			v, err := s.Resolve(resolvableCtx)
			results[idx] = result{idx: idx, values: v, err: err}
			if err != nil && m.failFast {
				cancel() // 取消其他源
			}
		}(i, src)
	}
	wg.Wait()

	// 按原顺序合并，以保持 MergeLastWins/MergeFirstWins 的可预测性。
	out := make(map[string]any)
	var firstErr error
	for _, r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		for k, v := range r.values {
			switch m.strategy {
			case MergeLastWins:
				out[k] = v
			case MergeFirstWins:
				if _, exists := out[k]; !exists {
					out[k] = v
				}
			case MergeError:
				if _, exists := out[k]; exists {
					return nil, fmt.Errorf("multi source %q: key %q conflict between sources", m.name, k)
				}
				out[k] = v
			}
		}
	}
	if firstErr != nil && m.failFast {
		return out, firstErr
	}
	// 非 failFast 模式下，即使有错误也返回已收集的结果。
	// 调用方可根据需要检查 err。
	if firstErr != nil {
		return out, firstErr
	}
	return out, nil
}

// Sources 返回子源列表的副本。
func (m *MultiSource) Sources() []InputSource {
	out := make([]InputSource, len(m.sources))
	copy(out, m.sources)
	return out
}

// 编译期断言
var _ InputSource = (*MultiSource)(nil)

// ============================================================================
// 辅助：ResolveAll 一次性解析多个源
// ============================================================================

// ResolveAll 是 MultiSource 的便捷等价：并发解析多个源并合并。
// 等同于 NewMultiSource("resolve-all", sources...).Resolve(ctx)。
func ResolveAll(ctx context.Context, sources ...InputSource) (map[string]any, error) {
	return NewMultiSource("resolve-all", sources...).Resolve(ctx)
}
