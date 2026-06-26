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

//go:build linux

package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// BubblewrapConfig 描述 Bubblewrap 沙箱的配置选项。
//
// 字段从 cfg map[string]any 解析而来，便于调用方以结构化方式传递。
type BubblewrapConfig struct {
	// BindRO 只读 bind mount：源路径 -> 目标路径。
	// 典型用法：把宿主 /usr、/bin 等只读挂入沙箱。
	BindRO []MountEntry
	// BindRW 读写 bind mount：源路径 -> 目标路径。
	// 典型用法：把宿主的临时工作目录挂入沙箱可写。
	BindRW []MountEntry
	// Tmpfs 在沙箱内挂载 tmpfs，key 为挂载点（绝对路径），value 为大小（字节，0 表示默认）。
	Tmpfs map[string]int64
	// UnshareAll 等价于 --unshare-all，隔离 user/pid/uts/ipc/cgroup/net 命名空间。
	// 与 ShareNet 互斥（后者会重新共享网络）。
	UnshareAll bool
	// ShareNet 在 UnshareAll 之后重新共享网络（--share-net）。
	// 通常用于需要联网但不需要其他命名空间隔离的场景。
	ShareNet bool
	// Clearenv 清空所有环境变量（--clearenv）。
	Clearenv bool
	// Timeout 单次 Execute 的最长执行时间。0 表示由 policy 决定。
	Timeout time.Duration
	// NewSession 在新会话中启动进程（setsid，--new-session）。
	NewSession bool
	// DieWithParent 父进程退出时沙箱内进程一并退出（--die-with-parent）。
	DieWithParent bool
}

// parseBubblewrapConfig 从 cfg map[string]any 解析 BubblewrapConfig。
//
// 已知字段：
//   - "bind_ro" ([]any of [2]string 或 []string "src:dst" 或 []MountEntry)
//   - "bind_rw" (同上)
//   - "tmpfs" (map[string]int 或 map[string]int64)
//   - "unshare_all" (bool)
//   - "share_net" (bool)
//   - "clearenv" (bool)
//   - "timeout_seconds" (int/float)
//   - "new_session" (bool)
//   - "die_with_parent" (bool)
//
// 未识别字段会被忽略，便于向前兼容。
func parseBubblewrapConfig(cfg map[string]any) BubblewrapConfig {
	out := BubblewrapConfig{}
	if cfg == nil {
		return out
	}
	if v, ok := cfg["bind_ro"]; ok {
		out.BindRO = parseMountEntries(v)
	}
	if v, ok := cfg["bind_rw"]; ok {
		out.BindRW = parseMountEntries(v)
	}
	if v, ok := cfg["tmpfs"]; ok {
		out.Tmpfs = parseTmpfsMap(v)
	}
	if v, ok := cfg["unshare_all"].(bool); ok {
		out.UnshareAll = v
	}
	if v, ok := cfg["share_net"].(bool); ok {
		out.ShareNet = v
	}
	if v, ok := cfg["clearenv"].(bool); ok {
		out.Clearenv = v
	}
	if v, ok := cfg["new_session"].(bool); ok {
		out.NewSession = v
	}
	if v, ok := cfg["die_with_parent"].(bool); ok {
		out.DieWithParent = v
	}
	switch d := cfg["timeout_seconds"].(type) {
	case int:
		out.Timeout = time.Duration(d) * time.Second
	case int64:
		out.Timeout = time.Duration(d) * time.Second
	case float64:
		out.Timeout = time.Duration(d * float64(time.Second))
	}
	return out
}

// parseMountEntries 解析多种格式的 mount 列表。
// 支持：
//   - []MountEntry
//   - []any 元素为 [2]string 或 []string 或 map[string]any{"source":...,"destination":...,"read_only":...}
//   - []string 形如 "src:dst" 或 "src:dst:ro"
func parseMountEntries(v any) []MountEntry {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []MountEntry:
		return t
	case []any:
		out := make([]MountEntry, 0, len(t))
		for _, item := range t {
			switch m := item.(type) {
			case MountEntry:
				out = append(out, m)
			case [2]string:
				out = append(out, MountEntry{Source: m[0], Destination: m[1]})
			case []string:
				if len(m) >= 2 {
					ro := false
					if len(m) >= 3 && (m[2] == "ro" || m[2] == "true") {
						ro = true
					}
					out = append(out, MountEntry{Source: m[0], Destination: m[1], ReadOnly: ro})
				}
			case map[string]any:
				me := MountEntry{}
				if s, ok := m["source"].(string); ok {
					me.Source = s
				}
				if d, ok := m["destination"].(string); ok {
					me.Destination = d
				}
				if ro, ok := m["read_only"].(bool); ok {
					me.ReadOnly = ro
				}
				out = append(out, me)
			}
		}
		return out
	case []string:
		out := make([]MountEntry, 0, len(t))
		for _, s := range t {
			parts := strings.Split(s, ":")
			if len(parts) < 2 {
				continue
			}
			ro := false
			if len(parts) >= 3 && (parts[2] == "ro" || parts[2] == "true") {
				ro = true
			}
			out = append(out, MountEntry{Source: parts[0], Destination: parts[1], ReadOnly: ro})
		}
		return out
	}
	return nil
}

// parseTmpfsMap 解析 tmpfs 配置。
// 接受 map[string]int / map[string]int64 / map[string]any。
func parseTmpfsMap(v any) map[string]int64 {
	if v == nil {
		return nil
	}
	out := map[string]int64{}
	switch t := v.(type) {
	case map[string]int:
		for k, val := range t {
			out[k] = int64(val)
		}
		return out
	case map[string]int64:
		for k, val := range t {
			out[k] = val
		}
		return out
	case map[string]any:
		for k, val := range t {
			switch n := val.(type) {
			case int:
				out[k] = int64(n)
			case int64:
				out[k] = n
			case float64:
				out[k] = int64(n)
			}
		}
		return out
	}
	return nil
}

// BubblewrapProvider 是 Linux 上基于 bwrap 命令的沙箱 Provider。
//
// Bubblewrap（bwrap）是大多数 Linux 发行版标配的非特权用户命名空间沙箱，
// 无需 root 即可创建隔离的进程环境。
type BubblewrapProvider struct {
	mu         sync.RWMutex
	binaryPath string
	available  bool
	probeErr   error
}

// NewBubblewrapProvider 探测系统 bwrap 并构造 Provider。
// 若 bwrap 不在 PATH 或不可执行，Provider 创建成功但 InspectAvailability 返回 false。
func NewBubblewrapProvider() *BubblewrapProvider {
	p := &BubblewrapProvider{}
	p.probe()
	return p
}

// probe 通过 exec.LookPath 探测 bwrap，缓存结果以便并发读取。
func (p *BubblewrapProvider) probe() {
	path, err := exec.LookPath("bwrap")
	p.mu.Lock()
	defer p.mu.Unlock()
	p.binaryPath = path
	p.available = err == nil
	p.probeErr = err
}

// Name 返回 "bubblewrap"。
func (p *BubblewrapProvider) Name() string { return "bubblewrap" }

// Kind 返回 "local"。
func (p *BubblewrapProvider) Kind() string { return "local" }

// InspectAvailability 返回 bwrap 的可用性信息。
// 复探测一次以便调用方在安装 bwrap 后无需重建 Provider 即可恢复可用性。
func (p *BubblewrapProvider) InspectAvailability() (*AvailabilityResult, error) {
	p.probe()
	p.mu.RLock()
	defer p.mu.RUnlock()
	res := &AvailabilityResult{
		Available:  p.available,
		Platform:   string(OSLinux),
		BinaryPath: p.binaryPath,
	}
	if !p.available && p.probeErr != nil {
		res.ErrorMessage = fmt.Sprintf("bwrap not found in PATH: %v", p.probeErr)
	}
	if p.available {
		// 探测版本
		if ver, err := bwrapVersion(p.binaryPath); err == nil {
			res.Version = ver
		}
	}
	return res, nil
}

// bwrapVersion 执行 `bwrap --version` 解析版本字符串。
func bwrapVersion(path string) (string, error) {
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return "", err
	}
	// 输出形如 "bubblewrap 0.10.0"
	s := strings.TrimSpace(string(out))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s, nil
}

// CreateHandle 根据配置和策略创建 BubblewrapHandle。
func (p *BubblewrapProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	p.mu.RLock()
	avail := p.available
	path := p.binaryPath
	p.mu.RUnlock()
	if !avail || path == "" {
		return nil, fmt.Errorf("%w: bwrap not available", ErrProviderUnavailable)
	}
	if policy == nil {
		def := DefaultPolicy()
		policy = &def
	}
	bwCfg := parseBubblewrapConfig(cfg)
	// 若 policy 指定了 Timeout 且 cfg 未指定，则用 policy.Timeout。
	if bwCfg.Timeout == 0 && policy.Timeout > 0 {
		bwCfg.Timeout = policy.Timeout
	}
	// 若 FilesystemPolicy 给出 Mounts，迁移到 BindRO/BindRW。
	for _, m := range policy.FilesystemAccess.Mounts {
		me := MountEntry{Source: m.Source, Destination: m.Destination, ReadOnly: m.ReadOnly}
		if m.ReadOnly {
			bwCfg.BindRO = append(bwCfg.BindRO, me)
		} else {
			bwCfg.BindRW = append(bwCfg.BindRW, me)
		}
	}
	// 默认开启 UnshareAll（如果未显式关闭且未提供自定义 mount）。
	// 用户可通过 cfg["unshare_all"]=false 关闭。
	return &BubblewrapHandle{
		binary: path,
		config: bwCfg,
		policy: policy,
		status: StatusCreated,
	}, nil
}

// 编译期断言：BubblewrapProvider 满足 Provider 接口。
var _ Provider = (*BubblewrapProvider)(nil)

// BubblewrapHandle 是一个 Bubblewrap 沙箱实例。
//
// bwrap 命令的构建在 Execute 时完成（而非 Start），以便多次 Execute 可使用不同 argv。
// Start 主要用于初始化临时目录（如 tmpfs 的工作目录需要预先在宿主上准备）。
type BubblewrapHandle struct {
	mu     sync.Mutex
	binary string
	config BubblewrapConfig
	policy *ExecutionPolicy
	status HandleStatus
	// preparedHostPath 是宿主侧预创建的临时目录，作为某些 bind mount 的源。
	// Stop 时会清理。
	preparedHostPath string
}

// Start 切换到 Running 状态。若配置中存在 BindRW 的源目录不存在，则尝试创建。
func (h *BubblewrapHandle) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.status == StatusRunning {
		return nil
	}
	// 预创建 RW bind 源目录。
	for _, m := range h.config.BindRW {
		if m.Source == "" {
			continue
		}
		if err := os.MkdirAll(m.Source, 0o755); err != nil {
			h.status = StatusError
			return fmt.Errorf("bubblewrap: prepare bind source %q: %w", m.Source, err)
		}
	}
	h.status = StatusRunning
	return nil
}

// buildArgv 根据 config 构造 bwrap 命令的 argv 前缀（不含 user command）。
//
// 注意：此方法只自动挂载 /proc、/dev（以及配置的 tmpfs）。
// /usr、/bin、/lib、/lib64 等基础路径需要用户在 config.BindRO 中显式配置，例如：
//
//	config.BindRO = []MountEntry{
//		{Source: "/usr", Destination: "/usr"},
//		{Source: "/bin", Destination: "/bin"},
//		{Source: "/lib", Destination: "/lib"},
//	}
//
// 测试验证参见 TestBubblewrapIntegrationEcho 和 TestBubblewrapIntegrationReadOnly。
func (h *BubblewrapHandle) buildArgv(userArgv []string) []string {
	args := []string{}
	if h.config.UnshareAll {
		args = append(args, "--unshare-all")
		if h.config.ShareNet {
			args = append(args, "--share-net")
		}
	} else {
		// 未 unshare-all：默认至少 unshare user + pid 以提供基本隔离。
		args = append(args, "--unshare-user", "--unshare-pid")
	}
	if h.config.DieWithParent {
		args = append(args, "--die-with-parent")
	}
	if h.config.NewSession {
		args = append(args, "--new-session")
	}
	if h.config.Clearenv {
		args = append(args, "--clearenv")
	}
	// 只读 bind。
	for _, m := range h.config.BindRO {
		if m.Source == "" || m.Destination == "" {
			continue
		}
		args = append(args, "--ro-bind", m.Source, m.Destination)
	}
	// 读写 bind。
	for _, m := range h.config.BindRW {
		if m.Source == "" || m.Destination == "" {
			continue
		}
		args = append(args, "--bind", m.Source, m.Destination)
	}
	// tmpfs。
	for path, size := range h.config.Tmpfs {
		if path == "" {
			continue
		}
		if size > 0 {
			args = append(args, "--tmpfs", path, "--size", fmt.Sprintf("%d", size))
		} else {
			args = append(args, "--tmpfs", path)
		}
	}
	// 默认 proc/dev：若用户未在 BindRO 中显式给出，则挂载最小集合。
	if !h.hasMountDest("/proc") {
		args = append(args, "--proc", "/proc")
	}
	if !h.hasMountDest("/dev") {
		args = append(args, "--dev", "/dev")
	}
	// 追加 user command。
	args = append(args, userArgv...)
	return args
}

// hasMountDest 判断用户配置中是否已包含目标路径的挂载。
func (h *BubblewrapHandle) hasMountDest(dest string) bool {
	for _, m := range h.config.BindRO {
		if m.Destination == dest {
			return true
		}
	}
	for _, m := range h.config.BindRW {
		if m.Destination == dest {
			return true
		}
	}
	return false
}

// Execute 在 bwrap 沙箱中执行命令。
//
// 调用者必须先调用 Start。每次 Execute 都会启动新的 bwrap 进程，
// 因为 bwrap 是 "exec into sandbox" 模型而非持久会话。
func (h *BubblewrapHandle) Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error) {
	if cmd == nil || len(cmd.Argv) == 0 {
		return nil, fmt.Errorf("bubblewrap: command is nil or empty")
	}
	h.mu.Lock()
	running := h.status == StatusRunning
	binary := h.binary
	policy := h.policy
	timeout := h.config.Timeout
	h.mu.Unlock()
	if !running {
		return nil, fmt.Errorf("%w: bubblewrap handle not running", ErrHandleNotRunning)
	}
	// AllowedCommands 白名单（与 trusted_local 一致）。
	if policy != nil && len(policy.AllowedCommands) > 0 {
		allowed := false
		for _, c := range policy.AllowedCommands {
			if c == cmd.Argv[0] {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("%w: %q", ErrCommandNotAllowed, cmd.Argv[0])
		}
	}
	// 构建 bwrap argv。
	userArgv := make([]string, len(cmd.Argv))
	copy(userArgv, cmd.Argv)
	fullArgv := h.buildArgv(userArgv)

	// Timeout：cfg.Timeout 优先，其次 policy.Timeout，最后 30s 默认。
	execTimeout := timeout
	if execTimeout <= 0 && policy != nil && policy.Timeout > 0 {
		execTimeout = policy.Timeout
	}
	if execTimeout <= 0 {
		execTimeout = 30 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	c := exec.CommandContext(execCtx, binary, fullArgv...)
	if cmd.Workdir != "" {
		c.Dir = cmd.Workdir
	}
	// bwrap --clearenv 会清空环境变量；未设置时透传 cmd.Env 或宿主 env。
	if len(cmd.Env) > 0 {
		c.Env = cmd.Env
	}
	if cmd.Stdin != nil {
		c.Stdin = cmd.Stdin
	}
	var stdout, stderr strings.Builder
	c.Stdout = &stdout
	c.Stderr = &stderr
	start := time.Now()
	runErr := c.Run()
	duration := time.Since(start)

	result := &ExecutionResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}
	if c.ProcessState != nil {
		result.ExitCode = c.ProcessState.ExitCode()
	}
	if runErr != nil {
		if execCtx.Err() != nil {
			return result, fmt.Errorf("%w: %v", execCtx.Err(), runErr)
		}
		// 非零退出码不算 Execute 错误。
		return result, nil
	}
	return result, nil
}

// Stop 释放资源。当前实现仅切换状态，bwrap 子进程已随命令结束而退出。
func (h *BubblewrapHandle) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.status != StatusRunning {
		return nil
	}
	// 若启动时创建了临时宿主目录，则清理。
	if h.preparedHostPath != "" {
		_ = os.RemoveAll(h.preparedHostPath)
		h.preparedHostPath = ""
	}
	h.status = StatusStopped
	return nil
}

// Status 返回当前生命周期状态。
func (h *BubblewrapHandle) Status() HandleStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

// 编译期断言：BubblewrapHandle 满足 Handle 接口。
var _ Handle = (*BubblewrapHandle)(nil)

// RegisterBubblewrapProvider 将 BubblewrapProvider 注册到 Manager（若 bwrap 可用）。
func RegisterBubblewrapProvider(m *Manager) error {
	provider := NewBubblewrapProvider()
	avail, err := provider.InspectAvailability()
	if err != nil {
		return err
	}
	if !avail.Available {
		return ErrProviderUnavailable
	}
	return m.Register(provider)
}
