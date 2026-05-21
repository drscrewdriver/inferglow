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

// Workspace 血缘管理（Lineage Management）
//
// 本文件为 Workspace 提供文件血缘（lineage / provenance）追踪能力：
// 记录每个文件由谁、何时、通过何种操作、从哪些父文件衍生而来，
// 并支持祖先/后代链查询与环检测。
//
// 设计要点：
//   - 与 workspace.go 解耦：血缘存储是独立抽象（LineageStore 接口），
//     调用方负责在合适的时机调用 Record；Workspace 本身无需修改。
//   - 默认提供内存实现 MemoryLineageStore，并发安全（sync.RWMutex）。
//   - 提供 JSON 持久化辅助函数 SaveLineageToFile / LoadLineageFromFile，
//     便于将血缘快照写入 Workspace 内的 sidecar 文件。
//   - Ancestors / Descendants 使用 BFS 遍历，显式 visited 集合防止环导致死循环；
//     Record 时主动检测"试图让自己成为自己祖先"的环并拒绝。

package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// 血缘相关错误哨兵。
var (
	// ErrLineageNotFound 表示查询的路径在血缘存储中不存在记录。
	ErrLineageNotFound = errors.New("lineage record not found")
	// ErrLineageCycle 表示记录时检测到会导致祖先环的关系。
	ErrLineageCycle = errors.New("lineage cycle detected")
	// ErrEmptyLineagePath 表示血缘节点的 Path 字段为空。
	ErrEmptyLineagePath = errors.New("lineage path is empty")
)

// LineageNode 描述单个文件的血缘信息。
//
// 字段说明：
//   - Path:       文件相对路径，作为唯一 key
//   - Operation:  创建/更新该文件的操作类型，如 "write"/"transform"/"copy"
//   - CreatedAt:  记录时间（UTC）；零值时 Record 自动填入 time.Now()
//   - CreatedBy:  创建者标识（agent 名称、tool 名等）
//   - Parents:    衍生来源文件路径列表
//   - Metadata:   任意附加元数据（如工具参数、prompt 摘要等）
type LineageNode struct {
	Path      string         `json:"path"`
	Operation string         `json:"operation,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	CreatedBy string         `json:"created_by,omitempty"`
	Parents   []string       `json:"parents,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// LineageStore 是血缘存储的抽象接口。
//
// 所有方法应当并发安全；Path 参数为相对路径（与 Workspace 一致）。
type LineageStore interface {
	// Record 记录或更新某路径的血缘信息。
	// 若 node.Parents 形成包含 node.Path 的环，返回 ErrLineageCycle。
	Record(node LineageNode) error

	// Get 查询某路径的血缘信息。
	// 不存在时返回 ErrLineageNotFound。
	Get(path string) (*LineageNode, error)

	// Parents 返回直接父节点路径列表。
	// 节点不存在时返回 ErrLineageNotFound。
	Parents(path string) ([]string, error)

	// Children 返回直接子节点路径列表（即所有 Parents 中包含 path 的节点）。
	// 节点不存在时返回 ErrLineageNotFound。
	Children(path string) ([]string, error)

	// Ancestors 返回所有祖先路径（递归向上，去重，BFS 顺序）。
	// 自身不包含在内。环数据不会导致死循环（visited 守卫）。
	Ancestors(path string) ([]string, error)

	// Descendants 返回所有后代路径（递归向下，去重，BFS 顺序）。
	// 自身不包含在内。环数据不会导致死循环。
	Descendants(path string) ([]string, error)

	// All 返回所有血缘记录（按 Path 字典序）。
	All() []LineageNode

	// Remove 移除某路径的血缘记录。
	// 注意：不会修改其他节点对已删除路径的 Parents 引用（保留审计链）。
	// 不存在时返回 ErrLineageNotFound。
	Remove(path string) error

	// Size 返回当前血缘记录总数。
	Size() int
}

// ============================================================================
// MemoryLineageStore - 内存实现
// ============================================================================

// MemoryLineageStore 是 LineageStore 的内存实现，使用 sync.RWMutex 保护并发访问。
//
// 适用场景：
//   - 单进程内的血缘追踪
//   - 测试与原型开发
//   - 通过 SaveLineageToFile 持久化后跨进程共享
type MemoryLineageStore struct {
	mu    sync.RWMutex
	nodes map[string]*LineageNode
}

// NewMemoryLineageStore 创建一个空的 MemoryLineageStore。
func NewMemoryLineageStore() *MemoryLineageStore {
	return &MemoryLineageStore{nodes: make(map[string]*LineageNode)}
}

// Record 实现 LineageStore.Record。
func (s *MemoryLineageStore) Record(node LineageNode) error {
	if node.Path == "" {
		return ErrEmptyLineagePath
	}
	// 防御性拷贝 Parents / Metadata，避免外部后续修改。
	nodeCopy := LineageNode{
		Path:      node.Path,
		Operation: node.Operation,
		CreatedAt: node.CreatedAt,
		CreatedBy: node.CreatedBy,
	}
	if len(node.Parents) > 0 {
		nodeCopy.Parents = append([]string(nil), node.Parents...)
	}
	if node.Metadata != nil {
		nodeCopy.Metadata = make(map[string]any, len(node.Metadata))
		for k, v := range node.Metadata {
			nodeCopy.Metadata[k] = v
		}
	}
	if nodeCopy.CreatedAt.IsZero() {
		nodeCopy.CreatedAt = time.Now().UTC()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 环检测：若任一 parent 的祖先链能回到 node.Path，则拒绝。
	if err := s.checkCycleLocked(nodeCopy.Path, nodeCopy.Parents); err != nil {
		return err
	}

	// 去重 Parents（保持首次出现顺序）。
	seen := make(map[string]struct{}, len(nodeCopy.Parents))
	dedup := make([]string, 0, len(nodeCopy.Parents))
	for _, p := range nodeCopy.Parents {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		dedup = append(dedup, p)
	}
	nodeCopy.Parents = dedup

	// 拷贝一份赋值，避免外部持有指针修改。
	stored := nodeCopy
	s.nodes[nodeCopy.Path] = &stored
	return nil
}

// checkCycleLocked 检查新增 (path -> parents) 关系是否会形成环。
// 算法：从 parents 出发 BFS 向上访问祖先，若途中遇到 path 则成环。
// 调用者必须持有 s.mu。
func (s *MemoryLineageStore) checkCycleLocked(path string, parents []string) error {
	visited := make(map[string]struct{})
	queue := make([]string, 0, len(parents))
	queue = append(queue, parents...)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == path {
			return fmt.Errorf("%w: %q is reachable from its own parents", ErrLineageCycle, path)
		}
		if _, ok := visited[cur]; ok {
			continue
		}
		visited[cur] = struct{}{}
		if n, ok := s.nodes[cur]; ok {
			queue = append(queue, n.Parents...)
		}
	}
	return nil
}

// Get 实现 LineageStore.Get。
func (s *MemoryLineageStore) Get(path string) (*LineageNode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[path]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrLineageNotFound, path)
	}
	// 返回拷贝避免外部修改内部状态。
	cp := *n
	if len(n.Parents) > 0 {
		cp.Parents = append([]string(nil), n.Parents...)
	}
	if n.Metadata != nil {
		cp.Metadata = make(map[string]any, len(n.Metadata))
		for k, v := range n.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp, nil
}

// Parents 实现 LineageStore.Parents。
func (s *MemoryLineageStore) Parents(path string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n, ok := s.nodes[path]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrLineageNotFound, path)
	}
	if len(n.Parents) == 0 {
		return nil, nil
	}
	return append([]string(nil), n.Parents...), nil
}

// Children 实现 LineageStore.Children。
// 扫描所有节点，收集 Parents 中包含 path 的节点。
func (s *MemoryLineageStore) Children(path string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.nodes[path]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrLineageNotFound, path)
	}
	out := make([]string, 0)
	for p, n := range s.nodes {
		for _, parent := range n.Parents {
			if parent == path {
				out = append(out, p)
				break
			}
		}
	}
	if len(out) <= 1 {
		return out, nil
	}
	// 字典序输出以保持稳定。
	// 简单插入排序，无额外依赖。
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j] < out[j-1] {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	return out, nil
}

// Ancestors 实现 LineageStore.Ancestors。
// 从 path 出发向上 BFS，返回所有祖先（不含自身），按 BFS 发现顺序。
// visited 集合防止数据被外部直接写入形成的环导致死循环。
func (s *MemoryLineageStore) Ancestors(path string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.nodes[path]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrLineageNotFound, path)
	}
	visited := map[string]struct{}{path: {}}
	out := make([]string, 0)
	queue := []string{path}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		n, ok := s.nodes[cur]
		if !ok {
			continue
		}
		for _, p := range n.Parents {
			if _, ok := visited[p]; ok {
				continue
			}
			visited[p] = struct{}{}
			out = append(out, p)
			queue = append(queue, p)
		}
	}
	return out, nil
}

// Descendants 实现 LineageStore.Descendants。
// 反向 BFS：从 path 出发，沿 Children 边向下遍历。
func (s *MemoryLineageStore) Descendants(path string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.nodes[path]; !ok {
		return nil, fmt.Errorf("%w: %q", ErrLineageNotFound, path)
	}
	// 预构建 parent -> children 映射。
	childrenOf := make(map[string][]string, len(s.nodes))
	for p, n := range s.nodes {
		for _, parent := range n.Parents {
			childrenOf[parent] = append(childrenOf[parent], p)
		}
	}
	visited := map[string]struct{}{path: {}}
	out := make([]string, 0)
	queue := []string{path}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, c := range childrenOf[cur] {
			if _, ok := visited[c]; ok {
				continue
			}
			visited[c] = struct{}{}
			out = append(out, c)
			queue = append(queue, c)
		}
	}
	return out, nil
}

// All 实现 LineageStore.All，按 Path 字典序返回拷贝。
func (s *MemoryLineageStore) All() []LineageNode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]LineageNode, 0, len(s.nodes))
	for _, n := range s.nodes {
		cp := *n
		if len(n.Parents) > 0 {
			cp.Parents = append([]string(nil), n.Parents...)
		}
		if n.Metadata != nil {
			cp.Metadata = make(map[string]any, len(n.Metadata))
			for k, v := range n.Metadata {
				cp.Metadata[k] = v
			}
		}
		out = append(out, cp)
	}
	// 按 Path 字典序。
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j].Path < out[j-1].Path {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	return out
}

// Remove 实现 LineageStore.Remove。
// 仅删除该路径自身的记录；不会清理其他节点对该路径的 Parents 引用
// （保留审计链）。如果希望级联清理，调用方需自行遍历 Children。
func (s *MemoryLineageStore) Remove(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[path]; !ok {
		return fmt.Errorf("%w: %q", ErrLineageNotFound, path)
	}
	delete(s.nodes, path)
	return nil
}

// Size 实现 LineageStore.Size。
func (s *MemoryLineageStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.nodes)
}

// 编译期断言。
var _ LineageStore = (*MemoryLineageStore)(nil)

// ============================================================================
// JSON 持久化
// ============================================================================

// lineageFileFormat 是 JSON 文件的顶层结构。
// version 字段预留以便未来迁移。
type lineageFileFormat struct {
	Version int           `json:"version"`
	Nodes   []LineageNode `json:"nodes"`
}

const lineageFileFormatVersion = 1

// SaveLineageToFile 将 LineageStore 内容序列化为 JSON 写入 path。
// 若文件已存在会被覆盖。仅支持 *MemoryLineageStore（其他实现需自行处理）。
//
// 注意：path 应是绝对路径或相对于进程 CWD 的路径；
// 若希望写入 Workspace 内部，调用方应先用 Workspace.SafePath 解析。
func SaveLineageToFile(s LineageStore, path string) error {
	if s == nil {
		return fmt.Errorf("lineage store is nil")
	}
	ms, ok := s.(*MemoryLineageStore)
	if !ok {
		return fmt.Errorf("unsupported LineageStore type: %T", s)
	}
	data := lineageFileFormat{
		Version: lineageFileFormatVersion,
		Nodes:   ms.All(),
	}
	buf, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lineage: %w", err)
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return fmt.Errorf("write lineage file: %w", err)
	}
	return nil
}

// LoadLineageFromFile 从 JSON 文件加载血缘记录到新的 MemoryLineageStore。
// 文件不存在时返回包装后的错误。
//
// 注意：加载时不会重新执行环检测——假定保存时数据是良构的。
// 若数据已损坏存在环，Ancestors/Descendants 的 visited 守卫会防止死循环。
func LoadLineageFromFile(path string) (*MemoryLineageStore, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read lineage file: %w", err)
	}
	var data lineageFileFormat
	if err := json.Unmarshal(buf, &data); err != nil {
		return nil, fmt.Errorf("unmarshal lineage: %w", err)
	}
	if data.Version > lineageFileFormatVersion {
		return nil, fmt.Errorf("lineage file version %d not supported (max %d)", data.Version, lineageFileFormatVersion)
	}
	store := NewMemoryLineageStore()
	for _, n := range data.Nodes {
		// 直接写入内部 map，跳过环检测（假定保存时已验证）。
		// 但仍做防御性拷贝。
		nodeCopy := n
		if len(n.Parents) > 0 {
			nodeCopy.Parents = append([]string(nil), n.Parents...)
		}
		if n.Metadata != nil {
			nodeCopy.Metadata = make(map[string]any, len(n.Metadata))
			for k, v := range n.Metadata {
				nodeCopy.Metadata[k] = v
			}
		}
		if nodeCopy.CreatedAt.IsZero() {
			nodeCopy.CreatedAt = time.Now().UTC()
		}
		store.nodes[n.Path] = &nodeCopy
	}
	return store, nil
}
