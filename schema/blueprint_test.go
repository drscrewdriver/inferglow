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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile 是测试辅助函数，将 bytes 写入指定路径。
func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

// ============================================================================
// TriggerFlowDefinition 测试
// ============================================================================

// Check: NewTriggerFlowDefinition 设置版本号和名称
func TestNewTriggerFlowDefinition(t *testing.T) {
	def := NewTriggerFlowDefinition("test_flow")
	if def.Version != FLOW_CONFIG_VERSION {
		t.Errorf("Version = %q, want %q", def.Version, FLOW_CONFIG_VERSION)
	}
	if def.Name != "test_flow" {
		t.Errorf("Name = %q, want test_flow", def.Name)
	}
	if len(def.Operators) != 0 {
		t.Errorf("expected empty operators, got %d", len(def.Operators))
	}
	if def.Signals == nil {
		t.Error("Signals should not be nil")
	}
}

// Check: TriggerFlowDefinition.Validate 校验完整性
func TestTriggerFlowDefinitionValidate(t *testing.T) {
	def := NewTriggerFlowDefinition("flow1")
	def.AddOperator(&FlowConfigOperator{Kind: "chunk", Name: "op1"})

	if err := def.Validate(); err != nil {
		t.Errorf("validate failed: %v", err)
	}

	// 空名称应失败
	bad := NewTriggerFlowDefinition("")
	if err := bad.Validate(); err == nil {
		t.Error("expected error for empty name")
	}

	// 错误版本应失败
	bad2 := NewTriggerFlowDefinition("flow2")
	bad2.Version = "v2/wrong"
	if err := bad2.Validate(); err == nil {
		t.Error("expected error for wrong version")
	}

	// 空 Kind 应失败
	bad3 := NewTriggerFlowDefinition("flow3")
	bad3.AddOperator(&FlowConfigOperator{Name: "op1"})
	if err := bad3.Validate(); err == nil {
		t.Error("expected error for empty kind")
	}

	// 重复名称应失败
	bad4 := NewTriggerFlowDefinition("flow4")
	bad4.AddOperator(&FlowConfigOperator{Kind: "chunk", Name: "op1"})
	bad4.AddOperator(&FlowConfigOperator{Kind: "chunk", Name: "op1"})
	if err := bad4.Validate(); err == nil {
		t.Error("expected error for duplicate name")
	}
}

// ============================================================================
// ToDict / FromDict 测试
// ============================================================================

// Check 2.5: ToDict 返回正确的字典
func TestDefinitionSerializerToDict(t *testing.T) {
	s := NewDefinitionSerializer()
	def := NewTriggerFlowDefinition("my_flow")
	def.AddOperator(&FlowConfigOperator{
		Kind:    "chunk",
		Name:    "step1",
		Input:   "START",
		Output:  "Chunk[step1]",
		Options: map[string]any{"key": "value"},
	})

	dict, err := s.ToDict(def, false, "my_flow")
	if err != nil {
		t.Fatalf("ToDict failed: %v", err)
	}
	if dict["version"] != FLOW_CONFIG_VERSION {
		t.Errorf("version = %v, want %v", dict["version"], FLOW_CONFIG_VERSION)
	}
	if dict["name"] != "my_flow" {
		t.Errorf("name = %v, want my_flow", dict["name"])
	}
	ops, ok := dict["operators"].([]any)
	if !ok {
		t.Fatalf("operators should be []any, got %T", dict["operators"])
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(ops))
	}
	opMap, ok := ops[0].(map[string]any)
	if !ok {
		t.Fatalf("operator[0] should be map, got %T", ops[0])
	}
	if opMap["kind"] != "chunk" {
		t.Errorf("kind = %v, want chunk", opMap["kind"])
	}
	if opMap["name"] != "step1" {
		t.Errorf("name = %v, want step1", opMap["name"])
	}
}

// Check: ToDict validate=true 校验失败返回 error
func TestDefinitionSerializerToDictValidate(t *testing.T) {
	s := NewDefinitionSerializer()
	def := &TriggerFlowDefinition{
		Version: "wrong_version",
		Name:    "bad_flow",
	}
	_, err := s.ToDict(def, true, "bad_flow")
	if err == nil {
		t.Error("expected error for invalid definition")
	}
}

// Check: FromDict 反序列化正确
func TestDefinitionSerializerFromDict(t *testing.T) {
	s := NewDefinitionSerializer()
	dict := map[string]any{
		"version": FLOW_CONFIG_VERSION,
		"name":    "test_flow",
		"operators": []any{
			map[string]any{
				"kind":   "chunk",
				"name":   "op1",
				"input":  "START",
				"output": "Chunk[op1]",
			},
		},
		"signals": map[string]any{
			"START": "handler1",
		},
	}
	def, err := s.FromDict(dict)
	if err != nil {
		t.Fatalf("FromDict failed: %v", err)
	}
	if def.Name != "test_flow" {
		t.Errorf("Name = %q, want test_flow", def.Name)
	}
	if len(def.Operators) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(def.Operators))
	}
	if def.Operators[0].Name != "op1" {
		t.Errorf("operator name = %q, want op1", def.Operators[0].Name)
	}
	if def.Operators[0].Input != "START" {
		t.Errorf("operator input = %q, want START", def.Operators[0].Input)
	}
	if def.Signals["START"] != "handler1" {
		t.Errorf("Signals[START] = %q, want handler1", def.Signals["START"])
	}
}

// Check: FromDict 版本不匹配返回 error
func TestDefinitionSerializerFromDictBadVersion(t *testing.T) {
	s := NewDefinitionSerializer()
	dict := map[string]any{
		"version": "wrong",
		"name":    "test_flow",
	}
	_, err := s.FromDict(dict)
	if err == nil {
		t.Error("expected error for wrong version")
	}
}

// Check: ToDict/FromDict 往返一致
func TestDefinitionSerializerRoundTripDict(t *testing.T) {
	s := NewDefinitionSerializer()
	original := NewTriggerFlowDefinition("round_trip")
	original.AddOperator(&FlowConfigOperator{
		Kind:    "chunk",
		Name:    "step1",
		Options: map[string]any{"k": "v"},
	})
	original.Signals["START"] = "h1"

	dict, err := s.ToDict(original, false, "round_trip")
	if err != nil {
		t.Fatalf("ToDict failed: %v", err)
	}
	restored, err := s.FromDict(dict)
	if err != nil {
		t.Fatalf("FromDict failed: %v", err)
	}
	if restored.Name != original.Name {
		t.Errorf("Name = %q, want %q", restored.Name, original.Name)
	}
	if len(restored.Operators) != len(original.Operators) {
		t.Fatalf("Operators length mismatch: %d vs %d", len(restored.Operators), len(original.Operators))
	}
	if restored.Operators[0].Name != original.Operators[0].Name {
		t.Errorf("Operator[0].Name = %q, want %q", restored.Operators[0].Name, original.Operators[0].Name)
	}
	if restored.Signals["START"] != original.Signals["START"] {
		t.Errorf("Signals[START] = %q, want %q", restored.Signals["START"], original.Signals["START"])
	}
}

// ============================================================================
// ToJSON / FromJSON 测试
// ============================================================================

// Check: ToJSON 生成合法 JSON
func TestDefinitionSerializerToJSON(t *testing.T) {
	s := NewDefinitionSerializer()
	def := NewTriggerFlowDefinition("json_flow")
	def.AddOperator(&FlowConfigOperator{Kind: "chunk", Name: "op1"})

	jsonStr, err := s.ToJSON(def, false, "json_flow")
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	// 验证是合法 JSON
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("ToJSON output is not valid JSON: %v", err)
	}
	if parsed["name"] != "json_flow" {
		t.Errorf("name = %v, want json_flow", parsed["name"])
	}
}

// Check: FromJSON 解析 JSON
func TestDefinitionSerializerFromJSON(t *testing.T) {
	s := NewDefinitionSerializer()
	jsonStr := `{
		"version": "trigger_flow/v1",
		"name": "from_json",
		"operators": [
			{"kind": "chunk", "name": "op1"}
		]
	}`
	def, err := s.FromJSON(jsonStr)
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}
	if def.Name != "from_json" {
		t.Errorf("Name = %q, want from_json", def.Name)
	}
	if len(def.Operators) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(def.Operators))
	}
}

// Check: ToJSON/FromJSON 往返一致
func TestDefinitionSerializerRoundTripJSON(t *testing.T) {
	s := NewDefinitionSerializer()
	original := NewTriggerFlowDefinition("rt_json")
	original.AddOperator(&FlowConfigOperator{
		Kind:    "batch_fanout",
		Name:    "fanout1",
		Input:   "START",
		Output:  "BatchItem[0]",
		Options: map[string]any{"expected_count": 3.0},
	})

	jsonStr, err := s.ToJSON(original, false, "rt_json")
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}
	restored, err := s.FromJSON(jsonStr)
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}
	if restored.Name != original.Name {
		t.Errorf("Name mismatch: %q vs %q", restored.Name, original.Name)
	}
	if len(restored.Operators) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(restored.Operators))
	}
	if restored.Operators[0].Kind != "batch_fanout" {
		t.Errorf("Kind = %q, want batch_fanout", restored.Operators[0].Kind)
	}
}

// ============================================================================
// ToYAML / FromYAML 测试
// ============================================================================

// Check: ToYAML 生成合法 YAML
func TestDefinitionSerializerToYAML(t *testing.T) {
	s := NewDefinitionSerializer()
	def := NewTriggerFlowDefinition("yaml_flow")
	def.AddOperator(&FlowConfigOperator{Kind: "chunk", Name: "op1"})

	yamlStr, err := s.ToYAML(def, false, "yaml_flow")
	if err != nil {
		t.Fatalf("ToYAML failed: %v", err)
	}
	if !strings.Contains(yamlStr, "yaml_flow") {
		t.Errorf("YAML should contain flow name, got:\n%s", yamlStr)
	}
	if !strings.Contains(yamlStr, "chunk") {
		t.Errorf("YAML should contain operator kind, got:\n%s", yamlStr)
	}
}

// Check: FromYAML 解析 YAML
func TestDefinitionSerializerFromYAML(t *testing.T) {
	s := NewDefinitionSerializer()
	yamlStr := `version: trigger_flow/v1
name: from_yaml
operators:
  - kind: chunk
    name: op1
`
	def, err := s.FromYAML(yamlStr)
	if err != nil {
		t.Fatalf("FromYAML failed: %v", err)
	}
	if def.Name != "from_yaml" {
		t.Errorf("Name = %q, want from_yaml", def.Name)
	}
	if len(def.Operators) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(def.Operators))
	}
}

// ============================================================================
// ToMermaid 测试
// ============================================================================

// Check: ToMermaidDef flow 模式生成正确
func TestDefinitionSerializerToMermaidFlow(t *testing.T) {
	s := NewDefinitionSerializer()
	def := NewTriggerFlowDefinition("mermaid_flow")
	def.AddOperator(&FlowConfigOperator{
		Kind:   "chunk",
		Name:   "step1",
		Input:  "START",
		Output: "Chunk[step1]",
	})
	def.AddOperator(&FlowConfigOperator{
		Kind:  "chunk",
		Name:  "step2",
		Input: "step1",
	})

	out, err := s.ToMermaidDef(def, "flow")
	if err != nil {
		t.Fatalf("ToMermaidDef failed: %v", err)
	}
	if !strings.HasPrefix(out, "flowchart TD") {
		t.Errorf("expected flowchart TD header, got: %s", out)
	}
	if !strings.Contains(out, "step1") {
		t.Errorf("output should contain 'step1', got: %s", out)
	}
	if !strings.Contains(out, "step2") {
		t.Errorf("output should contain 'step2', got: %s", out)
	}
}

// Check: ToMermaidDef signal 模式生成正确
func TestDefinitionSerializerToMermaidSignal(t *testing.T) {
	s := NewDefinitionSerializer()
	def := NewTriggerFlowDefinition("mermaid_sig")
	def.AddOperator(&FlowConfigOperator{
		Kind:   "chunk",
		Name:   "step1",
		Input:  "START",
		Output: "OUT",
	})

	out, err := s.ToMermaidDef(def, "signal")
	if err != nil {
		t.Fatalf("ToMermaidDef failed: %v", err)
	}
	if !strings.HasPrefix(out, "flowchart LR") {
		t.Errorf("expected flowchart LR header, got: %s", out)
	}
	if !strings.Contains(out, "START") {
		t.Errorf("output should contain 'START', got: %s", out)
	}
	if !strings.Contains(out, "OUT") {
		t.Errorf("output should contain 'OUT', got: %s", out)
	}
}

// Check: ToMermaidDef 不支持的 mode 返回 error
func TestDefinitionSerializerToMermaidBadMode(t *testing.T) {
	s := NewDefinitionSerializer()
	def := NewTriggerFlowDefinition("mermaid_bad")
	_, err := s.ToMermaidDef(def, "bogus")
	if err == nil {
		t.Error("expected error for bad mode")
	}
}

// Check: sanitizeMermaidID 正确处理特殊字符
func TestSanitizeMermaidID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"abc", "abc"},
		{"a-b-c", "a_b_c"},
		{"a.b.c", "a_b_c"},
		{"a[0]", "a_0_"},
		{"", "_"},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got := sanitizeMermaidID(c.input)
			if got != c.want {
				t.Errorf("sanitizeMermaidID(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

// ============================================================================
// BlueprintExporter / BlueprintImporter 接口实现测试
// ============================================================================

// Check: DefinitionSerializer 实现 BlueprintExporter 和 BlueprintImporter 接口
func TestDefinitionSerializerImplementsInterfaces(t *testing.T) {
	var _ BlueprintExporter = (*DefinitionSerializer)(nil)
	var _ BlueprintImporter = (*DefinitionSerializer)(nil)
}

// Check: RegisterBlueprint + GetFlowConfig 协作
func TestDefinitionSerializerRegisterAndGetFlowConfig(t *testing.T) {
	s := NewDefinitionSerializer()
	bp := NewTriggerFlowBlueprint("reg_flow")
	bp.Definition.AddOperator(&FlowConfigOperator{Kind: "chunk", Name: "op1"})
	s.RegisterBlueprint(bp)

	dict, err := s.GetFlowConfig("reg_flow", false)
	if err != nil {
		t.Fatalf("GetFlowConfig failed: %v", err)
	}
	if dict["name"] != "reg_flow" {
		t.Errorf("name = %v, want reg_flow", dict["name"])
	}
}

// Check: GetFlowConfig 校验失败返回 error
func TestDefinitionSerializerGetFlowConfigValidate(t *testing.T) {
	s := NewDefinitionSerializer()
	bp := &TriggerFlowBlueprint{
		Name: "bad",
		Definition: &TriggerFlowDefinition{
			Version: "wrong",
			Name:    "bad",
		},
	}
	s.RegisterBlueprint(bp)
	_, err := s.GetFlowConfig("bad", true)
	if err == nil {
		t.Error("expected error for invalid definition with validate=true")
	}
}

// Check: GetJSONFlow/GetYAMLFlow 返回字符串
func TestDefinitionSerializerGetJSONYAMLFlow(t *testing.T) {
	s := NewDefinitionSerializer()
	bp := NewTriggerFlowBlueprint("export_flow")
	bp.Definition.AddOperator(&FlowConfigOperator{Kind: "chunk", Name: "op1"})
	s.RegisterBlueprint(bp)

	jsonStr, err := s.GetJSONFlow("export_flow")
	if err != nil {
		t.Fatalf("GetJSONFlow failed: %v", err)
	}
	if !strings.Contains(jsonStr, "export_flow") {
		t.Errorf("JSON should contain name, got: %s", jsonStr)
	}

	yamlStr, err := s.GetYAMLFlow("export_flow")
	if err != nil {
		t.Fatalf("GetYAMLFlow failed: %v", err)
	}
	if !strings.Contains(yamlStr, "export_flow") {
		t.Errorf("YAML should contain name, got: %s", yamlStr)
	}
}

// Check: ToMermaid (无 name 参数) 使用活动 Blueprint
func TestDefinitionSerializerToMermaidActive(t *testing.T) {
	s := NewDefinitionSerializer()
	bp := NewTriggerFlowBlueprint("mermaid_active")
	bp.Definition.AddOperator(&FlowConfigOperator{Kind: "chunk", Name: "op1"})
	s.RegisterBlueprint(bp)

	out, err := s.ToMermaid("flow")
	if err != nil {
		t.Fatalf("ToMermaid failed: %v", err)
	}
	if !strings.Contains(out, "op1") {
		t.Errorf("output should contain 'op1', got: %s", out)
	}
}

// Check: SetActive 切换活动 Blueprint
func TestDefinitionSerializerSetActive(t *testing.T) {
	s := NewDefinitionSerializer()
	bp1 := NewTriggerFlowBlueprint("flow1")
	bp1.Definition.AddOperator(&FlowConfigOperator{Kind: "chunk", Name: "op1"})
	bp2 := NewTriggerFlowBlueprint("flow2")
	bp2.Definition.AddOperator(&FlowConfigOperator{Kind: "chunk", Name: "op2"})
	s.RegisterBlueprint(bp1)
	s.RegisterBlueprint(bp2)

	// 当前活动是 bp2（最后注册）
	out, _ := s.ToMermaid("flow")
	if !strings.Contains(out, "op2") {
		t.Errorf("expected active bp2 with 'op2', got: %s", out)
	}

	// 切换到 bp1
	if err := s.SetActive("flow1"); err != nil {
		t.Fatalf("SetActive failed: %v", err)
	}
	out, _ = s.ToMermaid("flow")
	if !strings.Contains(out, "op1") {
		t.Errorf("expected active bp1 with 'op1', got: %s", out)
	}
}

// Check: SetActive 不存在的 name 返回 error
func TestDefinitionSerializerSetActiveMissing(t *testing.T) {
	s := NewDefinitionSerializer()
	err := s.SetActive("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent name")
	}
}

// ============================================================================
// LoadFlowConfig / LoadJSONFlow / LoadYAMLFlow 测试
// ============================================================================

// Check: LoadFlowConfig 从字典加载
func TestDefinitionSerializerLoadFlowConfig(t *testing.T) {
	s := NewDefinitionSerializer()
	config := map[string]any{
		"version": FLOW_CONFIG_VERSION,
		"name":    "loaded_flow",
		"operators": []any{
			map[string]any{"kind": "chunk", "name": "op1"},
		},
	}
	bp, err := s.LoadFlowConfig(config, false)
	if err != nil {
		t.Fatalf("LoadFlowConfig failed: %v", err)
	}
	if bp.Name != "loaded_flow" {
		t.Errorf("Name = %q, want loaded_flow", bp.Name)
	}
	// 验证已注册到 store
	if _, ok := s.GetBlueprint("loaded_flow"); !ok {
		t.Error("blueprint not registered in store")
	}
	// 验证 activeName 已设置
	if s.ActiveName() != "loaded_flow" {
		t.Errorf("ActiveName = %q, want loaded_flow", s.ActiveName())
	}
}

// Check: LoadFlowConfig replace=false 已存在时返回 error
func TestDefinitionSerializerLoadFlowConfigNoReplace(t *testing.T) {
	s := NewDefinitionSerializer()
	config := map[string]any{
		"version": FLOW_CONFIG_VERSION,
		"name":    "dup_flow",
	}
	if _, err := s.LoadFlowConfig(config, false); err != nil {
		t.Fatalf("first LoadFlowConfig failed: %v", err)
	}
	if _, err := s.LoadFlowConfig(config, false); err == nil {
		t.Error("expected error for duplicate name (replace=false)")
	}
}

// Check: LoadFlowConfig replace=true 覆盖
func TestDefinitionSerializerLoadFlowConfigReplace(t *testing.T) {
	s := NewDefinitionSerializer()
	config1 := map[string]any{
		"version":   FLOW_CONFIG_VERSION,
		"name":      "rep_flow",
		"operators": []any{map[string]any{"kind": "chunk", "name": "op1"}},
	}
	config2 := map[string]any{
		"version":   FLOW_CONFIG_VERSION,
		"name":      "rep_flow",
		"operators": []any{map[string]any{"kind": "chunk", "name": "op2"}},
	}
	if _, err := s.LoadFlowConfig(config1, false); err != nil {
		t.Fatalf("first LoadFlowConfig failed: %v", err)
	}
	bp2, err := s.LoadFlowConfig(config2, true)
	if err != nil {
		t.Fatalf("second LoadFlowConfig (replace) failed: %v", err)
	}
	if len(bp2.Definition.Operators) != 1 {
		t.Fatalf("expected 1 operator, got %d", len(bp2.Definition.Operators))
	}
	if bp2.Definition.Operators[0].Name != "op2" {
		t.Errorf("Operator[0].Name = %q, want op2", bp2.Definition.Operators[0].Name)
	}
}

// Check: LoadJSONFlow 从字符串加载
func TestDefinitionSerializerLoadJSONFlow(t *testing.T) {
	s := NewDefinitionSerializer()
	jsonStr := `{"version":"trigger_flow/v1","name":"json_loaded","operators":[{"kind":"chunk","name":"op1"}]}`
	bp, err := s.LoadJSONFlow(jsonStr, false)
	if err != nil {
		t.Fatalf("LoadJSONFlow failed: %v", err)
	}
	if bp.Name != "json_loaded" {
		t.Errorf("Name = %q, want json_loaded", bp.Name)
	}
}

// Check: LoadJSONFlow 从文件加载
func TestDefinitionSerializerLoadJSONFlowFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flow.json")
	jsonBytes := []byte(`{"version":"trigger_flow/v1","name":"file_flow","operators":[]}`)
	if err := writeFile(path, jsonBytes); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	s := NewDefinitionSerializer()
	bp, err := s.LoadJSONFlow(path, false)
	if err != nil {
		t.Fatalf("LoadJSONFlow failed: %v", err)
	}
	if bp.Name != "file_flow" {
		t.Errorf("Name = %q, want file_flow", bp.Name)
	}
}

// Check: LoadYAMLFlow 从字符串加载
func TestDefinitionSerializerLoadYAMLFlow(t *testing.T) {
	s := NewDefinitionSerializer()
	yamlStr := `version: trigger_flow/v1
name: yaml_loaded
operators:
  - kind: chunk
    name: op1
`
	bp, err := s.LoadYAMLFlow(yamlStr, false)
	if err != nil {
		t.Fatalf("LoadYAMLFlow failed: %v", err)
	}
	if bp.Name != "yaml_loaded" {
		t.Errorf("Name = %q, want yaml_loaded", bp.Name)
	}
}
