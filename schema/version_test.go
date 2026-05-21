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

package schema

import (
	"errors"
	"strings"
	"testing"
)

// ============================================================================
// Version 解析与比较
// ============================================================================

func TestParseVersionFormats(t *testing.T) {
	cases := []struct {
		in   string
		want Version
	}{
		{"trigger_flow/v1", Version{1, 0, 0}},
		{"trigger_flow/v1.2", Version{1, 2, 0}},
		{"trigger_flow/v1.2.3", Version{1, 2, 3}},
		{"v2", Version{2, 0, 0}},
		{"v2.3", Version{2, 3, 0}},
		{"v2.3.4", Version{2, 3, 4}},
		{"trigger_flow/v10.20.30", Version{10, 20, 30}},
	}
	for _, c := range cases {
		got, err := ParseVersion(c.in)
		if err != nil {
			t.Errorf("ParseVersion(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseVersion(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestParseVersionInvalid(t *testing.T) {
	bad := []string{
		"",
		"trigger_flow/",
		"v",
		"vabc",
		"v1.x",
		"v1.2.3.4",
		"trigger_flow/v-1",
	}
	for _, s := range bad {
		if _, err := ParseVersion(s); err == nil {
			t.Errorf("ParseVersion(%q) expected error, got nil", s)
		}
	}
}

func TestVersionStringRoundTrip(t *testing.T) {
	cases := []struct {
		v    Version
		want string
	}{
		{Version{1, 0, 0}, "trigger_flow/v1"},
		{Version{1, 2, 0}, "trigger_flow/v1.2"},
		{Version{1, 2, 3}, "trigger_flow/v1.2.3"},
	}
	for _, c := range cases {
		got := c.v.String()
		if got != c.want {
			t.Errorf("Version{%d,%d,%d}.String() = %q, want %q", c.v.Major, c.v.Minor, c.v.Patch, got, c.want)
		}
		// 回环：String() 结果应能被 ParseVersion 解析回相同值。
		parsed, err := ParseVersion(got)
		if err != nil {
			t.Errorf("ParseVersion(%q) error: %v", got, err)
			continue
		}
		if parsed != c.v {
			t.Errorf("round-trip mismatch: got %+v, want %+v", parsed, c.v)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		a, b Version
		want int
	}{
		{Version{1, 0, 0}, Version{1, 0, 0}, 0},
		{Version{1, 0, 0}, Version{2, 0, 0}, -1},
		{Version{2, 0, 0}, Version{1, 0, 0}, 1},
		{Version{1, 2, 0}, Version{1, 3, 0}, -1},
		{Version{1, 2, 3}, Version{1, 2, 4}, -1},
		{Version{1, 2, 4}, Version{1, 2, 3}, 1},
	}
	for _, c := range cases {
		got := c.a.Compare(c.b)
		if got != c.want {
			t.Errorf("%+v.Compare(%+v) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// ============================================================================
// 迁移器注册表
// ============================================================================

// makeV1Config 构造一个最小可用的 v1 配置字典。
func makeV1Config(name string) map[string]any {
	return map[string]any{
		"version": "trigger_flow/v1",
		"name":    name,
		"operators": []any{
			map[string]any{"kind": "chunk", "name": "op1"},
		},
	}
}

func TestRegisterAndLookupMigrator(t *testing.T) {
	ResetMigratorsForTest()
	defer ResetMigratorsForTest()

	RegisterMigrator("trigger_flow/v1", "trigger_flow/v2", func(c map[string]any) (map[string]any, error) {
		out := shallowCopyMap(c)
		out["version"] = "trigger_flow/v2"
		out["migrated_from_v1"] = true
		return out, nil
	})

	if tgt := MigratorTarget("trigger_flow/v1"); tgt != "trigger_flow/v2" {
		t.Errorf("MigratorTarget = %q, want %q", tgt, "trigger_flow/v2")
	}
	if tgt := MigratorTarget("trigger_flow/v999"); tgt != "" {
		t.Errorf("MigratorTarget for unregistered = %q, want empty", tgt)
	}

	sources := SupportedSources()
	if len(sources) != 1 || sources[0] != "trigger_flow/v1" {
		t.Errorf("SupportedSources = %v, want [trigger_flow/v1]", sources)
	}
}

func TestRegisterNilMigratorIgnored(t *testing.T) {
	ResetMigratorsForTest()
	defer ResetMigratorsForTest()

	RegisterMigrator("trigger_flow/v1", "trigger_flow/v2", nil)
	if tgt := MigratorTarget("trigger_flow/v1"); tgt != "" {
		t.Errorf("nil migrator should be ignored, got target %q", tgt)
	}
}

// ============================================================================
// MigrateDict
// ============================================================================

func TestMigrateDictAlreadyAtTarget(t *testing.T) {
	ResetMigratorsForTest()
	defer ResetMigratorsForTest()

	cfg := makeV1Config("flow1")
	out, err := MigrateDict(cfg, "trigger_flow/v1")
	if err != nil {
		t.Fatalf("MigrateDict error: %v", err)
	}
	// 应返回浅拷贝，互不影响。
	out["name"] = "changed"
	if cfg["name"] == "changed" {
		t.Error("MigrateDict should return a copy, not mutate input")
	}
}

func TestMigrateDictSingleStep(t *testing.T) {
	ResetMigratorsForTest()
	defer ResetMigratorsForTest()

	RegisterMigrator("trigger_flow/v1", "trigger_flow/v2", func(c map[string]any) (map[string]any, error) {
		out := shallowCopyMap(c)
		// 模拟 v2 引入 schema_version 字段。
		out["schema_version"] = 2
		return out, nil
	})

	cfg := makeV1Config("flow1")
	out, err := MigrateDict(cfg, "trigger_flow/v2")
	if err != nil {
		t.Fatalf("MigrateDict error: %v", err)
	}
	if v, _ := out["version"].(string); v != "trigger_flow/v2" {
		t.Errorf("version = %v, want trigger_flow/v2", out["version"])
	}
	if sv, _ := out["schema_version"].(int); sv != 2 {
		t.Errorf("schema_version = %v, want 2", out["schema_version"])
	}
	// 原始配置不变。
	if v, _ := cfg["version"].(string); v != "trigger_flow/v1" {
		t.Errorf("input mutated: version = %v, want trigger_flow/v1", cfg["version"])
	}
}

func TestMigrateDictMultiStep(t *testing.T) {
	ResetMigratorsForTest()
	defer ResetMigratorsForTest()

	// v1 → v2：重命名 kind "chunk" → "chunk_v2"
	RegisterMigrator("trigger_flow/v1", "trigger_flow/v2", func(c map[string]any) (map[string]any, error) {
		out := shallowCopyMap(c)
		ops, _ := out["operators"].([]any)
		newOps := make([]any, len(ops))
		for i, raw := range ops {
			m, _ := raw.(map[string]any)
			cp := shallowCopyMap(m)
			if cp["kind"] == "chunk" {
				cp["kind"] = "chunk_v2"
			}
			newOps[i] = cp
		}
		out["operators"] = newOps
		return out, nil
	})
	// v2 → v3：增加 metadata 字段
	RegisterMigrator("trigger_flow/v2", "trigger_flow/v3", func(c map[string]any) (map[string]any, error) {
		out := shallowCopyMap(c)
		out["metadata"] = map[string]any{"upgraded_to": "v3"}
		return out, nil
	})

	cfg := makeV1Config("flow1")
	out, err := MigrateDict(cfg, "trigger_flow/v3")
	if err != nil {
		t.Fatalf("MigrateDict error: %v", err)
	}
	if v, _ := out["version"].(string); v != "trigger_flow/v3" {
		t.Errorf("version = %v, want trigger_flow/v3", out["version"])
	}
	ops, _ := out["operators"].([]any)
	if len(ops) != 1 {
		t.Fatalf("operators len = %d, want 1", len(ops))
	}
	op0, _ := ops[0].(map[string]any)
	if op0["kind"] != "chunk_v2" {
		t.Errorf("operator kind = %v, want chunk_v2", op0["kind"])
	}
	if _, ok := out["metadata"]; !ok {
		t.Error("metadata field not added by v2→v3 migrator")
	}
}

func TestMigrateDictNoPathToTarget(t *testing.T) {
	ResetMigratorsForTest()
	defer ResetMigratorsForTest()

	// 只注册 v1→v2，但请求迁移到 v3。
	RegisterMigrator("trigger_flow/v1", "trigger_flow/v2", func(c map[string]any) (map[string]any, error) {
		out := shallowCopyMap(c)
		return out, nil
	})

	cfg := makeV1Config("flow1")
	_, err := MigrateDict(cfg, "trigger_flow/v3")
	if err == nil {
		t.Fatal("expected error when no migrator from v2 to v3, got nil")
	}
	if !strings.Contains(err.Error(), "no migrator registered") {
		t.Errorf("error should mention missing migrator, got: %v", err)
	}
}

func TestMigrateDictMigratorError(t *testing.T) {
	ResetMigratorsForTest()
	defer ResetMigratorsForTest()

	wantErr := errors.New("synthetic failure")
	RegisterMigrator("trigger_flow/v1", "trigger_flow/v2", func(c map[string]any) (map[string]any, error) {
		return nil, wantErr
	})

	cfg := makeV1Config("flow1")
	_, err := MigrateDict(cfg, "trigger_flow/v2")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error should wrap synthetic failure, got: %v", err)
	}
}

func TestMigrateDictCycleProtection(t *testing.T) {
	ResetMigratorsForTest()
	defer ResetMigratorsForTest()

	// 故意构造循环：v1→v2 和 v2→v1。
	RegisterMigrator("trigger_flow/v1", "trigger_flow/v2", func(c map[string]any) (map[string]any, error) {
		return shallowCopyMap(c), nil
	})
	RegisterMigrator("trigger_flow/v2", "trigger_flow/v1", func(c map[string]any) (map[string]any, error) {
		return shallowCopyMap(c), nil
	})

	cfg := makeV1Config("flow1")
	_, err := MigrateDict(cfg, "trigger_flow/v3")
	if err == nil {
		t.Fatal("expected cycle protection error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeded 16 steps") {
		t.Errorf("error should mention step limit, got: %v", err)
	}
}

func TestMigrateDictNilConfig(t *testing.T) {
	_, err := MigrateDict(nil, "trigger_flow/v1")
	if err == nil {
		t.Fatal("expected error for nil config, got nil")
	}
}

func TestMigrateDictMissingVersion(t *testing.T) {
	cfg := map[string]any{"name": "no_version"}
	_, err := MigrateDict(cfg, "trigger_flow/v1")
	if err == nil {
		t.Fatal("expected error for missing version, got nil")
	}
}

func TestMigrateDictForcesVersionField(t *testing.T) {
	ResetMigratorsForTest()
	defer ResetMigratorsForTest()

	// 迁移器"忘记"更新 version 字段，MigrateDict 应强制写入。
	RegisterMigrator("trigger_flow/v1", "trigger_flow/v2", func(c map[string]any) (map[string]any, error) {
		out := shallowCopyMap(c)
		// 故意不更新 version。
		return out, nil
	})

	cfg := makeV1Config("flow1")
	out, err := MigrateDict(cfg, "trigger_flow/v2")
	if err != nil {
		t.Fatalf("MigrateDict error: %v", err)
	}
	if v, _ := out["version"].(string); v != "trigger_flow/v2" {
		t.Errorf("version = %v, want trigger_flow/v2 (should be force-set)", out["version"])
	}
}

// ============================================================================
// MigrateDefinition
// ============================================================================

func TestMigrateDefinitionSingleStep(t *testing.T) {
	ResetMigratorsForTest()
	defer ResetMigratorsForTest()

	RegisterMigrator("trigger_flow/v1", "trigger_flow/v2", func(c map[string]any) (map[string]any, error) {
		out := shallowCopyMap(c)
		// 模拟 v2：重命名 kind "chunk" → "chunk_v2"
		ops, _ := out["operators"].([]any)
		newOps := make([]any, len(ops))
		for i, raw := range ops {
			m, _ := raw.(map[string]any)
			cp := shallowCopyMap(m)
			if cp["kind"] == "chunk" {
				cp["kind"] = "chunk_v2"
			}
			newOps[i] = cp
		}
		out["operators"] = newOps
		return out, nil
	})

	def := NewTriggerFlowDefinition("flow1")
	def.AddOperator(&FlowConfigOperator{Kind: "chunk", Name: "op1"})

	migrated, err := MigrateDefinition(def, "trigger_flow/v2")
	if err != nil {
		t.Fatalf("MigrateDefinition error: %v", err)
	}
	if migrated.Version != "trigger_flow/v2" {
		t.Errorf("Version = %q, want trigger_flow/v2", migrated.Version)
	}
	if migrated.Name != "flow1" {
		t.Errorf("Name = %q, want flow1", migrated.Name)
	}
	if len(migrated.Operators) != 1 {
		t.Fatalf("Operators len = %d, want 1", len(migrated.Operators))
	}
	if migrated.Operators[0].Kind != "chunk_v2" {
		t.Errorf("Operator[0].Kind = %q, want chunk_v2", migrated.Operators[0].Kind)
	}
	// 原始 def 不变。
	if def.Version != "trigger_flow/v1" {
		t.Errorf("input def mutated: Version = %q", def.Version)
	}
	if def.Operators[0].Kind != "chunk" {
		t.Errorf("input def mutated: Operator[0].Kind = %q", def.Operators[0].Kind)
	}
}

func TestMigrateDefinitionNilDef(t *testing.T) {
	_, err := MigrateDefinition(nil, "trigger_flow/v2")
	if err == nil {
		t.Fatal("expected error for nil def, got nil")
	}
}

func TestSupportedSourcesSortedByVersion(t *testing.T) {
	ResetMigratorsForTest()
	defer ResetMigratorsForTest()

	// 乱序注册多个版本。
	RegisterMigrator("trigger_flow/v3", "trigger_flow/v4", func(c map[string]any) (map[string]any, error) { return c, nil })
	RegisterMigrator("trigger_flow/v1", "trigger_flow/v2", func(c map[string]any) (map[string]any, error) { return c, nil })
	RegisterMigrator("trigger_flow/v2", "trigger_flow/v3", func(c map[string]any) (map[string]any, error) { return c, nil })

	got := SupportedSources()
	want := []string{"trigger_flow/v1", "trigger_flow/v2", "trigger_flow/v3"}
	if len(got) != len(want) {
		t.Fatalf("SupportedSources len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SupportedSources[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
