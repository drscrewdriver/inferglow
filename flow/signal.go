package flow

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// SignalType 信号触发类型
type SignalType string

const (
	SignalEvent       SignalType = "event"
	SignalRuntimeData SignalType = "runtime_data"
	SignalFlowData    SignalType = "flow_data"
)

// Signal 信号，是 SignalNet 路由的基本单元。
type Signal struct {
	ID           string
	TriggerEvent string         // 如 "START", "Chunk[my_handler]-abc123"
	TriggerType  SignalType     // "event" | "runtime_data" | "flow_data"
	Value        any
	Meta         map[string]any
}

// TriggerFlowRuntimeData handler 执行时的运行时上下文。
type TriggerFlowRuntimeData struct {
	RuntimeData map[string]any // 可变运行时数据（per-execution）
	FlowData    map[string]any // 不可变流数据（flow-level）
	Signal      *Signal        // 当前触发的信号
	Result      any            // 累积结果

	// 以下字段用于将外层 context/SignalNet/EmitSignal 透传给 wrapOperatorHandlerAsHandler
	// 包装的 OperatorHandler。零值（nil）时包装器回退到 context.Background()。
	// 这些字段不参与 JSON 序列化（runtime-only）。
	Ctx        context.Context  `json:"-" yaml:"-"`
	SignalNet  *SignalNet       `json:"-" yaml:"-"`
	EmitSignal func(Signal)     `json:"-" yaml:"-"`
}

// Handler 信号处理函数。
type Handler func(data *TriggerFlowRuntimeData) (any, error)

// DynamicBinding 动态注册的 handler 绑定。
type DynamicBinding struct {
	Handler   Handler
	BindingID string
	Durable   bool // 是否跨轮次保留
}

// SignalAttemptTracker 信号处理尝试追踪。
type SignalAttemptTracker struct {
	Attempts  int
	LastError error
}

// RegisterOption 动态注册的可选参数。
type RegisterOption func(*DynamicBinding)

// WithDurable 设置 binding 为 durable（跨轮次保留）。
func WithDurable(durable bool) RegisterOption {
	return func(b *DynamicBinding) {
		b.Durable = durable
	}
}

// signalAcceptedEntry 内部追踪已接受的信号。
type signalAcceptedEntry struct {
	signal  *Signal
	tracker *SignalAttemptTracker
}

// SignalNet 信号路由网络。
// 管理静态 handler（编译期注册）和动态 handler（运行时注册），
// 并追踪信号接受/尝试状态。
type SignalNet struct {
	mu sync.RWMutex

	// 静态 handler 注册表: triggerEvent -> handlerName -> Handler
	staticHandlers map[string]map[string]Handler

	// 动态 handler 注册表: bindingID -> *DynamicBinding
	dynamicBindings map[string]*DynamicBinding

	// 动态 handler 按 triggerEvent 索引: triggerEvent -> []bindingID
	dynamicIndex map[string][]string

	// 信号接受/尝试追踪: signalID -> entry
	signalAccepted map[string]*signalAcceptedEntry

	// bindingID 生成器
	bindingCounter uint64

	// BUG-14/F-MEDIUM-4: signalAccepted TTL 清理。
	// acceptedOrder 记录信号接受顺序（FIFO），用于大小上限驱逐。
	// acceptedAt 记录每个信号的接受时间，用于 TTL 过期判断。
	// maxAccepted 为 0 表示无大小上限；acceptedTTL 为 0 表示无 TTL。
	// 默认 maxAccepted=10000，acceptedTTL=1 小时。
	acceptedOrder []string
	acceptedAt    map[string]time.Time
	maxAccepted   int
	acceptedTTL   time.Duration
}

// 默认的 signalAccepted 清理参数。
const (
	defaultMaxAcceptedSignals = 10000
	defaultAcceptedSignalsTTL = time.Hour
)

// NewSignalNet 创建信号网络。
func NewSignalNet() *SignalNet {
	return &SignalNet{
		staticHandlers:  make(map[string]map[string]Handler),
		dynamicBindings: make(map[string]*DynamicBinding),
		dynamicIndex:    make(map[string][]string),
		signalAccepted:  make(map[string]*signalAcceptedEntry),
		acceptedAt:      make(map[string]time.Time),
		maxAccepted:     defaultMaxAcceptedSignals,
		acceptedTTL:     defaultAcceptedSignalsTTL,
	}
}

// SetAcceptedSignalsLimit 设置 signalAccepted 的最大条目数。
// n=0 表示无大小上限。超过上限时按 FIFO 顺序驱逐最旧条目。
func (sn *SignalNet) SetAcceptedSignalsLimit(n int) {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	sn.maxAccepted = n
	// 立即应用新的上限
	sn.evictOversizeLocked()
}

// GetAcceptedSignalsLimit 返回当前 signalAccepted 的最大条目数（0=无上限）。
func (sn *SignalNet) GetAcceptedSignalsLimit() int {
	sn.mu.RLock()
	defer sn.mu.RUnlock()
	return sn.maxAccepted
}

// SetAcceptedSignalsTTL 设置 signalAccepted 的 TTL。
// ttl=0 表示无 TTL（不会因过期被清理）。
func (sn *SignalNet) SetAcceptedSignalsTTL(ttl time.Duration) {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	sn.acceptedTTL = ttl
}

// GetAcceptedSignalsTTL 返回当前 signalAccepted 的 TTL（0=无 TTL）。
func (sn *SignalNet) GetAcceptedSignalsTTL() time.Duration {
	sn.mu.RLock()
	defer sn.mu.RUnlock()
	return sn.acceptedTTL
}

// AcceptedSignalsCount 返回当前 signalAccepted 中的条目数。
func (sn *SignalNet) AcceptedSignalsCount() int {
	sn.mu.RLock()
	defer sn.mu.RUnlock()
	return len(sn.signalAccepted)
}

// CleanupAcceptedSignals 显式清理 signalAccepted：
//   - 移除超过 TTL 的条目（若 TTL > 0）
//   - 若条目数超过 maxAccepted，按 FIFO 顺序驱逐最旧条目
//
// 此方法在每次 AcceptSignal 时也会自动调用，因此大多数情况下无需手动调用。
func (sn *SignalNet) CleanupAcceptedSignals() {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	sn.evictExpiredLocked(time.Now())
	sn.evictOversizeLocked()
}

// evictExpiredLocked 移除超过 TTL 的条目。调用者必须持有 sn.mu。
func (sn *SignalNet) evictExpiredLocked(now time.Time) {
	if sn.acceptedTTL <= 0 {
		return
	}
	for _, id := range sn.acceptedOrder {
		if acceptedAt, ok := sn.acceptedAt[id]; ok {
			if now.Sub(acceptedAt) > sn.acceptedTTL {
				delete(sn.signalAccepted, id)
				delete(sn.acceptedAt, id)
			}
		}
	}
	sn.rebuildAcceptedOrderLocked()
}

// evictOversizeLocked 若条目数超过 maxAccepted，按 FIFO 顺序驱逐最旧条目。
// 调用者必须持有 sn.mu。
func (sn *SignalNet) evictOversizeLocked() {
	if sn.maxAccepted <= 0 {
		return
	}
	for len(sn.signalAccepted) > sn.maxAccepted {
		if len(sn.acceptedOrder) == 0 {
			break
		}
		oldest := sn.acceptedOrder[0]
		sn.acceptedOrder = sn.acceptedOrder[1:]
		delete(sn.signalAccepted, oldest)
		delete(sn.acceptedAt, oldest)
	}
}

// rebuildAcceptedOrderLocked 重建 acceptedOrder，移除已被删除的 signalID。
// 调用者必须持有 sn.mu。
func (sn *SignalNet) rebuildAcceptedOrderLocked() {
	newOrder := make([]string, 0, len(sn.acceptedOrder))
	for _, id := range sn.acceptedOrder {
		if _, ok := sn.signalAccepted[id]; ok {
			newOrder = append(newOrder, id)
		}
	}
	sn.acceptedOrder = newOrder
}

// RegisterStaticHandler 注册静态 handler。
// triggerEvent 为信号触发事件名（如 "START"），name 为 handler 名称（唯一标识）。
func (sn *SignalNet) RegisterStaticHandler(triggerEvent, name string, handler Handler) {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	if sn.staticHandlers[triggerEvent] == nil {
		sn.staticHandlers[triggerEvent] = make(map[string]Handler)
	}
	sn.staticHandlers[triggerEvent][name] = handler
}

// RegisterDynamicHandler 注册动态 handler，返回 bindingID。
// 可选参数：WithDurable(true) 设置为 durable binding。
func (sn *SignalNet) RegisterDynamicHandler(triggerEvent string, handler Handler, opts ...RegisterOption) (string, error) {
	if handler == nil {
		return "", fmt.Errorf("handler cannot be nil")
	}
	sn.mu.Lock()
	defer sn.mu.Unlock()

	id := atomic.AddUint64(&sn.bindingCounter, 1)
	bindingID := fmt.Sprintf("dyn-%d", id)

	binding := &DynamicBinding{
		Handler:   handler,
		BindingID: bindingID,
		Durable:   false,
	}
	for _, opt := range opts {
		opt(binding)
	}

	sn.dynamicBindings[bindingID] = binding
	sn.dynamicIndex[triggerEvent] = append(sn.dynamicIndex[triggerEvent], bindingID)
	return bindingID, nil
}

// UnregisterDynamicHandler 注销动态 handler。返回是否成功找到并删除。
func (sn *SignalNet) UnregisterDynamicHandler(bindingID string) bool {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	binding, ok := sn.dynamicBindings[bindingID]
	if !ok {
		return false
	}
	delete(sn.dynamicBindings, bindingID)

	// 从索引中移除
	for event, ids := range sn.dynamicIndex {
		for i, id := range ids {
			if id == bindingID {
				sn.dynamicIndex[event] = append(ids[:i], ids[i+1:]...)
				if len(sn.dynamicIndex[event]) == 0 {
					delete(sn.dynamicIndex, event)
				}
				break
			}
		}
		_ = binding // 避免 unused 警告
	}
	return true
}

// GetDynamicBinding 按 bindingID 查找动态绑定。
func (sn *SignalNet) GetDynamicBinding(bindingID string) *DynamicBinding {
	sn.mu.RLock()
	defer sn.mu.RUnlock()
	return sn.dynamicBindings[bindingID]
}

// ClearNonDurable 清除所有非 durable 的动态绑定。
func (sn *SignalNet) ClearNonDurable() {
	sn.mu.Lock()
	defer sn.mu.Unlock()

	// First, build the new index from the EXISTING dynamicIndex, keeping only
	// entries whose binding is still present (i.e. durable ones, since we
	// haven't deleted anything yet). Doing this BEFORE clearing dynamicIndex
	// is critical — otherwise newIndex is always empty.
	newIndex := make(map[string][]string)
	for event, ids := range sn.dynamicIndex {
		for _, id := range ids {
			if binding, exists := sn.dynamicBindings[id]; exists && binding.Durable {
				newIndex[event] = append(newIndex[event], id)
			}
		}
	}

	// Now remove non-durable bindings from dynamicBindings.
	for id, binding := range sn.dynamicBindings {
		if !binding.Durable {
			delete(sn.dynamicBindings, id)
		}
	}

	// Finally, replace the index with the durable-only index built above.
	sn.dynamicIndex = newIndex
}

// Route 返回给定信号匹配的所有 handler（静态 + 动态）。
func (sn *SignalNet) Route(sig *Signal) []Handler {
	sn.mu.RLock()
	defer sn.mu.RUnlock()

	var handlers []Handler

	// 静态 handler
	if eventMap, ok := sn.staticHandlers[sig.TriggerEvent]; ok {
		for _, h := range eventMap {
			handlers = append(handlers, h)
		}
	}

	// 动态 handler
	if ids, ok := sn.dynamicIndex[sig.TriggerEvent]; ok {
		for _, id := range ids {
			if binding, exists := sn.dynamicBindings[id]; exists {
				handlers = append(handlers, binding.Handler)
			}
		}
	}

	return handlers
}

// AcceptSignal 记录信号已被接受，并初始化尝试追踪器。
//
// F-MEDIUM-3: 若信号已存在，保留原 tracker（Attempts + LastError）以保留
// 尝试历史。仅替换 signal 引用为新的 Signal，并刷新 acceptedAt 时间戳。
//
// BUG-14/F-MEDIUM-4: 每次接受新信号时顺带清理过期条目并 enforcing 大小上限。
func (sn *SignalNet) AcceptSignal(sig *Signal) {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	now := time.Now()
	// 清理过期条目（TTL）
	sn.evictExpiredLocked(now)

	// F-MEDIUM-3: 若信号已存在，保留原 tracker（Attempts + LastError）。
	// 仅替换 signal 引用并刷新 acceptedAt；不重置尝试历史。
	var preservedTracker *SignalAttemptTracker
	if existing, exists := sn.signalAccepted[sig.ID]; exists {
		preservedTracker = existing.tracker
		// 移除旧条目（更新接受顺序与时间），但 tracker 已保留
		delete(sn.signalAccepted, sig.ID)
		delete(sn.acceptedAt, sig.ID)
		// 从 acceptedOrder 中移除（稍后 rebuild 会处理）
	}

	if preservedTracker != nil {
		sn.signalAccepted[sig.ID] = &signalAcceptedEntry{
			signal:  sig,
			tracker: preservedTracker,
		}
	} else {
		sn.signalAccepted[sig.ID] = &signalAcceptedEntry{
			signal:  sig,
			tracker: &SignalAttemptTracker{Attempts: 0},
		}
	}
	sn.acceptedAt[sig.ID] = now
	sn.acceptedOrder = append(sn.acceptedOrder, sig.ID)
	// Enforce 大小上限（FIFO 驱逐）
	sn.evictOversizeLocked()
}

// IsAccepted 检查信号是否已被接受。
func (sn *SignalNet) IsAccepted(signalID string) bool {
	sn.mu.RLock()
	defer sn.mu.RUnlock()
	_, ok := sn.signalAccepted[signalID]
	return ok
}

// GetAttemptTracker 获取信号的尝试追踪器。
func (sn *SignalNet) GetAttemptTracker(signalID string) *SignalAttemptTracker {
	sn.mu.RLock()
	defer sn.mu.RUnlock()
	entry, ok := sn.signalAccepted[signalID]
	if !ok {
		return nil
	}
	return entry.tracker
}

// IncrementAttempt 递增信号的尝试次数。
func (sn *SignalNet) IncrementAttempt(signalID string) {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	if entry, ok := sn.signalAccepted[signalID]; ok {
		entry.tracker.Attempts++
	}
}

// MarkAttemptError 记录信号处理的上次错误。
func (sn *SignalNet) MarkAttemptError(signalID string, err error) {
	sn.mu.Lock()
	defer sn.mu.Unlock()
	if entry, ok := sn.signalAccepted[signalID]; ok {
		entry.tracker.LastError = err
	}
}

// GetAcceptedSignal 返回已接受信号的实际 Signal（用于 BatchCollect 等需要读取信号 Value 的场景）。
// 若信号未被接受，返回 nil。
func (sn *SignalNet) GetAcceptedSignal(signalID string) *Signal {
	sn.mu.RLock()
	defer sn.mu.RUnlock()
	entry, ok := sn.signalAccepted[signalID]
	if !ok {
		return nil
	}
	return entry.signal
}
