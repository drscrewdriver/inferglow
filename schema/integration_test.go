package schema

import (
	"testing"
)

// Check 2.6.1: 端到端流程测试通过
func TestEndToEndDefineToValidate(t *testing.T) {
	// Step 1: 定义 Schema
	schema := DefineOutput[map[string]any]()
	schema.Format = OutputJSON
	schema.EnsureAll = true
	schema.Fields = map[string]*FieldDef{
		"title": {
			Type:        TypeString,
			Description: "文章标题",
			Required:    true,
		},
		"content": {
			Type:        TypeString,
			Description: "文章内容",
		},
		"tags": {
			Type:    TypeList,
			ItemDef: &FieldDef{Type: TypeString},
		},
		"metadata": {
			Type: TypeDict,
			Children: map[string]*FieldDef{
				"author": {Type: TypeString, Required: true},
				"date":   {Type: TypeString},
			},
		},
	}

	// Step 2: 生成 JSON Schema
	jsSchema := GenerateJSONSchema(schema)
	if jsSchema == nil {
		t.Fatal("GenerateJSONSchema() returned nil")
	}

	props, ok := jsSchema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be a map")
	}

	// 验证 title 是 string 类型
	titleSchema := props["title"].(map[string]any)
	if titleSchema["type"] != "string" {
		t.Errorf("title.type = %v, want string", titleSchema["type"])
	}

	// 验证 required 包含 title
	required, ok := jsSchema["required"].([]string)
	if !ok {
		t.Fatal("required should be []string")
	}
	found := false
	for _, r := range required {
		if r == "title" {
			found = true
		}
	}
	if !found {
		t.Error("required should contain 'title'")
	}

	// Step 3: 创建 ContractEngine
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"title":  EnsurePresence,
			"tags":   EnsurePresence,
			"metadata.author": EnsurePresence,
		},
		EnsureAll: true,
	}

	// Step 4: 校验有效的 LLM 输出
	validOutput := map[string]any{
		"title": "Test Title",
		"tags":  []any{"go", "testing"},
		"metadata": map[string]any{
			"author": "Author Name",
			"date":   "2025-01-01",
		},
	}
	err := ce.ValidateResult(validOutput)
	if err != nil {
		t.Errorf("ValidateResult(valid) = %v, want nil", err)
	}

	// Step 5: 校验无效的 LLM 输出（缺少必填字段）
	invalidOutput := map[string]any{
		"title": "Test Title",
		// 缺少 tags 和 metadata
	}
	err = ce.ValidateResult(invalidOutput)
	if err == nil {
		t.Error("ValidateResult(invalid) should return error")
	}
}

// TestEndToEndJSONExtractionAndRepair tests the full extraction pipeline
func TestEndToEndJSONExtractionAndRepair(t *testing.T) {
	// 模拟 LLM 返回包含中文标点的损坏 JSON
	llmOutput := `这里是我分析的结果：

｛"title": "测试标题"，"tags": ["标签1"，"标签2"]，"score": 9.5｝

希望能帮到你！`

	// 尝试修复并提取
	repaired := RepairJSONFragment(llmOutput)

	// 从修复后的文本中提取 JSON
	extracted, err := ExtractJSON(repaired)
	if err != nil {
		// 如果仍然无法提取，尝试从原始文本提取
		extracted, err = ExtractJSON(llmOutput)
		if err != nil {
			t.Fatalf("failed to extract JSON: %v", err)
		}
	}

	// 验证提取结果
	if extracted["title"] != "测试标题" {
		t.Errorf("title = %v, want %q", extracted["title"], "测试标题")
	}

	if f, ok := extracted["score"].(float64); !ok || f != 9.5 {
		t.Errorf("score = %v, want 9.5", extracted["score"])
	}
}

// TestEndToEndPathBasedValidation tests path-based validation
func TestEndToEndPathBasedValidation(t *testing.T) {
	// 定义包含嵌套路径的 schema
	ce := &ContractEngine{
		EnsureKeys: map[string]EnsurePolicy{
			"response.status":      EnsurePresence,
			"response.data.items":  EnsurePresence,
			"response.data.count":  EnsurePresence,
		},
		EnsureAll: true,
	}

	validResponse := map[string]any{
		"response": map[string]any{
			"status": "ok",
			"data": map[string]any{
				"items": []any{1, 2, 3},
				"count": 3,
			},
		},
	}

	err := ce.ValidateResult(validResponse)
	if err != nil {
		t.Errorf("ValidateResult(valid nested) = %v, want nil", err)
	}

	// 缺少 data.count
	invalidResponse := map[string]any{
		"response": map[string]any{
			"status": "ok",
			"data": map[string]any{
				"items": []any{1, 2, 3},
			},
		},
	}

	err = ce.ValidateResult(invalidResponse)
	if err == nil {
		t.Error("ValidateResult(missing nested field) should return error")
	}
}

// TestEndToEndWildcardPathValidation tests wildcard path in validation
func TestEndToEndWildcardPathValidation(t *testing.T) {
	// 定义数据
	data := map[string]any{
		"results": []any{
			map[string]any{"id": 1, "valid": true},
			map[string]any{"id": 2, "valid": true},
			map[string]any{"id": 3, "valid": false},
		},
	}

	// 使用通配符路径定位数据
	result, ok := LocatePathInDict(data, "results[*].id")
	if !ok {
		t.Fatal("should find results[*].id")
	}

	ids, ok := result.([]any)
	if !ok {
		t.Fatalf("result should be []any, got %T", result)
	}

	if len(ids) != 3 {
		t.Errorf("len(ids) = %d, want 3", len(ids))
	}
}

// TestEndToEndSchemaGenerationWithAllTypes tests JSON Schema generation for all types
func TestEndToEndSchemaGenerationWithAllTypes(t *testing.T) {
	schema := &OutputSchema{
		Format: OutputJSON,
		Fields: map[string]*FieldDef{
			"name":      {Type: TypeString, Required: true},
			"age":       {Type: TypeInt},
			"score":     {Type: TypeFloat},
			"active":    {Type: TypeBool},
			"nickname":  {Type: TypeOptional},
			"tags":      {Type: TypeList, ItemDef: &FieldDef{Type: TypeString}},
			"settings":  {Type: TypeDict, Children: map[string]*FieldDef{
				"theme": {Type: TypeString},
				"lang":  {Type: TypeString},
			}},
			"status": {Type: TypeModel},
		},
	}

	js := GenerateJSONSchema(schema)
	props := js["properties"].(map[string]any)

	tests := []struct {
		name     string
		wantType string
	}{
		{"name", "string"},
		{"age", "integer"},
		{"score", "number"},
		{"active", "boolean"},
		{"nickname", "string"},
		{"tags", "array"},
		{"settings", "object"},
		{"status", "object"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fieldSchema := props[tt.name].(map[string]any)
			if fieldSchema["type"] != tt.wantType {
				t.Errorf("%s.type = %v, want %s", tt.name, fieldSchema["type"], tt.wantType)
			}
		})
	}
}
