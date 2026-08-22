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
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// PolicyFile 是声明式策略 yaml 文件的结构，字段镜像 ExecutionPolicy 的可配置面。
// 未在文件中声明的字段保持零值（deny-by-default），加载后还需与服务器基线求交。
type PolicyFile struct {
	SandboxMode     string               `yaml:"sandbox_mode"`
	Network         policyFileNetwork    `yaml:"network"`
	Filesystem      policyFileFilesystem `yaml:"filesystem"`
	AllowedCommands []string             `yaml:"allowed_commands"`
	Timeout         string               `yaml:"timeout"`
	ResourceLimit   policyFileResources  `yaml:"resource_limit"`
}

// policyFileNetwork 镜像 NetworkPolicy 的可配置面。
type policyFileNetwork struct {
	Level         string   `yaml:"level"`
	AllowInternet bool     `yaml:"allow_internet"`
	AllowedHosts  []string `yaml:"allowed_hosts"`
	AllowedPorts  []int    `yaml:"allowed_ports"`
}

// policyFileFilesystem 镜像 FilesystemPolicy 的可配置面。
type policyFileFilesystem struct {
	ReadOnlyRoot bool              `yaml:"read_only_root"`
	Mounts       []policyFileMount `yaml:"mounts"`
	AllowedPaths []string          `yaml:"allowed_paths"`
	DeniedPaths  []string          `yaml:"denied_paths"`
}

// policyFileMount 镜像 MountEntry 的可配置面。
type policyFileMount struct {
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
	ReadOnly    bool   `yaml:"read_only"`
}

// policyFileResources 镜像 ResourceLimit 的可配置面。
type policyFileResources struct {
	CPUShares   int64 `yaml:"cpu_shares"`
	MemoryBytes int64 `yaml:"memory_bytes"`
	DiskBytes   int64 `yaml:"disk_bytes"`
	NPROC       int   `yaml:"nproc"`
}

// LoadPolicyFromFile 从 yaml 文件构建 ExecutionPolicy，并与服务器基线求交：
// 配置只能收紧、不能放宽基线（deny-by-default）。
// 任何未知字段或非法值都会导致加载失败，错误消息携带字段路径
// （如 "unknown field network.acces_level"），且不产生部分生效的策略。
func LoadPolicyFromFile(path string, baseline ServerPolicyBaseline) (ExecutionPolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ExecutionPolicy{}, fmt.Errorf("读取策略文件失败: %w", err)
	}
	return loadPolicyFromBytes(data, baseline)
}

// loadPolicyFromBytes 依次执行：未知字段校验、严格解码、枚举与取值校验、基线求交。
func loadPolicyFromBytes(data []byte, baseline ServerPolicyBaseline) (ExecutionPolicy, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return ExecutionPolicy{}, fmt.Errorf("解析策略文件失败: %w", err)
	}
	// 先基于 yaml 节点树校验未知字段，使错误消息携带完整字段路径
	// （如 "unknown field network.acces_level"）。
	if err := validateUnknownFields(&root, reflect.TypeOf(PolicyFile{}), ""); err != nil {
		return ExecutionPolicy{}, err
	}

	var pf PolicyFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // 严格解码兜底：任何未知字段都会导致失败
	if err := dec.Decode(&pf); err != nil && !errors.Is(err, io.EOF) {
		return ExecutionPolicy{}, fmt.Errorf("解析策略文件失败: %w", err)
	}

	ep, err := pf.toExecutionPolicy()
	if err != nil {
		return ExecutionPolicy{}, err
	}
	return intersectWithBaseline(ep, baseline), nil
}

// yamlFieldName 返回结构体字段对应的 yaml 名称，规则与 yaml.v3 一致：
// 优先取 yaml 标签名，无标签时使用小写字段名；不可导出或标签为 "-" 时忽略。
func yamlFieldName(f reflect.StructField) (string, bool) {
	if f.PkgPath != "" {
		return "", false
	}
	name := strings.ToLower(f.Name)
	if tag := f.Tag.Get("yaml"); tag != "" {
		parts := strings.Split(tag, ",")
		if parts[0] == "-" {
			return "", false
		}
		if parts[0] != "" {
			name = parts[0]
		}
	}
	return name, true
}

// findYAMLField 在结构体类型中查找 yaml 名称对应的字段。
func findYAMLField(t reflect.Type, name string) (reflect.StructField, bool) {
	for i := 0; i < t.NumField(); i++ {
		if n, ok := yamlFieldName(t.Field(i)); ok && n == name {
			return t.Field(i), true
		}
	}
	return reflect.StructField{}, false
}

// joinPath 拼接字段路径，如 joinPath("network", "level") = "network.level"。
func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

// validateUnknownFields 递归校验 yaml 节点中不存在于目标结构（t）之外的未知字段。
// path 为当前字段路径前缀，错误消息形如 "unknown field network.acces_level"。
// 标量节点的取值合法性交由后续严格解码与枚举校验处理。
func validateUnknownFields(node *yaml.Node, t reflect.Type, path string) error {
	if node == nil || node.Kind == 0 {
		return nil
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil
		}
		return validateUnknownFields(node.Content[0], t, path)
	case yaml.SequenceNode:
		if t.Kind() != reflect.Slice {
			return nil
		}
		for _, item := range node.Content {
			if err := validateUnknownFields(item, t.Elem(), path); err != nil {
				return err
			}
		}
		return nil
	case yaml.MappingNode:
		if t.Kind() != reflect.Struct {
			return nil
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode, valNode := node.Content[i], node.Content[i+1]
			field, ok := findYAMLField(t, keyNode.Value)
			if !ok {
				return fmt.Errorf("unknown field %s", joinPath(path, keyNode.Value))
			}
			if err := validateUnknownFields(valNode, field.Type, joinPath(path, keyNode.Value)); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

// toExecutionPolicy 将 PolicyFile 逐字段映射为 ExecutionPolicy 并校验枚举与取值。
// 任何字段非法都返回错误（错误消息含字段路径），不产生部分生效的策略。
// 未声明的 SandboxMode / IsolationLevel 保持零值，由调用方或后端自行决定默认值。
func (pf PolicyFile) toExecutionPolicy() (ExecutionPolicy, error) {
	var ep ExecutionPolicy

	mode := SandboxMode(pf.SandboxMode)
	switch mode {
	case "", ModeTrustedLocal, ModeLocal, ModeDocker, ModeGVisor, ModeAuto:
		ep.SandboxMode = mode
	default:
		return ExecutionPolicy{}, fmt.Errorf("invalid value %q for field sandbox_mode (must be one of: trusted_local, local, docker, gvisor, auto)", pf.SandboxMode)
	}

	level, err := parseNetworkAccessLevel(pf.Network.Level)
	if err != nil {
		return ExecutionPolicy{}, err
	}
	ep.NetworkAccess = NetworkPolicy{
		Level:         level,
		AllowInternet: pf.Network.AllowInternet,
		AllowedHosts:  append([]string(nil), pf.Network.AllowedHosts...),
		AllowedPorts:  append([]int(nil), pf.Network.AllowedPorts...),
	}
	for _, p := range ep.NetworkAccess.AllowedPorts {
		if p < 1 || p > 65535 {
			return ExecutionPolicy{}, fmt.Errorf("invalid value %d for field network.allowed_ports (must be in [1, 65535])", p)
		}
	}

	ep.FilesystemAccess = FilesystemPolicy{
		ReadOnlyRoot: pf.Filesystem.ReadOnlyRoot,
		AllowedPaths: append([]string(nil), pf.Filesystem.AllowedPaths...),
		DeniedPaths:  append([]string(nil), pf.Filesystem.DeniedPaths...),
	}
	for _, m := range pf.Filesystem.Mounts {
		ep.FilesystemAccess.Mounts = append(ep.FilesystemAccess.Mounts, MountEntry{
			Source:      m.Source,
			Destination: m.Destination,
			ReadOnly:    m.ReadOnly,
		})
	}
	ep.AllowedCommands = append([]string(nil), pf.AllowedCommands...)

	if pf.Timeout != "" {
		d, err := time.ParseDuration(pf.Timeout)
		if err != nil {
			return ExecutionPolicy{}, fmt.Errorf("invalid duration %q for field timeout", pf.Timeout)
		}
		if d < 0 {
			return ExecutionPolicy{}, fmt.Errorf("invalid value %q for field timeout (must be non-negative)", pf.Timeout)
		}
		ep.Timeout = d
	}

	ep.ResourceLimit = ResourceLimit{
		CPUShares:   pf.ResourceLimit.CPUShares,
		MemoryBytes: pf.ResourceLimit.MemoryBytes,
		DiskBytes:   pf.ResourceLimit.DiskBytes,
		NPROC:       pf.ResourceLimit.NPROC,
	}
	return ep, nil
}

// parseNetworkAccessLevel 校验并解析 network.level 的枚举值。
// 空字符串表示未声明，交由基线求交阶段按 deny-by-default 处理。
func parseNetworkAccessLevel(v string) (NetworkAccessLevel, error) {
	switch v {
	case "":
		return "", nil
	case string(NetworkAccessNone), string(NetworkAccessEgressOnly), string(NetworkAccessFull):
		return NetworkAccessLevel(v), nil
	default:
		return "", fmt.Errorf("invalid value %q for field network.level (must be one of: none, egress_only, full)", v)
	}
}

// intersectWithBaseline 将已映射的策略与服务器基线求交：配置只能收紧、不能放宽基线。
// 求交规则：
//   - 网络级别：基线 NetworkAccess 为空时按 NetworkAccessNone 处理（见
//     ServerPolicyBaseline 的注释语义），再用 MoreRestrictiveNetwork 取更严者；
//     最终级别为 none 时网络面完全关闭（hosts/ports/internet 全部清空）；
//     AllowInternet 仅在最终级别为 full 时才可能为 true。
//   - 超时：基线 Timeout 为 0 表示不限制，否则取两者较小值。
//   - 路径白名单：基线 PathAllowlist 为空表示不限制，否则取交集。
//   - AllowedCommands / DeniedPaths / Mounts / ResourceLimit 等基线未覆盖的面：
//     直接保留配置声明（deny-by-default：未声明即为零值）。
func intersectWithBaseline(ep ExecutionPolicy, baseline ServerPolicyBaseline) ExecutionPolicy {
	out := ep

	// 网络级别求交：空基线按 NetworkAccessNone 处理，再取更严者。
	baselineLevel := baseline.NetworkAccess
	if baselineLevel == "" {
		baselineLevel = NetworkAccessNone
	}
	level := MoreRestrictiveNetwork(ep.NetworkAccess.Level, baselineLevel)
	net := NetworkPolicy{Level: level}
	if level != NetworkAccessNone {
		// 级别高于 none 时保留配置声明的白名单（AllowedPorts 的收紧由级别封顶控制）。
		net.AllowedHosts = ep.NetworkAccess.AllowedHosts
		net.AllowedPorts = ep.NetworkAccess.AllowedPorts
		net.AllowInternet = ep.NetworkAccess.AllowInternet && level == NetworkAccessFull
	}
	// level == none 时 AllowInternet / AllowedHosts / AllowedPorts 保持零值。
	out.NetworkAccess = net

	// 超时求交：基线为 0 表示不限制，否则取较小值。
	if baseline.Timeout > 0 {
		if out.Timeout == 0 || out.Timeout > baseline.Timeout {
			out.Timeout = baseline.Timeout
		}
	}

	// 路径白名单求交：基线非空时配置只能在其子集内收紧。
	out.FilesystemAccess.AllowedPaths = intersectStrings(ep.FilesystemAccess.AllowedPaths, baseline.PathAllowlist)

	return out
}

// intersectStrings 返回两个切片的交集（按 config 的顺序保留）。
// baseline 为空表示不限制，直接返回 config；交集为空时返回 nil（最严格）。
func intersectStrings(config, baseline []string) []string {
	if len(baseline) == 0 {
		return config
	}
	allowed := make(map[string]struct{}, len(baseline))
	for _, s := range baseline {
		allowed[s] = struct{}{}
	}
	out := make([]string, 0, len(config))
	for _, s := range config {
		if _, ok := allowed[s]; ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
