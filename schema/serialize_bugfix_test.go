package schema

import (
	"encoding/json"
	"testing"
	"time"
)

// ============================================================================
// SC-MEDIUM-2: serialize time.Time UTC
// ============================================================================

// TestSerialize_TimeTimeUTC 验证序列化带非 UTC 时区的 time.Time 后，反序列化结果为 UTC。
func TestSerialize_TimeTimeUTC(t *testing.T) {
	// 使用一个非 UTC 时区（北京时间 UTC+8）
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation failed: %v", err)
	}
	original := time.Date(2024, 6, 15, 12, 30, 0, 0, loc)

	// 通过 SerializeValue 归一化为 UTC 后再序列化
	normalized := SerializeValue(original)
	data, err := json.Marshal(normalized)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// 反序列化回 time.Time
	var restored time.Time
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// 验证反序列化后为 UTC
	if restored.Location() != time.UTC {
		t.Errorf("restored.Location() = %v, want UTC", restored.Location())
	}

	// 验证时间值一致（时刻相同，只是时区不同）
	if !restored.Equal(original) {
		t.Errorf("restored = %v, want equal to %v", restored, original)
	}

	// 验证序列化字符串以 Z 结尾（UTC 标记）
	str := string(data)
	if str[len(str)-2] != 'Z' && str[len(str)-1] != 'Z' {
		// JSON 字符串以 "Z" 结尾表示 UTC
		// 形如 "2024-06-15T04:30:00Z"
		wantSuffix := "Z\""
		if str[len(str)-2:] != wantSuffix {
			t.Errorf("serialized time should end with %q (UTC), got %q", wantSuffix, str[len(str)-2:])
		}
	}
}

// TestSerialize_TimeTimeUTCPointer 验证 *time.Time 也被归一化为 UTC。
func TestSerialize_TimeTimeUTCPointer(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation failed: %v", err)
	}
	original := time.Date(2024, 6, 15, 12, 30, 0, 0, loc)

	normalized := SerializeValue(&original)
	data, err := json.Marshal(normalized)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored time.Time
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.Location() != time.UTC {
		t.Errorf("restored.Location() = %v, want UTC", restored.Location())
	}
}

// TestSerialize_TimeTimeUTCStruct 验证 struct 中的 time.Time 字段也被归一化为 UTC。
func TestSerialize_TimeTimeUTCStruct(t *testing.T) {
	type Event struct {
		Name string    `json:"name"`
		When time.Time `json:"when"`
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation failed: %v", err)
	}
	event := Event{
		Name: "test",
		When: time.Date(2024, 6, 15, 12, 30, 0, 0, loc),
	}

	normalized := SerializeValue(event)
	data, err := json.Marshal(normalized)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var restored Event
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if restored.When.Location() != time.UTC {
		t.Errorf("restored.When.Location() = %v, want UTC", restored.When.Location())
	}
}

// TestSerialize_ValuePassthrough 验证非 time.Time 值原样返回。
func TestSerialize_ValuePassthrough(t *testing.T) {
	cases := []any{
		"hello",
		42,
		3.14,
		true,
		nil,
		[]string{"a", "b"},
		map[string]int{"x": 1},
	}
	for i, v := range cases {
		got := SerializeValue(v)
		// 简单验证：非 time.Time 类型应原样返回
		if i == 4 { // nil
			if got != nil {
				t.Errorf("case %d: expected nil, got %v", i, got)
			}
			continue
		}
		// 用 JSON 序列化对比，确保数据未被破坏
		origJSON, _ := json.Marshal(v)
		gotJSON, _ := json.Marshal(got)
		if string(origJSON) != string(gotJSON) {
			t.Errorf("case %d: JSON mismatch: orig=%s, got=%s", i, origJSON, gotJSON)
		}
	}
}
