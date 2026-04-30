package model

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

// collectEvents 从 parser 的事件 channel 收集所有事件直到关闭。
// 返回事件切片和完成信号（done 或 error 事件）。
func collectEvents(t *testing.T, parser *StreamingJSONParser, timeout time.Duration) []JSONParseEvent {
	t.Helper()
	var events []JSONParseEvent
	var mu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		for evt := range parser.Events() {
			mu.Lock()
			events = append(events, evt)
			mu.Unlock()
			if evt.Type == ParseDone || evt.Type == ParseError {
				// drain remaining
				for range parser.Events() {
				}
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("collectEvents timed out after %v", timeout)
	}
	mu.Lock()
	defer mu.Unlock()
	return events
}

// Check: 完整 JSON 一次性 Feed，产生正确事件序列
func TestStreamingJSONParserSingleFeed(t *testing.T) {
	parser := NewStreamingJSONParser()
	go func() {
		_ = parser.Feed(`{"name":"Alice","age":30}`)
		_ = parser.Close()
	}()

	events := collectEvents(t, parser, 2*time.Second)

	// 预期事件序列：
	// 1. object_start path=""
	// 2. key="name" path=""
	// 3. value="Alice" path="name"
	// 4. key="age" path=""
	// 5. value=30 path="age"
	// 6. object_end path=""
	// 7. done
	expectedTypes := []ParseEventType{
		ParseObjectStart, ParseKey, ParseValue,
		ParseKey, ParseValue,
		ParseObjectEnd, ParseDone,
	}
	if len(events) != len(expectedTypes) {
		t.Fatalf("event count = %d, want %d; events: %+v", len(events), len(expectedTypes), events)
	}
	for i, want := range expectedTypes {
		if events[i].Type != want {
			t.Errorf("event[%d].Type = %q, want %q", i, events[i].Type, want)
		}
	}

	// 验证具体事件
	if events[1].Key != "name" {
		t.Errorf("event[1].Key = %q, want %q", events[1].Key, "name")
	}
	if events[2].Path != "name" {
		t.Errorf("event[2].Path = %q, want %q", events[2].Path, "name")
	}
	if v, ok := events[2].Value.(string); !ok || v != "Alice" {
		t.Errorf("event[2].Value = %v, want \"Alice\"", events[2].Value)
	}
	if events[4].Path != "age" {
		t.Errorf("event[4].Path = %q, want %q", events[4].Path, "age")
	}
	// 数字默认解析为 float64
	if v, ok := events[4].Value.(float64); !ok || v != 30 {
		t.Errorf("event[4].Value = %v, want 30", events[4].Value)
	}

	// 验证 Result
	result := parser.Result()
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Result type = %T, want map[string]any", result)
	}
	if m["name"] != "Alice" {
		t.Errorf("Result[\"name\"] = %v, want Alice", m["name"])
	}
	if m["age"] != float64(30) {
		t.Errorf("Result[\"age\"] = %v, want 30", m["age"])
	}
}

// Check: 增量 Feed（拆分多次）产生与一次性 Feed 相同事件序列
func TestStreamingJSONParserIncrementalFeed(t *testing.T) {
	// 一次性 Feed 的基准
	parser1 := NewStreamingJSONParser()
	go func() {
		_ = parser1.Feed(`{"user":{"name":"Bob","tags":["a","b"]},"active":true}`)
		_ = parser1.Close()
	}()
	baseline := collectEvents(t, parser1, 2*time.Second)

	// 增量 Feed：按字符拆分
	parser2 := NewStreamingJSONParser()
	go func() {
		full := `{"user":{"name":"Bob","tags":["a","b"]},"active":true}`
		// 按 5 字符一组喂入
		for i := 0; i < len(full); i += 5 {
			end := i + 5
			if end > len(full) {
				end = len(full)
			}
			if err := parser2.Feed(full[i:end]); err != nil {
				t.Errorf("incremental Feed at offset %d failed: %v", i, err)
				return
			}
		}
		_ = parser2.Close()
	}()
	incremental := collectEvents(t, parser2, 2*time.Second)

	if len(baseline) != len(incremental) {
		t.Fatalf("event count mismatch: baseline=%d, incremental=%d", len(baseline), len(incremental))
	}
	for i := range baseline {
		if baseline[i].Type != incremental[i].Type {
			t.Errorf("event[%d].Type: baseline=%q, incremental=%q", i, baseline[i].Type, incremental[i].Type)
			continue
		}
		if baseline[i].Path != incremental[i].Path {
			t.Errorf("event[%d].Path: baseline=%q, incremental=%q", i, baseline[i].Path, incremental[i].Path)
		}
		if baseline[i].Key != incremental[i].Key {
			t.Errorf("event[%d].Key: baseline=%q, incremental=%q", i, baseline[i].Key, incremental[i].Key)
		}
		if !reflect.DeepEqual(baseline[i].Value, incremental[i].Value) {
			t.Errorf("event[%d].Value: baseline=%v, incremental=%v", i, baseline[i].Value, incremental[i].Value)
		}
	}

	// Result 应一致
	r1 := parser1.Result()
	r2 := parser2.Result()
	if !reflect.DeepEqual(r1, r2) {
		t.Errorf("Result mismatch:\nbaseline=%v\nincremental=%v", r1, r2)
	}
}

// Check: 嵌套对象 + 数组的路径正确
func TestStreamingJSONParserNestedPath(t *testing.T) {
	parser := NewStreamingJSONParser()
	go func() {
		_ = parser.Feed(`{"users":[{"name":"Alice","age":25}]}`)
		_ = parser.Close()
	}()
	events := collectEvents(t, parser, 2*time.Second)

	// 找到 name 的 value 事件，验证路径
	var nameEvt *JSONParseEvent
	var ageEvt *JSONParseEvent
	for i := range events {
		if events[i].Type == ParseValue && events[i].Key == "name" {
			nameEvt = &events[i]
		}
		if events[i].Type == ParseValue && events[i].Key == "age" {
			ageEvt = &events[i]
		}
	}
	if nameEvt == nil {
		t.Fatal("missing value event for 'name'")
	}
	if nameEvt.Path != "users[0].name" {
		t.Errorf("name value path = %q, want %q", nameEvt.Path, "users[0].name")
	}
	if ageEvt == nil {
		t.Fatal("missing value event for 'age'")
	}
	if ageEvt.Path != "users[0].age" {
		t.Errorf("age value path = %q, want %q", ageEvt.Path, "users[0].age")
	}

	// 验证数组元素的 ObjectStart 路径
	var arrObjStart *JSONParseEvent
	for i := range events {
		if events[i].Type == ParseObjectStart && events[i].Path == "users[0]" {
			arrObjStart = &events[i]
			break
		}
	}
	if arrObjStart == nil {
		t.Fatal("missing ObjectStart event with path users[0]")
	}
}

// Check: 不完整 JSON 中间状态不报错，等待更多数据
func TestStreamingJSONParserIncompleteJSON(t *testing.T) {
	parser := NewStreamingJSONParser()

	// 喂入不完整 JSON
	if err := parser.Feed(`{"name":"Al`); err != nil {
		t.Fatalf("Feed incomplete JSON should not error: %v", err)
	}

	// 此时不应有 Done 事件
	// 给一点时间确保 parser 处理完
	time.Sleep(50 * time.Millisecond)

	// 继续喂入剩余部分
	go func() {
		_ = parser.Feed(`ice"}`)
		_ = parser.Close()
	}()
	events := collectEvents(t, parser, 2*time.Second)

	// 应该有 done 事件
	var hasDone bool
	for _, evt := range events {
		if evt.Type == ParseDone {
			hasDone = true
		}
	}
	if !hasDone {
		t.Error("expected Done event after completing JSON")
	}

	// Result 应正确
	m, ok := parser.Result().(map[string]any)
	if !ok {
		t.Fatalf("Result type = %T, want map[string]any", parser.Result())
	}
	if m["name"] != "Alice" {
		t.Errorf("Result[name] = %v, want Alice", m["name"])
	}
}

// Check: 无效 JSON 触发 error 事件
func TestStreamingJSONParserInvalidJSON(t *testing.T) {
	parser := NewStreamingJSONParser()
	go func() {
		_ = parser.Feed(`{"name":invalid}`)
		_ = parser.Close()
	}()
	events := collectEvents(t, parser, 2*time.Second)

	var hasError bool
	for _, evt := range events {
		if evt.Type == ParseError {
			hasError = true
			if evt.Value == nil {
				t.Error("ParseError event should have non-nil Value (error message)")
			}
		}
	}
	if !hasError {
		t.Error("expected ParseError event for invalid JSON")
	}
}

// Check: Close 后 Feed 返回错误
func TestStreamingJSONParserFeedAfterDone(t *testing.T) {
	parser := NewStreamingJSONParser()
	go func() {
		_ = parser.Feed(`{}`)
		_ = parser.Close()
	}()
	_ = collectEvents(t, parser, 2*time.Second)

	// 等 parser 完成后 Feed 应报错
	if err := parser.Feed(`{"a":1}`); err == nil {
		t.Error("Feed after done should return error")
	}
}

// Check: 各种值类型正确解析
func TestStreamingJSONParserValueTypes(t *testing.T) {
	parser := NewStreamingJSONParser()
	go func() {
		_ = parser.Feed(`{"s":"str","n":42.5,"b":true,"null":null,"arr":[1,"two",false]}`)
		_ = parser.Close()
	}()
	events := collectEvents(t, parser, 2*time.Second)

	result, ok := parser.Result().(map[string]any)
	if !ok {
		t.Fatalf("Result type = %T, want map[string]any", parser.Result())
	}

	if result["s"] != "str" {
		t.Errorf("s = %v, want str", result["s"])
	}
	if result["n"] != 42.5 {
		t.Errorf("n = %v, want 42.5", result["n"])
	}
	if result["b"] != true {
		t.Errorf("b = %v, want true", result["b"])
	}
	if result["null"] != nil {
		t.Errorf("null = %v, want nil", result["null"])
	}
	arr, ok := result["arr"].([]any)
	if !ok {
		t.Fatalf("arr type = %T, want []any", result["arr"])
	}
	if len(arr) != 3 {
		t.Fatalf("arr len = %d, want 3", len(arr))
	}
	if arr[0] != float64(1) {
		t.Errorf("arr[0] = %v, want 1", arr[0])
	}
	if arr[1] != "two" {
		t.Errorf("arr[1] = %v, want two", arr[1])
	}
	if arr[2] != false {
		t.Errorf("arr[2] = %v, want false", arr[2])
	}

	// 验证 arr 数组元素的 Path
	var arrVals []JSONParseEvent
	for _, evt := range events {
		if evt.Type == ParseValue && evt.Path != "" && (evt.Path == "arr[0]" || evt.Path == "arr[1]" || evt.Path == "arr[2]") {
			arrVals = append(arrVals, evt)
		}
	}
	if len(arrVals) != 3 {
		t.Errorf("arr value events = %d, want 3", len(arrVals))
	}
}

// Check: 顶层数组 JSON
func TestStreamingJSONParserTopLevelArray(t *testing.T) {
	parser := NewStreamingJSONParser()
	go func() {
		_ = parser.Feed(`[1,2,3]`)
		_ = parser.Close()
	}()
	events := collectEvents(t, parser, 2*time.Second)

	// 事件序列：array_start, value[0], value[1], value[2], array_end, done
	expectedTypes := []ParseEventType{
		ParseArrayStart, ParseValue, ParseValue, ParseValue, ParseArrayEnd, ParseDone,
	}
	if len(events) != len(expectedTypes) {
		t.Fatalf("event count = %d, want %d; events: %+v", len(events), len(expectedTypes), events)
	}
	for i, want := range expectedTypes {
		if events[i].Type != want {
			t.Errorf("event[%d].Type = %q, want %q", i, events[i].Type, want)
		}
	}

	// 验证路径
	wantPaths := []string{"", "[0]", "[1]", "[2]", "", ""}
	for i, want := range wantPaths {
		if events[i].Path != want {
			t.Errorf("event[%d].Path = %q, want %q", i, events[i].Path, want)
		}
	}

	result, ok := parser.Result().([]any)
	if !ok {
		t.Fatalf("Result type = %T, want []any", parser.Result())
	}
	if len(result) != 3 {
		t.Errorf("Result len = %d, want 3", len(result))
	}
}

// Check: 顶层原始值 JSON（非对象/数组）
func TestStreamingJSONParserTopLevelPrimitive(t *testing.T) {
	parser := NewStreamingJSONParser()
	go func() {
		_ = parser.Feed(`"hello world"`)
		_ = parser.Close()
	}()
	events := collectEvents(t, parser, 2*time.Second)

	// 事件序列：value, done
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2; events: %+v", len(events), events)
	}
	if events[0].Type != ParseValue {
		t.Errorf("event[0].Type = %q, want %q", events[0].Type, ParseValue)
	}
	if events[0].Value != "hello world" {
		t.Errorf("event[0].Value = %v, want hello world", events[0].Value)
	}
	if events[1].Type != ParseDone {
		t.Errorf("event[1].Type = %q, want %q", events[1].Type, ParseDone)
	}
}

// Check: 空 Feed 不影响解析
func TestStreamingJSONParserEmptyFeed(t *testing.T) {
	parser := NewStreamingJSONParser()
	go func() {
		_ = parser.Feed("")
		_ = parser.Feed(`{"a":1}`)
		_ = parser.Feed("")
		_ = parser.Close()
	}()
	events := collectEvents(t, parser, 2*time.Second)

	var hasDone bool
	for _, evt := range events {
		if evt.Type == ParseDone {
			hasDone = true
		}
	}
	if !hasDone {
		t.Error("expected Done event")
	}
	if parser.Result() == nil {
		t.Error("Result should not be nil")
	}
}

// Check: Close 不完整 JSON 触发 error
func TestStreamingJSONParserCloseIncomplete(t *testing.T) {
	parser := NewStreamingJSONParser()
	go func() {
		_ = parser.Feed(`{"name":`) // 不完整
		_ = parser.Close()
	}()
	events := collectEvents(t, parser, 2*time.Second)

	var hasError bool
	for _, evt := range events {
		if evt.Type == ParseError {
			hasError = true
		}
	}
	if !hasError {
		t.Error("expected ParseError for incomplete JSON on Close")
	}
}
