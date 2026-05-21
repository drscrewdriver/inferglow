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

package sandbox

import (
	"fmt"
	"sync"
)

// LocalSandboxProvider 是 OS 原生沙箱的调度 Provider。
//
// 它本身不提供隔离能力，而是持有一条后端链（backends），按优先级依次
// 探测可用性，将 InspectAvailability / CreateHandle 委托给第一个可用后端。
//
// 默认后端链（DefaultLocalBackends）按 OS 选择：
//   - darwin:  [Seatbelt]
//   - linux:   [Bubblewrap, Landlock]（已实现，按优先级探测）
//   - windows: [WindowsRuntime]
//   - 其他:    []
//
// 调用者可通过 WithBackends 注入自定义后端链（测试或扩展用）。
type LocalSandboxProvider struct {
	mu       sync.RWMutex
	backends []Provider
}

// NewLocalSandboxProvider 创建 LocalSandboxProvider，使用当前 OS 的默认后端链。
func NewLocalSandboxProvider() *LocalSandboxProvider {
	return &LocalSandboxProvider{
		backends: DefaultLocalBackends(),
	}
}

// DefaultLocalBackends 返回当前 OS 的默认本地沙箱后端链（按优先级）。
//
// 平台特定后端在不支持的 OS 上为 stub（InspectAvailability 返回 false），
// 因此无需条件编译即可安全加入链中。
func DefaultLocalBackends() []Provider {
	switch DetectOS() {
	case OSDarwin:
		return []Provider{NewSeatbeltProvider()}
	case OSLinux:
		// Bubblewrap 优先（提供命名空间隔离），Landlock 作为 fallback（仅文件系统沙箱）。
		return []Provider{NewBubblewrapProvider(), NewLandlockProvider()}
	case OSWindows:
		return []Provider{NewWindowsRuntimeProvider()}
	default:
		return []Provider{}
	}
}

// Name 返回 "local"。
func (p *LocalSandboxProvider) Name() string { return "local" }

// Kind 返回 "local"。
func (p *LocalSandboxProvider) Kind() string { return "local" }

// WithBackends 替换后端链并返回 provider 自身（链式调用）。
// 传入空列表使 provider 始终不可用。nil 元素会被过滤掉。
// 此方法主要用于测试注入；生产代码应使用 NewLocalSandboxProvider。
func (p *LocalSandboxProvider) WithBackends(backends ...Provider) *LocalSandboxProvider {
	p.mu.Lock()
	defer p.mu.Unlock()
	filtered := make([]Provider, 0, len(backends))
	for _, b := range backends {
		if b != nil {
			filtered = append(filtered, b)
		}
	}
	p.backends = filtered
	return p
}

// Backends 返回当前后端链的副本。
func (p *LocalSandboxProvider) Backends() []Provider {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]Provider, len(p.backends))
	copy(out, p.backends)
	return out
}

// SelectBackend 遍历后端链，返回第一个 InspectAvailability 报告可用的后端。
// 若所有后端均不可用，返回 ErrProviderUnavailable。
func (p *LocalSandboxProvider) SelectBackend() (Provider, error) {
	p.mu.RLock()
	chain := make([]Provider, len(p.backends))
	copy(chain, p.backends)
	p.mu.RUnlock()

	var lastErr error
	var lastErrorMessage string
	for _, b := range chain {
		avail, err := b.InspectAvailability()
		if err != nil {
			lastErr = err
			continue
		}
		if avail != nil && avail.Available {
			return b, nil
		}
		if avail != nil && avail.ErrorMessage != "" {
			lastErrorMessage = avail.ErrorMessage
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("%w: no local backend available on %s (last error: %v)", ErrProviderUnavailable, DetectOS(), lastErr)
	}
	if lastErrorMessage != "" {
		return nil, fmt.Errorf("%w: no local backend available on %s: %s", ErrProviderUnavailable, DetectOS(), lastErrorMessage)
	}
	return nil, fmt.Errorf("%w: no local backend available on %s", ErrProviderUnavailable, DetectOS())
}

// InspectAvailability 报告后端链中是否有可用后端。
// 若有，返回该后端的 AvailabilityResult；否则返回 Available=false。
func (p *LocalSandboxProvider) InspectAvailability() (*AvailabilityResult, error) {
	selected, err := p.SelectBackend()
	if err != nil {
		return &AvailabilityResult{
			Available:    false,
			Platform:     string(DetectOS()),
			ErrorMessage: err.Error(),
		}, nil
	}
	// 委托给选中后端的可用性报告。
	return selected.InspectAvailability()
}

// CreateHandle 委托给第一个可用后端。
// 若无可用后端，返回 ErrProviderUnavailable。
func (p *LocalSandboxProvider) CreateHandle(cfg map[string]any, policy *ExecutionPolicy) (Handle, error) {
	selected, err := p.SelectBackend()
	if err != nil {
		return nil, err
	}
	return selected.CreateHandle(cfg, policy)
}

// 编译期断言：LocalSandboxProvider 满足 Provider 接口。
var _ Provider = (*LocalSandboxProvider)(nil)
