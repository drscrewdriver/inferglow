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

package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RolloutItemType 标识一条 Rollout 记录的类型，用于区分会话级的各类事件。
type RolloutItemType string

const (
	// RolloutUserMessage 表示用户输入消息。
	RolloutUserMessage RolloutItemType = "user_message"
	// RolloutAssistantMessage 表示助手产出消息（含最终回复）。
	RolloutAssistantMessage RolloutItemType = "assistant_message"
	// RolloutToolCall 表示一次工具调用（派发前）。
	RolloutToolCall RolloutItemType = "tool_call"
	// RolloutToolResult 表示一次工具调用结果。
	RolloutToolResult RolloutItemType = "tool_result"
)

// RolloutItem 是会话级 Rollout 的单个条目。它以 JSONL 形式逐行追加落盘，
// 携带 SessionID 便于按会话隔离、携带 Seq 以维持发生顺序，Timestamp 记录
// 事件时间。tool_call / tool_result 可附带 AuditRecordID 关联 audit 链记录。
type RolloutItem struct {
	SessionID string          `json:"session_id"`
	Seq       int64           `json:"seq"`
	Type      RolloutItemType `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Content   string          `json:"content,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	Params    map[string]any  `json:"params,omitempty"`
	Result    string          `json:"result,omitempty"`
	Error     string          `json:"error,omitempty"`
	// AuditRecordID 关联 audit 链记录 ID。刻意不使用 omitempty：即使为空
	// 也须以显式空串出现在 JSONL 中，保证外部工具可按字段探测。
	AuditRecordID string `json:"audit_record_id"`
}

// RolloutRecorder 将会话级 Rollout 事件按发生顺序以 JSONL 追加记录到文件。
//
// 数据布局参考 UsageRecorder：写入 <dataDir>/sessions/<sessionID>.rollout.jsonl。
// 每个 recorder 绑定一个会话 ID；Record 会自动填充 SessionID、Seq 与 Timestamp，
// 调用方只需提供类型与业务字段。
//
// 对 ephemeral（进程内存态）会话的 no-op 支持：当 dataDir 为空字符串时，
// Record 直接返回 nil 而不产生任何文件——上层接线（Agent/Engine）对
// ephemeral 会话传入空目录即可关闭落盘，行为与 UsageRecorder 的
// "sessions/<id>.usage.jsonl" 约定一致。
type RolloutRecorder struct {
	mu        sync.Mutex
	dataDir   string
	sessionID string
	seq       int64
	clock     func() time.Time
}

// NewRolloutRecorder 创建绑定指定会话 ID 的 RolloutRecorder。
// dir 为空字符串表示关闭落盘（用于 ephemeral 会话的 no-op）。
func NewRolloutRecorder(dir, sessionID string) *RolloutRecorder {
	return &RolloutRecorder{
		dataDir:   dir,
		sessionID: sessionID,
		clock:     time.Now,
	}
}

// Record 追加一条 Rollout 记录：填充 SessionID / Seq / Timestamp 后以
// JSONL 追加到文件。对 ephemeral（空 dataDir）会话为 no-op，直接返回 nil。
func (r *RolloutRecorder) Record(item RolloutItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// ephemeral 会话或未配置目录时不落盘。
	if r.dataDir == "" {
		return nil
	}

	item.SessionID = r.sessionID
	r.seq++
	item.Seq = r.seq
	if item.Timestamp.IsZero() {
		item.Timestamp = r.clock()
	}

	return r.appendLocked(item)
}

// appendLocked 在持有 r.mu 的情况下将单条 item 序列化为一行 JSON 追加到底层文件。
func (r *RolloutRecorder) appendLocked(item RolloutItem) error {
	dir := filepath.Join(r.dataDir, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create rollout dir: %w", err)
	}
	path := filepath.Join(dir, r.sessionID+".rollout.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open rollout file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal rollout item: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write rollout item: %w", err)
	}
	return nil
}

// loadLocked 返回排序后的 item 列表。持有调用方锁时调用。
func (r *RolloutRecorder) loadLocked(sessionID string) ([]RolloutItem, error) {
	if r.dataDir == "" {
		return nil, nil
	}
	path := filepath.Join(r.dataDir, "sessions", sessionID+".rollout.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read rollout file: %w", err)
	}

	items := make([]RolloutItem, 0)
	for _, line := range splitLines(string(data)) {
		if line == "" {
			continue
		}
		var item RolloutItem
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			// 跳过损坏行，保持 List/Replay 健壮性。
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

// List 按发生顺序（Seq 升序）返回指定会话的 Rollout items 流。
// 会话不存在或没有记录时返回空切片（nil），不返回错误。
func (r *RolloutRecorder) List(sessionID string) ([]RolloutItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked(sessionID)
}

// Replay 按记录顺序返回指定会话的 Rollout items，供按记录重放。
// 与 List 在数据层面等价（均按 Seq 升序重建发生顺序流）；语义上
// Replay 用于将已记录的事件流还原以便重放驱动。
func (r *RolloutRecorder) Replay(sessionID string) ([]RolloutItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked(sessionID)
}
