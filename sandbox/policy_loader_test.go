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
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package sandbox

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// writePolicyFile 将 yaml 内容写入临时文件并返回其路径。
func writePolicyFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入临时策略文件失败: %v", err)
	}
	return path
}

// assertZeroPolicy 断言加载失败时不产生部分生效的策略（必须返回零值策略）。
func assertZeroPolicy(t *testing.T, got ExecutionPolicy) {
	t.Helper()
	if !reflect.DeepEqual(got, ExecutionPolicy{}) {
		t.Fatalf("加载失败时应返回零值策略，实际得到: %+v", got)
	}
}

func TestLoadPolicyFromFileFullMapping(t *testing.T) {
	const yamlContent = `
sandbox_mode: docker
network:
  level: egress_only
  allow_internet: true
  allowed_hosts:
    - example.com
    - api.example.com
  allowed_ports:
    - 80
    - 443
filesystem:
  read_only_root: true
  mounts:
    - source: /data
      destination: /data
      read_only: true
  allowed_paths:
    - /tmp
    - /data
  denied_paths:
    - /etc/shadow
allowed_commands:
  - ls
  - cat
timeout: 45s
resource_limit:
  cpu_shares: 512
  memory_bytes: 268435456
  disk_bytes: 1073741824
  nproc: 64
`
	// 宽松基线：网络不超过 full、超时不超过 120s、路径白名单为空（不限制路径）
	baseline := ServerPolicyBaseline{
		NetworkAccess: NetworkAccessFull,
		Timeout:       120 * time.Second,
	}
	got, err := LoadPolicyFromFile(writePolicyFile(t, yamlContent), baseline)
	if err != nil {
		t.Fatalf("LoadPolicyFromFile() 意外失败: %v", err)
	}

	if got.SandboxMode != ModeDocker {
		t.Errorf("SandboxMode = %q, want %q", got.SandboxMode, ModeDocker)
	}
	// 网络级别：配置 egress_only 与基线 full 取更严者 -> egress_only
	if got.NetworkAccess.Level != NetworkAccessEgressOnly {
		t.Errorf("NetworkAccess.Level = %q, want %q", got.NetworkAccess.Level, NetworkAccessEgressOnly)
	}
	// AllowInternet 仅在最终级别为 full 时保留
	if got.NetworkAccess.AllowInternet {
		t.Error("NetworkAccess.AllowInternet = true, want false（最终级别 egress_only 非 full）")
	}
	wantHosts := []string{"example.com", "api.example.com"}
	if !reflect.DeepEqual(got.NetworkAccess.AllowedHosts, wantHosts) {
		t.Errorf("NetworkAccess.AllowedHosts = %v, want %v", got.NetworkAccess.AllowedHosts, wantHosts)
	}
	wantPorts := []int{80, 443}
	if !reflect.DeepEqual(got.NetworkAccess.AllowedPorts, wantPorts) {
		t.Errorf("NetworkAccess.AllowedPorts = %v, want %v", got.NetworkAccess.AllowedPorts, wantPorts)
	}
	if !got.FilesystemAccess.ReadOnlyRoot {
		t.Error("FilesystemAccess.ReadOnlyRoot = false, want true")
	}
	wantMounts := []MountEntry{{Source: "/data", Destination: "/data", ReadOnly: true}}
	if !reflect.DeepEqual(got.FilesystemAccess.Mounts, wantMounts) {
		t.Errorf("FilesystemAccess.Mounts = %+v, want %+v", got.FilesystemAccess.Mounts, wantMounts)
	}
	wantAllowedPaths := []string{"/tmp", "/data"}
	if !reflect.DeepEqual(got.FilesystemAccess.AllowedPaths, wantAllowedPaths) {
		t.Errorf("FilesystemAccess.AllowedPaths = %v, want %v", got.FilesystemAccess.AllowedPaths, wantAllowedPaths)
	}
	wantDeniedPaths := []string{"/etc/shadow"}
	if !reflect.DeepEqual(got.FilesystemAccess.DeniedPaths, wantDeniedPaths) {
		t.Errorf("FilesystemAccess.DeniedPaths = %v, want %v", got.FilesystemAccess.DeniedPaths, wantDeniedPaths)
	}
	wantCommands := []string{"ls", "cat"}
	if !reflect.DeepEqual(got.AllowedCommands, wantCommands) {
		t.Errorf("AllowedCommands = %v, want %v", got.AllowedCommands, wantCommands)
	}
	// 超时取 min(45s, 120s)
	if got.Timeout != 45*time.Second {
		t.Errorf("Timeout = %v, want 45s", got.Timeout)
	}
	wantResources := ResourceLimit{CPUShares: 512, MemoryBytes: 268435456, DiskBytes: 1073741824, NPROC: 64}
	if got.ResourceLimit != wantResources {
		t.Errorf("ResourceLimit = %+v, want %+v", got.ResourceLimit, wantResources)
	}
}

func TestLoadPolicyBaselineNoneTightensNetwork(t *testing.T) {
	const yamlContent = `
network:
  level: egress_only
  allow_internet: true
  allowed_hosts:
    - example.com
  allowed_ports:
    - 80
    - 443
timeout: 120s
`
	// DefaultDenyBaseline 的网络级别为 none：配置声明的任何网络面都必须被收紧
	got, err := LoadPolicyFromFile(writePolicyFile(t, yamlContent), DefaultDenyBaseline())
	if err != nil {
		t.Fatalf("LoadPolicyFromFile() 意外失败: %v", err)
	}
	if got.NetworkAccess.Level != NetworkAccessNone {
		t.Errorf("NetworkAccess.Level = %q, want %q（不得高于基线）", got.NetworkAccess.Level, NetworkAccessNone)
	}
	if got.NetworkAccess.AllowInternet {
		t.Error("NetworkAccess.AllowInternet = true, want false（基线为 none）")
	}
	if len(got.NetworkAccess.AllowedHosts) != 0 {
		t.Errorf("NetworkAccess.AllowedHosts = %v, want 空（基线为 none）", got.NetworkAccess.AllowedHosts)
	}
	if len(got.NetworkAccess.AllowedPorts) != 0 {
		t.Errorf("NetworkAccess.AllowedPorts = %v, want 空（基线为 none）", got.NetworkAccess.AllowedPorts)
	}
	// 超时取 min(120s, 基线 30s) = 30s
	if got.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", got.Timeout)
	}
}

func TestLoadPolicyConfigTighterThanBaseline(t *testing.T) {
	const yamlContent = `
network:
  level: none
filesystem:
  allowed_paths:
    - /tmp
    - /etc
timeout: 45s
`
	// 宽松基线：full 网络、路径白名单 [/tmp /data]、超时 60s；配置更严时应取更严者
	baseline := ServerPolicyBaseline{
		NetworkAccess: NetworkAccessFull,
		PathAllowlist: []string{"/tmp", "/data"},
		Timeout:       60 * time.Second,
	}
	got, err := LoadPolicyFromFile(writePolicyFile(t, yamlContent), baseline)
	if err != nil {
		t.Fatalf("LoadPolicyFromFile() 意外失败: %v", err)
	}
	// 配置 none 比基线 full 更严 -> 取 none
	if got.NetworkAccess.Level != NetworkAccessNone {
		t.Errorf("NetworkAccess.Level = %q, want %q（取更严者）", got.NetworkAccess.Level, NetworkAccessNone)
	}
	// 路径白名单求交：[/tmp /etc] 交 [/tmp /data] = [/tmp]
	wantPaths := []string{"/tmp"}
	if !reflect.DeepEqual(got.FilesystemAccess.AllowedPaths, wantPaths) {
		t.Errorf("FilesystemAccess.AllowedPaths = %v, want %v（与基线求交）", got.FilesystemAccess.AllowedPaths, wantPaths)
	}
	// 超时取 min(45s, 60s) = 45s
	if got.Timeout != 45*time.Second {
		t.Errorf("Timeout = %v, want 45s（取更严者）", got.Timeout)
	}
}

func TestLoadPolicyZeroBaselineTreatedAsNone(t *testing.T) {
	const yamlContent = `
network:
  level: egress_only
  allowed_hosts:
    - example.com
`
	// 零值基线的 NetworkAccess 为空，按 NetworkAccessNone 处理（deny-by-default）
	got, err := LoadPolicyFromFile(writePolicyFile(t, yamlContent), ServerPolicyBaseline{})
	if err != nil {
		t.Fatalf("LoadPolicyFromFile() 意外失败: %v", err)
	}
	if got.NetworkAccess.Level != NetworkAccessNone {
		t.Errorf("NetworkAccess.Level = %q, want %q（空基线按 none 处理）", got.NetworkAccess.Level, NetworkAccessNone)
	}
	if len(got.NetworkAccess.AllowedHosts) != 0 {
		t.Errorf("NetworkAccess.AllowedHosts = %v, want 空（空基线按 none 处理）", got.NetworkAccess.AllowedHosts)
	}
}

func TestLoadPolicyInvalidFiles(t *testing.T) {
	cases := []struct {
		name        string
		yamlContent string
		wantErrSubs []string
	}{
		{
			name:        "未知嵌套字段",
			yamlContent: "network:\n  level: full\n  acces_level: full\n",
			wantErrSubs: []string{"unknown field network.acces_level"},
		},
		{
			name:        "未知顶层字段",
			yamlContent: "timeout: 10s\nunknown_top: 1\n",
			wantErrSubs: []string{"unknown field unknown_top"},
		},
		{
			name:        "未知挂载字段",
			yamlContent: "filesystem:\n  mounts:\n    - source: /a\n      destination: /b\n      writable: true\n",
			wantErrSubs: []string{"unknown field filesystem.mounts.writable"},
		},
		{
			name:        "非法网络级别枚举",
			yamlContent: "network:\n  level: half\n",
			wantErrSubs: []string{"network.level", "half"},
		},
		{
			name:        "非法沙箱模式枚举",
			yamlContent: "sandbox_mode: hyperspace\n",
			wantErrSubs: []string{"sandbox_mode", "hyperspace"},
		},
		{
			name:        "非法超时格式",
			yamlContent: "timeout: 10x\n",
			wantErrSubs: []string{"timeout"},
		},
		{
			name:        "非法端口号",
			yamlContent: "network:\n  level: full\n  allowed_ports:\n    - 0\n",
			wantErrSubs: []string{"network.allowed_ports"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := LoadPolicyFromFile(writePolicyFile(t, c.yamlContent), DefaultDenyBaseline())
			if err == nil {
				t.Fatalf("LoadPolicyFromFile() 期望返回错误，实际成功: %+v", got)
			}
			for _, sub := range c.wantErrSubs {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("错误消息 %q 未包含期望子串 %q", err.Error(), sub)
				}
			}
			assertZeroPolicy(t, got)
		})
	}
}

func TestLoadPolicyEmptyFile(t *testing.T) {
	// 空文件应视为未声明任何配置：与基线求交后按 deny-by-default 收紧
	got, err := LoadPolicyFromFile(writePolicyFile(t, ""), DefaultDenyBaseline())
	if err != nil {
		t.Fatalf("LoadPolicyFromFile() 意外失败: %v", err)
	}
	if got.NetworkAccess.Level != NetworkAccessNone {
		t.Errorf("NetworkAccess.Level = %q, want %q", got.NetworkAccess.Level, NetworkAccessNone)
	}
	if got.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s（基线收紧）", got.Timeout)
	}
}

func TestLoadPolicyMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	got, err := LoadPolicyFromFile(path, DefaultDenyBaseline())
	if err == nil {
		t.Fatalf("LoadPolicyFromFile() 期望返回错误，实际成功: %+v", got)
	}
	assertZeroPolicy(t, got)
}
