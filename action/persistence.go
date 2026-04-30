package action

import (
	"encoding/json"
	"fmt"
	"os"
)

// Save 将 Registry 序列化为 JSON 文件。
//
// 注意：Executor 是不可序列化的，序列化时仅记录类型名称。
// 重新加载后 Actions 的 Executor 将为 nil，需要使用者自行恢复。
func (r *ActionRegistry) Save(path string) error {
	r.mu.RLock()
	actions := make(map[string]map[string]any, len(r.actions))
	for id, action := range r.actions {
		actions[id] = map[string]any{
			"name":          action.Name,
			"description":   action.Description,
			"schema":        action.Schema,
			"executor_type": executorType(action.Executor),
		}
	}
	r.mu.RUnlock()

	data, err := json.MarshalIndent(map[string]any{"actions": actions}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// Load 从 JSON 文件加载 Registry。
//
// 加载后 Actions 的 Executor 为 nil，需要使用者自行恢复。
func (r *ActionRegistry) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	return r.UnmarshalJSON(data)
}

// MarshalJSON 实现 json.Marshaler 接口。
func (r *ActionRegistry) MarshalJSON() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	actions := make(map[string]map[string]any, len(r.actions))
	for id, action := range r.actions {
		actions[id] = map[string]any{
			"name":          action.Name,
			"description":   action.Description,
			"schema":        action.Schema,
			"executor_type": executorType(action.Executor),
		}
	}
	return json.Marshal(map[string]any{
		"actions": actions,
	})
}

// UnmarshalJSON 实现 json.Unmarshaler 接口。
//
// 加载后 Actions 的 Executor 为 nil，需要使用者自行恢复。
func (r *ActionRegistry) UnmarshalJSON(data []byte) error {
	var raw struct {
		Actions map[string]map[string]any `json:"actions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = make(map[string]*Action)
	for id, spec := range raw.Actions {
		name, _ := spec["name"].(string)
		desc, _ := spec["description"].(string)
		schema, _ := spec["schema"].(map[string]any)
		r.actions[id] = &Action{
			Name:        name,
			Description: desc,
			Schema:      schema,
		}
	}
	return nil
}

// executorType 返回 executor 的类型名字符串。
func executorType(exec ActionExecutor) string {
	if exec == nil {
		return ""
	}
	switch exec.(type) {
	case *LocalFunctionExecutor:
		return "local_function"
	default:
		return fmt.Sprintf("%T", exec)
	}
}
