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

//go:build ignore

// sandbox_compare sandbox 模式对比演示
//
// 对比 trusted_local 与 windows_runtime 三种后端的行为差异。
// 通过命令行参数选择模式：
//
//   go run -tags skip_docker_real sandbox_compare.go                      # 运行全部对比
//   go run -tags skip_docker_real sandbox_compare.go --mode trusted       # 仅 trusted_local
//   go run -tags skip_docker_real sandbox_compare.go --mode restricted    # 仅 RestrictedToken
//   go run -tags skip_docker_real sandbox_compare.go --mode appcontainer  # 仅 AppContainer
//   go run -tags skip_docker_real sandbox_compare.go --mode sandbox       # 仅 Windows Sandbox
//
// 核心差异：
//   trusted_local    = 完全信任，无沙箱限制（无白名单、无超时、无文件系统限制）
//   restricted       = 受限令牌，有沙箱限制（白名单、超时、文件系统限制）
//   appcontainer     = UWP 风格隔离，策略检查同 restricted，实际隔离由 OS AppContainer 提供
//   windows_sandbox  = VM 级别隔离，策略检查同 restricted，需要安装 Windows Sandbox 功能

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/inferglow/sandbox"
)

func main() {
	mode := flag.String("mode", "all", "sandbox mode: trusted | restricted | appcontainer | sandbox | all")
	flag.Parse()

	ctx := context.Background()

	fmt.Println("========================================")
	fmt.Println("  Sandbox 模式对比演示 (Windows)")
	fmt.Println("========================================")
	fmt.Println()

	// --- 可用性检查 ---
	fmt.Println("--- Provider 可用性 ---")
	trustedProvider := sandbox.NewTrustedLocalProvider()
	trustedAvail, _ := trustedProvider.InspectAvailability()
	fmt.Printf("  trusted_local:      Available=%v, Platform=%s\n", trustedAvail.Available, trustedAvail.Platform)

	windowsProvider := sandbox.NewWindowsRuntimeProvider()
	windowsAvail, _ := windowsProvider.InspectAvailability()
	fmt.Printf("  windows_runtime:    Available=%v, Platform=%s\n", windowsAvail.Available, windowsAvail.Platform)
	fmt.Println()

	// --- 定义测试策略 ---
	// trusted_local 策略：完全信任，无沙箱限制
	trustedPolicy := &sandbox.ExecutionPolicy{
		IsolationLevel: sandbox.LevelProcess,
	}

	// 受限模式策略：有沙箱限制（三种受限后端共用）
	restrictedPolicy := &sandbox.ExecutionPolicy{
		ResourceLimit: sandbox.ResourceLimit{
			MemoryBytes: 64 * 1024 * 1024, // 64MB
			NPROC:       5,
		},
		NetworkAccess: sandbox.NetworkPolicy{
			AllowInternet: false,
			AllowedHosts:  []string{},
		},
		FilesystemAccess: sandbox.FilesystemPolicy{
			ReadOnlyRoot: true,
			DeniedPaths:  []string{"C:\\Windows", "C:\\Program Files"},
		},
		AllowedCommands: []string{"echo", "dir", "whoami", "hostname", "cmd"},
		Timeout:         5 * time.Second,
		IsolationLevel:  sandbox.LevelProcess,
	}

	// 临时目录用于测试写入
	tmpDir := filepath.Join(os.TempDir(), "sandbox_compare_test")
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test_write.txt")

	// --- 测试用例 ---
	type testCase struct {
		name             string
		cmd              *sandbox.Command
		trustedPolicy    *sandbox.ExecutionPolicy
		restrictedPolicy *sandbox.ExecutionPolicy
		desc             string
	}

	testCases := []testCase{
		{
			name:             "任意命令 (powershell)",
			cmd:              &sandbox.Command{Argv: []string{"powershell", "-Command", "echo test"}},
			trustedPolicy:    trustedPolicy,
			restrictedPolicy: restrictedPolicy,
			desc:             "powershell 不在白名单中，trusted_local 不限制",
		},
		{
			name:             "任意命令 (ping)",
			cmd:              &sandbox.Command{Argv: []string{"ping", "-n", "1", "127.0.0.1"}},
			trustedPolicy:    trustedPolicy,
			restrictedPolicy: restrictedPolicy,
			desc:             "ping 不在白名单中，trusted_local 不限制",
		},
		{
			name:             "白名单命令 (echo)",
			cmd:              &sandbox.Command{Argv: []string{"echo", "hello sandbox"}},
			trustedPolicy:    trustedPolicy,
			restrictedPolicy: restrictedPolicy,
			desc:             "echo 在白名单中，所有模式都应成功",
		},
		{
			name:             "白名单命令 (whoami)",
			cmd:              &sandbox.Command{Argv: []string{"whoami"}},
			trustedPolicy:    trustedPolicy,
			restrictedPolicy: restrictedPolicy,
			desc:             "whoami 在白名单中，所有模式都应成功",
		},
		{
			name: "文件写入（临时目录）",
			cmd: &sandbox.Command{
				Argv: []string{"cmd", "/c", fmt.Sprintf("echo test > %s", testFile)},
			},
			trustedPolicy:    trustedPolicy,
			restrictedPolicy: restrictedPolicy,
			desc:             "写入临时目录，不在 DeniedPaths 中，所有模式都应成功",
		},
		{
			name: "文件写入（受保护路径 C:\\Windows\\Temp）",
			cmd: &sandbox.Command{
				Argv: []string{"cmd", "/c", "echo test > C:\\Windows\\Temp\\sandbox_test.txt"},
			},
			trustedPolicy:    trustedPolicy,
			restrictedPolicy: restrictedPolicy,
			desc:             "写入受保护路径，trusted_local 不限制，受限模式应被拒绝",
		},
		{
			name: "超时测试 (ping 30秒)",
			cmd: &sandbox.Command{
				Argv: []string{"ping", "-n", "30", "127.0.0.1"},
			},
			trustedPolicy: trustedPolicy,
			restrictedPolicy: &sandbox.ExecutionPolicy{
				Timeout:         2 * time.Second,
				AllowedCommands: []string{"ping"},
			},
			desc: "ping 持续 30 秒，trusted_local 无超时（手动 3 秒截断），受限模式 2 秒后超时",
		},
	}

	// --- 通用运行函数 ---
	type backendInfo struct {
		label            string
		provider         sandbox.Provider
		cfg              map[string]any
		policy           *sandbox.ExecutionPolicy
		useTrustedPolicy bool
	}

	runBackend := func(bi backendInfo) {
		fmt.Printf("========================================\n")
		fmt.Printf("  %s\n", bi.label)
		fmt.Printf("========================================\n\n")

		for i, tc := range testCases {
			fmt.Printf("  [%d] %s\n", i+1, tc.name)
			fmt.Printf("      描述: %s\n", tc.desc)

			pol := tc.restrictedPolicy
			if bi.useTrustedPolicy {
				pol = tc.trustedPolicy
			}

			handle, err := bi.provider.CreateHandle(bi.cfg, pol)
			if err != nil {
				fmt.Printf("      结果: SKIP - 创建 Handle 失败: %v\n\n", err)
				continue
			}

			if err := handle.Start(ctx); err != nil {
				fmt.Printf("      结果: SKIP - Start 失败: %v\n\n", err)
				continue
			}

			// trusted_local 对超时测试手动截断
			execCtx := ctx
			var cancel context.CancelFunc
			if bi.useTrustedPolicy && tc.name == "超时测试 (ping 30秒)" {
				execCtx, cancel = context.WithTimeout(ctx, 3*time.Second)
			}

			result, err := handle.Execute(execCtx, tc.cmd)
			_ = handle.Stop(ctx)
			if cancel != nil {
				cancel()
			}

			if err != nil {
				fmt.Printf("      结果: ERROR - %v\n", err)
			} else {
				stdout := result.Stdout
				if len(stdout) > 80 {
					stdout = stdout[:80] + "..."
				}
				fmt.Printf("      结果: OK (exit=%d, %.3fs)\n", result.ExitCode, result.Duration.Seconds())
				if stdout != "" {
					fmt.Printf("      输出: %s\n", stdout)
				}
			}
			fmt.Println()
		}
	}

	// --- 定义各后端 ---
	backends := map[string]backendInfo{
		"trusted": {
			label:            "模式: trusted_local (完全信任，无沙箱限制)",
			provider:         trustedProvider,
			cfg:              nil,
			policy:           trustedPolicy,
			useTrustedPolicy: true,
		},
		"restricted": {
			label:    "模式: windows_runtime / RestrictedToken (受限令牌)",
			provider: windowsProvider,
			cfg: map[string]any{
				"backend":           int(sandbox.BackendRestrictedToken),
				"sandbox_directory": tmpDir,
				"network_isolation": true,
			},
			useTrustedPolicy: false,
		},
		"appcontainer": {
			label:    "模式: windows_runtime / AppContainer (UWP 风格隔离)",
			provider: windowsProvider,
			cfg: map[string]any{
				"backend":           int(sandbox.BackendAppContainer),
				"sandbox_directory": tmpDir,
				"network_isolation": true,
			},
			useTrustedPolicy: false,
		},
		"sandbox": {
			label:    "模式: windows_runtime / Windows Sandbox (VM 级别隔离)",
			provider: windowsProvider,
			cfg: map[string]any{
				"backend":           int(sandbox.BackendWindowsSandbox),
				"sandbox_directory": tmpDir,
				"network_isolation": true,
			},
			useTrustedPolicy: false,
		},
	}

	// --- 按模式运行 ---
	order := []string{"trusted", "restricted", "appcontainer", "sandbox"}

	switch *mode {
	case "all":
		for _, name := range order {
			runBackend(backends[name])
			fmt.Println()
		}
	case "trusted", "restricted", "appcontainer", "sandbox":
		runBackend(backends[*mode])
	default:
		fmt.Printf("  未知模式: %s (可选: trusted | restricted | appcontainer | sandbox | all)\n", *mode)
		return
	}

	// --- 差异总结 ---
	fmt.Println("========================================")
	fmt.Println("  差异总结")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("  四种模式的策略执行对比:")
	fmt.Println()
	fmt.Println("  ┌─────────────┬──────────┬────────────┬────────────┬──────────────┬────────────┐")
	fmt.Println("  │   模式       │ 命令白名单 │ 超时控制    │ 文件系统    │ 网络          │ 隔离级别    │")
	fmt.Println("  ├─────────────┼──────────┼────────────┼────────────┼──────────────┼────────────┤")
	fmt.Println("  │ trusted     │ 不限制    │ 不限制      │ 不限制      │ 不限制        │ 无隔离      │")
	fmt.Println("  │ restricted  │ 强制执行   │ 强制执行    │ DeniedPaths │ 当前未强制    │ 进程级      │")
	fmt.Println("  │ appcontainer│ 强制执行   │ 强制执行    │ DeniedPaths │ 当前未强制    │ 应用级(UWP) │")
	fmt.Println("  │ sandbox     │ 强制执行   │ 强制执行    │ DeniedPaths │ 当前未强制    │ VM 级      │")
	fmt.Println("  └─────────────┴──────────┴────────────┴────────────┴──────────────┴────────────┘")
	fmt.Println()
	fmt.Println("  说明:")
	fmt.Println("  - trusted_local: 完全信任，适合开发环境和可信代码")
	fmt.Println("  - RestrictedToken: 受限令牌，移除高权限特权，适合不可信代码")
	fmt.Println("  - AppContainer: UWP 风格沙箱，文件系统/注册表/设备受限，适合第三方插件")
	fmt.Println("  - Windows Sandbox: VM 级别隔离，最强但需安装功能，适合恶意代码分析")
	fmt.Println()
	fmt.Println("  注意: 三种受限后端的策略检查（白名单、DeniedPaths、超时）在应用层执行。")
	fmt.Println("  完整的 OS 级别隔离（restricted token、AppContainer profile、Sandbox VM）")
	fmt.Println("  需要实现对应的 Windows API（DuplicateTokenEx、StartAppContainerOperation 等）。")
}
