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
	"os/exec"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ============================================================================
// Landlock 常量与 syscall 包装
//
// Landlock 是 Linux 内核 5.13+ 提供的用户态 LSM（Linux Security Module），
// 允许非特权进程对自己及子进程施加文件系统访问限制。限制一旦应用不可撤销。
//
// 三个核心 syscall（x86_64/arm64 上号均为 444/445/446，其他架构通过
// syscall.SYS_LANDLOCK_* 获取）：
//   - landlock_create_ruleset(attr, size, flags) -> ruleset_fd
//   - landlock_add_rule(ruleset_fd, type, rule_attr, flags) -> 0/-1
//   - landlock_restrict_self(ruleset_fd, flags) -> 0/-1
//
// 我们使用 raw syscall 而非 golang.org/x/sys/unix，以避免为 sandbox 模块
// 显式新增依赖（unix 是 indirect）。
// ============================================================================

const (
	// landlock_create_ruleset flags
	landlockCreateRulesetVersion = 1 << 0
)

// LandlockAccessFS 是 Landlock 文件系统访问位掩码（合并 ABI v1~v3 的位）。
type LandlockAccessFS uint64

// LandlockAccessFS bit flags for Landlock filesystem access, grouped by ABI
// version: v1 (Linux 5.13+), v2 adds Refer (Linux 5.19+), v3 adds Truncate
// (Linux 6.2+).
const (
	LandlockAccessFSExecute    LandlockAccessFS = 1 << 0
	LandlockAccessFSWriteFile  LandlockAccessFS = 1 << 1
	LandlockAccessFSReadFile   LandlockAccessFS = 1 << 2
	LandlockAccessFSReadDir    LandlockAccessFS = 1 << 3
	LandlockAccessFSRemoveDir  LandlockAccessFS = 1 << 4
	LandlockAccessFSRemoveFile LandlockAccessFS = 1 << 5
	LandlockAccessFSMakeChar   LandlockAccessFS = 1 << 6
	LandlockAccessFSMakeDir    LandlockAccessFS = 1 << 7
	LandlockAccessFSMakeReg    LandlockAccessFS = 1 << 8
	LandlockAccessFSMakeSock   LandlockAccessFS = 1 << 9
	LandlockAccessFSMakeFifo   LandlockAccessFS = 1 << 10
	LandlockAccessFSMakeBlock  LandlockAccessFS = 1 << 11
	LandlockAccessFSMakeSym    LandlockAccessFS = 1 << 12
	LandlockAccessFSRefer      LandlockAccessFS = 1 << 13
	LandlockAccessFSTruncate   LandlockAccessFS = 1 << 14

	// LandlockAccessFSAllWrite 汇总所有写操作位。
	LandlockAccessFSAllWrite = LandlockAccessFSWriteFile |
		LandlockAccessFSRemoveDir |
		LandlockAccessFSRemoveFile |
		LandlockAccessFSMakeChar |
		LandlockAccessFSMakeDir |
		LandlockAccessFSMakeReg |
		LandlockAccessFSMakeSock |
		LandlockAccessFSMakeFifo |
		LandlockAccessFSMakeBlock |
		LandlockAccessFSMakeSym |
		LandlockAccessFSRefer |
		LandlockAccessFSTruncate

	// LandlockAccessFSAllRead 汇总所有读操作位。
	LandlockAccessFSAllRead = LandlockAccessFSExecute | LandlockAccessFSReadFile | LandlockAccessFSReadDir
)

// landlockRulesetAttr 对应 struct landlock_ruleset_attr。
type landlockRulesetAttr struct {
	HandledAccessFS uint64
}

// landlockPathBeneathAttr 对应 struct landlock_path_beneath_attr。
type landlockPathBeneathAttr struct {
	AllowedAccess uint64
	ParentFD      int32
	_             [4]byte // 显式 padding 对齐 8 字节
}

// landlockCreateRuleset 调用 landlock_create_ruleset(2)。
// 返回 ruleset 文件描述符或错误。
func landlockCreateRuleset(handled LandlockAccessFS) (int, error) {
	attr := landlockRulesetAttr{HandledAccessFS: uint64(handled)}
	r1, _, e := unix.Syscall(
		unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0, // flags=0：实际创建 ruleset；非 probe 模式
	)
	if e != 0 {
		return -1, e
	}
	return int(r1), nil
}

// landlockCreateRulesetProbe 仅探测当前内核支持的最大 ABI 版本，
// 不创建实际的 ruleset。返回值：>=1 表示支持，0 表示不支持。
func landlockCreateRulesetProbe() int {
	// 传入 attr=nil + size=0 + flags=LANDLOCK_CREATE_RULESET_VERSION
	// 返回值是当前内核支持的最高 ABI 版本号。
	r1, _, _ := unix.Syscall(
		unix.SYS_LANDLOCK_CREATE_RULESET,
		0, // nil
		0, // size = 0
		landlockCreateRulesetVersion,
	)
	// 错误时 r1 是 -errno；正数表示版本。
	if int(r1) < 0 {
		return 0
	}
	return int(r1)
}

// landlockAddPathRule 向 ruleset_fd 添加一条 path_beneath 规则。
func landlockAddPathRule(rulesetFD int, path string, allowed LandlockAccessFS) error {
	// 打开路径获取 parent_fd（O_PATH | O_CLOEXEC）。
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("landlock: open %q: %w", path, err)
	}
	defer unix.Close(fd)

	rule := landlockPathBeneathAttr{
		AllowedAccess: uint64(allowed),
		ParentFD:      int32(fd),
	}
	_, _, e := unix.Syscall6(
		unix.SYS_LANDLOCK_ADD_RULE,
		uintptr(rulesetFD),
		1, // LANDLOCK_RULE_PATH_BENEATH
		uintptr(unsafe.Pointer(&rule)),
		0, // flags=0
		0,
		0,
	)
	if e != 0 {
		return fmt.Errorf("landlock: add rule for %q: %w", path, e)
	}
	return nil
}

// landlockRestrictSelf 将 ruleset 应用到当前进程。
// 应用后，本进程及其所有子进程的文件系统访问将被限制为 ruleset 中的允许集。
// **此调用不可逆**。
func landlockRestrictSelf(rulesetFD int) error {
	_, _, e := unix.Syscall(
		unix.SYS_LANDLOCK_RESTRICT_SELF,
		uintptr(rulesetFD),
		0,
		0,
	)
	if e != 0 {
		return e
	}
	return nil
}

// ============================================================================
// LandlockConfig
// ============================================================================

// LandlockConfig 描述 Landlock 沙箱的文件系统策略。
type LandlockConfig struct {
	// AllowedReadDirs 允许读取（含列目录）的目录（递归）。
	AllowedReadDirs []string
	// AllowedWriteDirs 允许读写的目录（递归）。
	AllowedWriteDirs []string
	// AllowedReadFiles 允许读取的单个文件。
	AllowedReadFiles []string
	// AllowedWriteFiles 允许读写的单个文件。
	AllowedWriteFiles []string
	// HandledAccessFS 显式指定要拦截的访问位（0 表示自动计算）。
	// 用于高级用户精确控制 ABI 兼容性。
	HandledAccessFS LandlockAccessFS
	// ABIVersion 期望使用的 ABI 版本（0 = 自动协商最高可用）。
	ABIVersion int
}

// parseLandlockConfig 从 cfg map[string]any 解析 LandlockConfig。
//
// 已知字段：
//   - "read_dirs"  ([]string 或 []any of string)
//   - "write_dirs" (同上)
//   - "read_files" (同上)
//   - "write_files"(同上)
//   - "abi_version" (int)
func parseLandlockConfig(cfg map[string]any) LandlockConfig {
	out := LandlockConfig{}
	if cfg == nil {
		return out
	}
	out.AllowedReadDirs = parseStringSlice(cfg["read_dirs"])
	out.AllowedWriteDirs = parseStringSlice(cfg["write_dirs"])
	out.AllowedReadFiles = parseStringSlice(cfg["read_files"])
	out.AllowedWriteFiles = parseStringSlice(cfg["write_files"])
	if v, ok := cfg["abi_version"].(int); ok {
		out.ABIVersion = v
	}
	return out
}

// parseStringSlice 接受 []string 或 []any of string，统一返回 []string。
func parseStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// ============================================================================
// Provider
// ============================================================================

// LandlockProvider 是基于 Linux Landlock LSM 的文件系统沙箱 Provider。
//
// 注意：Landlock 的限制一旦应用即不可撤销，会同时影响当前进程和子进程。
// 因此 LandlockHandle 是 **一次性** 的：调用 Execute 后，当前进程将永久受限。
// 调用方应在使用 LandlockHandle 后退出当前进程，或仅将其用于隔离 fork 出去的子进程。
type LandlockProvider struct {
	mu        sync.RWMutex
	available bool
	abiVer    int
	probeErr  error
}

// NewLandlockProvider 探测 Landlock 支持并构造 Provider。
func NewLandlockProvider() *LandlockProvider {
	p := &LandlockProvider{}
	p.probe()
	return p
}

func (p *LandlockProvider) probe() {
	p.mu.Lock()
	defer p.mu.Unlock()
	ver := landlockCreateRulesetProbe()
	if ver <= 0 {
		p.available = false
		p.abiVer = 0
		p.probeErr = fmt.Errorf("landlock not supported by kernel (need Linux 5.13+)")
		return
	}
	p.abiVer = ver
	// 真正验证可用性：尝试创建一个空的 ruleset，仅处理内核支持的访问位。
	supported := landlockSupportedAccess(ver)
	fd, err := landlockCreateRuleset(supported)
	if err != nil {
		p.available = false
		p.probeErr = fmt.Errorf("landlock_create_ruleset failed: %w", err)
		return
	}
	unix.Close(fd)
	p.available = true
}

// landlockSupportedAccess 根据内核 ABI 版本返回支持的访问位掩码。
// 高版本 ABI 是低版本的超集。
func landlockSupportedAccess(abiVersion int) LandlockAccessFS {
	mask := LandlockAccessFSAllRead | LandlockAccessFSWriteFile |
		LandlockAccessFSRemoveDir | LandlockAccessFSRemoveFile |
		LandlockAccessFSMakeChar | LandlockAccessFSMakeDir |
		LandlockAccessFSMakeReg | LandlockAccessFSMakeSock |
		LandlockAccessFSMakeFifo | LandlockAccessFSMakeBlock |
		LandlockAccessFSMakeSym
	if abiVersion >= 2 {
		mask |= LandlockAccessFSRefer
	}
	if abiVersion >= 3 {
		mask |= LandlockAccessFSTruncate
	}
	return mask
}

// Name 返回 "landlock"。
func (p *LandlockProvider) Name() string { return "landlock" }

// Kind 返回 "local"。
func (p *LandlockProvider) Kind() string { return "local" }

// InspectAvailability 报告 Landlock 是否可用。
func (p *LandlockProvider) InspectAvailability() (*AvailabilityResult, error) {
	p.probe()
	p.mu.RLock()
	defer p.mu.RUnlock()
	res := &AvailabilityResult{
		Available:  p.available,
		Platform:   string(OSLinux),
		BinaryPath: "(kernel)",
	}
	if p.available {
		res.Version = fmt.Sprintf("ABI v%d", p.abiVer)
	} else if p.probeErr != nil {
		res.ErrorMessage = p.probeErr.Error()
	}
	return res, nil
}

// CreateHandle 根据 cfg 和 policy 创建 LandlockHandle。
func (p *LandlockProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	p.mu.RLock()
	avail := p.available
	abiVer := p.abiVer
	p.mu.RUnlock()
	if !avail {
		return nil, fmt.Errorf("%w: landlock not available", ErrProviderUnavailable)
	}
	if policy == nil {
		def := DefaultPolicy()
		policy = &def
	}
	llCfg := parseLandlockConfig(cfg)
	// 若 policy 给出 AllowedPaths，迁移到 Landlock 规则。
	if len(policy.FilesystemAccess.AllowedPaths) > 0 {
		llCfg.AllowedReadDirs = append(llCfg.AllowedReadDirs, policy.FilesystemAccess.AllowedPaths...)
	}
	// ABI 版本协商：用户可指定更低版本，但不能超过内核支持。
	if llCfg.ABIVersion <= 0 {
		llCfg.ABIVersion = abiVer
	} else if llCfg.ABIVersion > abiVer {
		llCfg.ABIVersion = abiVer
	}
	// 根据 ABI 限制 handled 位（v1 不支持 Refer / Truncate）。
	if llCfg.ABIVersion < 2 {
		llCfg.HandledAccessFS &^= LandlockAccessFSRefer
	}
	if llCfg.ABIVersion < 3 {
		llCfg.HandledAccessFS &^= LandlockAccessFSTruncate
	}
	return &LandlockHandle{
		config: llCfg,
		policy: policy,
		status: StatusCreated,
	}, nil
}

// 编译期断言
var _ Provider = (*LandlockProvider)(nil)

// ============================================================================
// Handle
// ============================================================================

// LandlockHandle 是 Landlock 沙箱实例。
//
// **一次性约束**：调用 Execute 后，当前 Go 进程会应用 Landlock 限制，
// 此限制不可撤销。Stop 之后再次 Execute 会失败。
//
// 适用场景：
//   - main() 中：配置好 Handle，Execute 一次后退出。
//   - 子进程模式：在 fork 后的进程中使用，父进程不受影响。
//
// 不适用场景：
//   - 长生命周期的服务进程内反复创建/销毁沙箱。
//   - 需要在沙箱结束后继续访问受限路径的代码。
type LandlockHandle struct {
	mu        sync.Mutex
	config    LandlockConfig
	policy    *ExecutionPolicy
	status    HandleStatus
	rulesetFD int
	consumed  bool // 是否已应用过 landlock_restrict_self
}

// Start 准备 ruleset 并添加规则，但不应用 restrict_self。
// restrict_self 推迟到 Execute，以便 Start 失败不影响进程。
func (h *LandlockHandle) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.status == StatusRunning {
		return nil
	}
	// 计算要 handled 的访问位。
	handled := h.config.HandledAccessFS
	if handled == 0 {
		// 默认处理内核支持的所有访问位。
		handled = landlockSupportedAccess(h.config.ABIVersion)
	} else {
		// 与内核支持位取交集，避免 EINVAL。
		handled &= landlockSupportedAccess(h.config.ABIVersion)
	}
	fd, err := landlockCreateRuleset(handled)
	if err != nil {
		h.status = StatusError
		return fmt.Errorf("landlock: create ruleset: %w", err)
	}
	// 计算实际可用的读/写位（不能超出 handled）。
	readAccess := LandlockAccessFSAllRead & handled
	writeAccess := (LandlockAccessFSAllRead | LandlockAccessFSAllWrite) & handled
	// 添加读规则。
	for _, d := range h.config.AllowedReadDirs {
		if err := landlockAddPathRule(fd, d, readAccess); err != nil {
			unix.Close(fd)
			h.status = StatusError
			return fmt.Errorf("landlock: add read dir %q: %w", d, err)
		}
	}
	for _, f := range h.config.AllowedReadFiles {
		if err := landlockAddPathRule(fd, f, readAccess); err != nil {
			unix.Close(fd)
			h.status = StatusError
			return fmt.Errorf("landlock: add read file %q: %w", f, err)
		}
	}
	// 添加写规则。
	for _, d := range h.config.AllowedWriteDirs {
		if err := landlockAddPathRule(fd, d, writeAccess); err != nil {
			unix.Close(fd)
			h.status = StatusError
			return fmt.Errorf("landlock: add write dir %q: %w", d, err)
		}
	}
	for _, f := range h.config.AllowedWriteFiles {
		if err := landlockAddPathRule(fd, f, writeAccess); err != nil {
			unix.Close(fd)
			h.status = StatusError
			return fmt.Errorf("landlock: add write file %q: %w", f, err)
		}
	}
	h.rulesetFD = fd
	h.status = StatusRunning
	return nil
}

// Execute 在应用 Landlock 限制后执行命令。
// 调用者必须先调用 Start。
//
// 注意：第一次 Execute 后，landlock_restrict_self 已应用，
// 当前进程将永久受限于 ruleset。后续 Execute 调用仍可执行命令（继承限制），
// 但若命令需要访问 ruleset 外的路径将失败。
func (h *LandlockHandle) Execute(ctx context.Context, cmd *Command) (*ExecutionResult, error) {
	if cmd == nil || len(cmd.Argv) == 0 {
		return nil, fmt.Errorf("landlock: command is nil or empty")
	}
	h.mu.Lock()
	running := h.status == StatusRunning
	rulesetFD := h.rulesetFD
	policy := h.policy
	consumed := h.consumed
	h.mu.Unlock()
	if !running {
		return nil, fmt.Errorf("%w: landlock handle not running", ErrHandleNotRunning)
	}
	if rulesetFD <= 0 {
		return nil, fmt.Errorf("landlock: ruleset not prepared")
	}

	// AllowedCommands 白名单。
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

	// 首次 Execute：应用 landlock_restrict_self。
	if !consumed {
		if err := landlockRestrictSelf(rulesetFD); err != nil {
			return nil, fmt.Errorf("landlock: restrict_self: %w", err)
		}
		h.mu.Lock()
		h.consumed = true
		h.mu.Unlock()
	}

	// 应用 Timeout。
	execCtx := ctx
	var cancel context.CancelFunc
	if policy != nil && policy.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, policy.Timeout)
		defer cancel()
	} else {
		execCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	argv, err := buildArgv(cmd.Argv)
	if err != nil {
		return nil, err
	}
	c := exec.CommandContext(execCtx, argv[0], argv[1:]...)
	if cmd.Workdir != "" {
		c.Dir = cmd.Workdir
	}
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
		return result, nil
	}
	return result, nil
}

// Stop 关闭 ruleset fd（如果尚未关闭）。不撤销 landlock 限制。
func (h *LandlockHandle) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.status != StatusRunning {
		return nil
	}
	if h.rulesetFD > 0 {
		_ = unix.Close(h.rulesetFD)
		h.rulesetFD = 0
	}
	h.status = StatusStopped
	return nil
}

// Status 返回当前生命周期状态。
func (h *LandlockHandle) Status() HandleStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

// Consumed 报告 landlock_restrict_self 是否已被应用。
// 一旦为 true，当前进程已永久受限。
func (h *LandlockHandle) Consumed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.consumed
}

// 编译期断言
var _ Handle = (*LandlockHandle)(nil)

// RegisterLandlockProvider 将 LandlockProvider 注册到 Manager（若可用）。
func RegisterLandlockProvider(m *Manager) error {
	provider := NewLandlockProvider()
	avail, err := provider.InspectAvailability()
	if err != nil {
		return err
	}
	if !avail.Available {
		return ErrProviderUnavailable
	}
	return m.Register(provider)
}
