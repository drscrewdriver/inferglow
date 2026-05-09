package schema

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// Version 表示 TriggerFlow 配置的语义化版本号。
//
// 配置版本号格式为 "trigger_flow/v<MAJOR>"（历史格式）或
// "trigger_flow/v<MAJOR>.<MINOR>.<PATCH>"（SemVer 格式）。
// 当 Minor/Patch 缺失时按 0 处理。
type Version struct {
	Major int
	Minor int
	Patch int
}

// ParseVersion 解析版本字符串。
//
// 支持以下格式：
//   - "trigger_flow/v1"       → {1, 0, 0}
//   - "trigger_flow/v1.2"     → {1, 2, 0}
//   - "trigger_flow/v1.2.3"   → {1, 2, 3}
//   - "v1.2.3"                → {1, 2, 3}（无前缀也接受）
//
// 不支持的格式返回 error。
func ParseVersion(s string) (Version, error) {
	raw := strings.TrimSpace(s)
	// 剥离 "trigger_flow/" 前缀（大小写敏感）。
	if strings.HasPrefix(raw, "trigger_flow/") {
		raw = strings.TrimPrefix(raw, "trigger_flow/")
	}
	// 剥离前导 "v"。
	raw = strings.TrimPrefix(raw, "v")
	if raw == "" {
		return Version{}, fmt.Errorf("empty version: %q", s)
	}
	parts := strings.Split(raw, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return Version{}, fmt.Errorf("invalid version format: %q", s)
	}
	v := Version{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("invalid version segment %q in %q: %w", p, s, err)
		}
		switch i {
		case 0:
			v.Major = n
		case 1:
			v.Minor = n
		case 2:
			v.Patch = n
		}
	}
	return v, nil
}

// String 渲染回 "trigger_flow/vMAJOR.MINOR.PATCH" 形式。
// 当 Minor=Patch=0 时省略，保持与历史格式 "trigger_flow/v1" 兼容。
func (v Version) String() string {
	if v.Minor == 0 && v.Patch == 0 {
		return fmt.Sprintf("trigger_flow/v%d", v.Major)
	}
	if v.Patch == 0 {
		return fmt.Sprintf("trigger_flow/v%d.%d", v.Major, v.Minor)
	}
	return fmt.Sprintf("trigger_flow/v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare 返回 -1 / 0 / +1，表示 v 小于 / 等于 / 大于 other。
func (v Version) Compare(other Version) int {
	switch {
	case v.Major != other.Major:
		return cmpInt(v.Major, other.Major)
	case v.Minor != other.Minor:
		return cmpInt(v.Minor, other.Minor)
	case v.Patch != other.Patch:
		return cmpInt(v.Patch, other.Patch)
	default:
		return 0
	}
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// Less 便捷包装 Compare，用于排序。
func (v Version) Less(other Version) bool { return v.Compare(other) < 0 }

// ============================================================================
// 迁移器注册表
// ============================================================================

// MigrateFn 将一个配置字典从某个版本升级到下一版本。
// 实现者应：
//   - 读取并更新 "version" 字段
//   - 对其他字段做必要的转换（重命名、删除、补默认值）
//   - 不修改原始字典（应返回新字典或深拷贝）
type MigrateFn func(config map[string]any) (map[string]any, error)

type migratorEntry struct {
	From string
	To   string
	Fn   MigrateFn
}

var (
	migratorMu       sync.RWMutex
	migratorRegistry = map[string]migratorEntry{}
)

// RegisterMigrator 注册一条 from → to 的迁移路径。
// 重复注册同一 from 版本会覆盖旧条目。
// from 与 to 应为 "trigger_flow/vN" 形式；from 必须可被 ParseVersion 解析。
func RegisterMigrator(from, to string, fn MigrateFn) {
	if fn == nil {
		return
	}
	migratorMu.Lock()
	defer migratorMu.Unlock()
	migratorRegistry[from] = migratorEntry{From: from, To: to, Fn: fn}
}

// ResetMigratorsForTest 清空注册表，仅供测试使用。
func ResetMigratorsForTest() {
	migratorMu.Lock()
	defer migratorMu.Unlock()
	migratorRegistry = map[string]migratorEntry{}
}

// MigratorTarget 返回已注册的 from 版本对应的目标版本。
// 若 from 没有注册迁移器，返回空字符串。
func MigratorTarget(from string) string {
	migratorMu.RLock()
	defer migratorMu.RUnlock()
	if e, ok := migratorRegistry[from]; ok {
		return e.To
	}
	return ""
}

// SupportedSources 返回所有已注册迁移器的源版本，按版本号升序排序。
func SupportedSources() []string {
	migratorMu.RLock()
	defer migratorMu.RUnlock()
	out := make([]string, 0, len(migratorRegistry))
	for k := range migratorRegistry {
		out = append(out, k)
	}
	sortVersionsAsc(out)
	return out
}

// MigrateDict 将 config 从其当前版本逐级迁移到 target 版本。
//
// 行为：
//   - 读取 config["version"]（必须为 string）
//   - 若已等于 target，原样返回（浅拷贝）
//   - 沿 from→to 链逐级调用注册的迁移器
//   - 若链路中断（某一步未注册），返回错误
//   - 防止循环：最多迁移 16 步
//
// 浅拷贝输入以避免副作用；迁移器内部应自行深拷贝需要修改的字段。
func MigrateDict(config map[string]any, target string) (map[string]any, error) {
	if config == nil {
		return nil, fmt.Errorf("config is nil")
	}
	current, _ := config["version"].(string)
	if current == "" {
		return nil, fmt.Errorf("config has no version field")
	}
	if current == target {
		// 已是目标版本，浅拷贝返回。
		out := make(map[string]any, len(config))
		for k, v := range config {
			out[k] = v
		}
		return out, nil
	}

	// 沿链路迁移，最多 16 步防止循环注册。
	cur := shallowCopyMap(config)
	for step := 0; step < 16; step++ {
		v, _ := cur["version"].(string)
		if v == target {
			return cur, nil
		}
		migratorMu.RLock()
		entry, ok := migratorRegistry[v]
		migratorMu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("no migrator registered from version %q (target %q)", v, target)
		}
		next, err := entry.Fn(cur)
		if err != nil {
			return nil, fmt.Errorf("migrate %q → %q: %w", entry.From, entry.To, err)
		}
		if next == nil {
			return nil, fmt.Errorf("migrator %q → %q returned nil", entry.From, entry.To)
		}
		// 强制写入目标版本，防止迁移器忘记更新。
		next["version"] = entry.To
		cur = next
	}
	return nil, fmt.Errorf("migration chain exceeded 16 steps (possible cycle) reaching target %q", target)
}

// MigrateDefinition 包装 MigrateDict：将 def 序列化为字典，迁移后再反序列化。
// 迁移后的 Definition.Version 会等于 target。
func MigrateDefinition(def *TriggerFlowDefinition, target string) (*TriggerFlowDefinition, error) {
	if def == nil {
		return nil, fmt.Errorf("definition is nil")
	}
	// 用 DefinitionSerializer 的 ToDict / FromDict 完成序列化往返。
	s := NewDefinitionSerializer()
	dict, err := s.ToDict(def, false, def.Name)
	if err != nil {
		return nil, fmt.Errorf("serialize definition: %w", err)
	}
	migrated, err := MigrateDict(dict, target)
	if err != nil {
		return nil, err
	}
	// FromDict 默认只接受 FLOW_CONFIG_VERSION。若 target 不是当前版本，
	// 直接构造 Definition 绕过版本校验。
	out := &TriggerFlowDefinition{
		Version: target,
		Name:    def.Name,
		Signals: make(map[string]string),
	}
	if rawName, _ := migrated["name"].(string); rawName != "" {
		out.Name = rawName
	}
	if rawOps, ok := migrated["operators"]; ok && rawOps != nil {
		opsList, err := toAnySlice(rawOps)
		if err != nil {
			return nil, fmt.Errorf("operators: %w", err)
		}
		for i, raw := range opsList {
			opMap, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("operators[%d]: expected map, got %T", i, raw)
			}
			op, err := operatorFromDict(opMap)
			if err != nil {
				return nil, fmt.Errorf("operators[%d]: %w", i, err)
			}
			out.AddOperator(op)
		}
	}
	if rawSignals, ok := migrated["signals"]; ok && rawSignals != nil {
		if sigMap, ok := rawSignals.(map[string]any); ok {
			for k, v := range sigMap {
				if vs, ok := v.(string); ok {
					out.Signals[k] = vs
				}
			}
		}
	}
	return out, nil
}

// shallowCopyMap 浅拷贝 map。
func shallowCopyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// sortVersionsAsc 按版本号升序排序字符串切片（原地）。
func sortVersionsAsc(versions []string) {
	parsed := make([]Version, len(versions))
	for i, s := range versions {
		parsed[i], _ = ParseVersion(s) // 解析失败保持零值，排到最前
	}
	// 简单插入排序，版本列表通常很短。
	for i := 1; i < len(versions); i++ {
		j := i
		for j > 0 && parsed[j].Less(parsed[j-1]) {
			parsed[j], parsed[j-1] = parsed[j-1], parsed[j]
			versions[j], versions[j-1] = versions[j-1], versions[j]
			j--
		}
	}
}
