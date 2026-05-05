package flow

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// ChunkHandler 算子 (OpChunk)。
// 透传输入，并发射一个 "Chunk[<operator_name>]" 信号。
type ChunkHandler struct{}

// Kind 返回 OpChunk。
func (h *ChunkHandler) Kind() OperatorKind { return OpChunk }

// Execute 原样返回 oc.Input，并通过 oc.EmitSignal 发射 Signal{TriggerEvent: "Chunk[<name>]"}。
func (h *ChunkHandler) Execute(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("chunk handler: nil operator context")
	}
	if oc.EmitSignal != nil {
		oc.EmitSignal(Signal{
			ID:           "Chunk[" + oc.Operator.Name + "]",
			TriggerEvent: "Chunk[" + oc.Operator.Name + "]",
			TriggerType:  SignalEvent,
			Value:        oc.Input,
		})
	}
	return oc.Input, nil
}

// SignalGateHandler 算子 (OpSignalGate)。
// 仅当 Options["required_signals"] 中列出的所有信号都已被 SignalNet 接受时，
// 才透传输入；否则返回 (nil, nil)。
type SignalGateHandler struct{}

// Kind 返回 OpSignalGate。
func (h *SignalGateHandler) Kind() OperatorKind { return OpSignalGate }

// Execute 检查所有 required_signals 是否已被接受，全部满足时透传 oc.Input。
func (h *SignalGateHandler) Execute(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("signal_gate handler: nil operator context")
	}
	required, err := readRequiredSignals(oc.Operator)
	if err != nil {
		return nil, fmt.Errorf("signal_gate handler: %w", err)
	}
	if oc.SignalNet == nil {
		return nil, fmt.Errorf("signal_gate handler: nil signal net")
	}
	for _, sigID := range required {
		if !oc.SignalNet.IsAccepted(sigID) {
			return nil, nil
		}
	}
	return oc.Input, nil
}

// readRequiredSignals 从 Operator.Options["required_signals"] 读取 []string。
// 接受 []string 或 []any（每个元素需可转为 string）。
func readRequiredSignals(op *Operator) ([]string, error) {
	if op == nil || op.Options == nil {
		return nil, nil
	}
	raw, ok := op.Options["required_signals"]
	if !ok || raw == nil {
		return nil, nil
	}
	switch v := raw.(type) {
	case []string:
		return v, nil
	case []any:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("required_signals[%d]: expected string, got %T", i, item)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("required_signals: unsupported type %T", raw)
	}
}

// BatchFanoutHandler 算子 (OpBatchFanout)。
// 输入 []any，通过 WorkerPool 并发执行，每个元素由 Options["handler"] 处理；
// 若未提供 handler，使用透传函数。为每个结果发射 "BatchItem[i]" 信号。
//
// 并发度由 Options["max_concurrency"] 控制（默认 min(N, 8)）。
type BatchFanoutHandler struct{}

// Kind 返回 OpBatchFanout。
func (h *BatchFanoutHandler) Kind() OperatorKind { return OpBatchFanout }

// Execute 通过 WorkerPool 并发处理 []any 输入，返回 []any 结果切片。
func (h *BatchFanoutHandler) Execute(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("batch_fanout handler: nil operator context")
	}
	input, ok := oc.Input.([]any)
	if !ok {
		return nil, fmt.Errorf("batch_fanout handler: input expected []any, got %T", oc.Input)
	}

	n := len(input)
	if n == 0 {
		return []any{}, nil
	}

	concurrency := readMaxConcurrency(oc.Operator, n)
	itemHandler := readBatchItemHandler(oc.Operator)

	results := make([]any, n)
	errs := make([]error, n)

	// 使用 WorkerPool 替换硬编码并发：每个任务一个槽位，worker 数量限制并发。
	pool := NewWorkerPool(concurrency, n)
	pool.Start()

	for i := 0; i < n; i++ {
		idx, val := i, input[i]
		if err := pool.Submit(func() {
			res, err := itemHandler(val)
			errs[idx] = err
			results[idx] = res
			if err == nil && oc.EmitSignal != nil {
				oc.EmitSignal(Signal{
					ID:           "BatchItem[" + strconv.Itoa(idx) + "]",
					TriggerEvent: "BatchItem[" + strconv.Itoa(idx) + "]",
					TriggerType:  SignalEvent,
					Value:        res,
				})
			}
		}); err != nil {
			// Submit 失败通常意味着 pool 已被停止；提交后续任务已无意义。
			pool.Stop()
			return nil, fmt.Errorf("batch_fanout handler: submit[%d] failed: %w", idx, err)
		}
	}
	// Stop 关闭任务通道并等待所有 worker 处理完剩余任务后退出。
	// 返回后 results/errs 已被所有任务完整写入。
	pool.Stop()

	// 任意一个失败则整体失败。
	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("batch_fanout handler: item[%d] failed: %w", i, err)
		}
	}
	return results, nil
}

// readMaxConcurrency 从 Options["max_concurrency"] 读取并发度。
// 缺省/非法值时回退到 min(n, 8)。
func readMaxConcurrency(op *Operator, n int) int {
	defaultConcurrency := n
	if defaultConcurrency > 8 {
		defaultConcurrency = 8
	}
	if op == nil || op.Options == nil {
		return defaultConcurrency
	}
	raw, ok := op.Options["max_concurrency"]
	if !ok || raw == nil {
		return defaultConcurrency
	}
	var v int
	switch x := raw.(type) {
	case int:
		v = x
	case int64:
		v = int(x)
	case float64:
		v = int(x)
	default:
		return defaultConcurrency
	}
	if v <= 0 {
		return defaultConcurrency
	}
	if v > n {
		return n
	}
	return v
}

// readBatchItemHandler 从 Options["handler"] 读取 func(any) (any, error)。
// 不存在时返回透传函数。
func readBatchItemHandler(op *Operator) func(any) (any, error) {
	if op != nil && op.Options != nil {
		if raw, ok := op.Options["handler"]; ok {
			if fn, ok := raw.(func(any) (any, error)); ok && fn != nil {
				return fn
			}
		}
	}
	return func(v any) (any, error) { return v, nil }
}

// BatchCollectHandler 算子 (OpBatchCollect)。
// 等待 N 个 "BatchItem[0..N-1]" 信号被接受后，按 index 顺序合并 Value 为 []any。
type BatchCollectHandler struct{}

// Kind 返回 OpBatchCollect。
func (h *BatchCollectHandler) Kind() OperatorKind { return OpBatchCollect }

// Execute 轮询等待所有 BatchItem 信号到达，按 index 合并结果。
func (h *BatchCollectHandler) Execute(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("batch_collect handler: nil operator context")
	}
	if oc.SignalNet == nil {
		return nil, fmt.Errorf("batch_collect handler: nil signal net")
	}
	expected, err := readExpectedCount(oc.Operator)
	if err != nil {
		return nil, fmt.Errorf("batch_collect handler: %w", err)
	}
	if expected <= 0 {
		return []any{}, nil
	}

	signalIDs := make([]string, expected)
	for i := 0; i < expected; i++ {
		signalIDs[i] = "BatchItem[" + strconv.Itoa(i) + "]"
	}

	// 解析父 context（缺省 context.Background()）
	ctx := context.Background()
	if oc.Ctx != nil {
		ctx = oc.Ctx
	}

	const pollInterval = 10 * time.Millisecond
	const maxWait = 30 * time.Second
	deadline := time.Now().Add(maxWait)

	for {
		allAccepted := true
		for _, id := range signalIDs {
			if !oc.SignalNet.IsAccepted(id) {
				allAccepted = false
				break
			}
		}
		if allAccepted {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("batch_collect handler: timed out waiting for %d batch items", expected)
		}
		// 等待 pollInterval 或 ctx 取消，二者先到先返回
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("batch_collect handler: context cancelled: %w", ctx.Err())
		case <-time.After(pollInterval):
		}
	}

	results := make([]any, expected)
	for i, id := range signalIDs {
		sig := oc.SignalNet.GetAcceptedSignal(id)
		if sig == nil {
			return nil, fmt.Errorf("batch_collect handler: signal %q disappeared after accept", id)
		}
		results[i] = sig.Value
	}
	return results, nil
}

// readExpectedCount 从 Options["expected_count"] 读取 int。
func readExpectedCount(op *Operator) (int, error) {
	if op == nil || op.Options == nil {
		return 0, fmt.Errorf("expected_count not set")
	}
	raw, ok := op.Options["expected_count"]
	if !ok || raw == nil {
		return 0, fmt.Errorf("expected_count not set")
	}
	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("expected_count: unsupported type %T", raw)
	}
}

// MatchRouteHandler 算子 (OpMatchRoute)。
// 调用 Options["matcher"](input) 得到 case key，
// 命中 Options["cases"][key] 则执行；否则走 Options["default"]；都没有则返回 error。
type MatchRouteHandler struct{}

// Kind 返回 OpMatchRoute。
func (h *MatchRouteHandler) Kind() OperatorKind { return OpMatchRoute }

// Execute 根据 matcher 路由到对应 case handler 执行。
func (h *MatchRouteHandler) Execute(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("match_route handler: nil operator context")
	}
	matcher, cases, defaultHandler, err := readMatchRouteOptions(oc.Operator)
	if err != nil {
		return nil, fmt.Errorf("match_route handler: %w", err)
	}

	key := matcher(oc.Input)
	if h, ok := cases[key]; ok && h != nil {
		rd := &TriggerFlowRuntimeData{
			RuntimeData: map[string]any{},
			FlowData:    map[string]any{},
			Signal: &Signal{
				ID:           "MatchRoute[" + key + "]",
				TriggerEvent: "MatchRoute[" + key + "]",
				TriggerType:  SignalEvent,
				Value:        oc.Input,
			},
			Result: oc.Input,
		}
		return h(rd)
	}

	if defaultHandler != nil {
		rd := &TriggerFlowRuntimeData{
			RuntimeData: map[string]any{},
			FlowData:    map[string]any{},
			Signal: &Signal{
				ID:           "MatchRoute[default]",
				TriggerEvent: "MatchRoute[default]",
				TriggerType:  SignalEvent,
				Value:        oc.Input,
			},
			Result: oc.Input,
		}
		return defaultHandler(rd)
	}

	return nil, fmt.Errorf("match_route handler: no case matched %q and no default provided", key)
}

// readMatchRouteOptions 从 Operator.Options 读取 matcher / cases / default。
func readMatchRouteOptions(op *Operator) (
	matcher func(any) string,
	cases map[string]Handler,
	defaultHandler Handler,
	err error,
) {
	if op == nil || op.Options == nil {
		return nil, nil, nil, fmt.Errorf("options not set")
	}

	rawMatcher, ok := op.Options["matcher"]
	if !ok || rawMatcher == nil {
		return nil, nil, nil, fmt.Errorf("matcher not set")
	}
	matcher, ok = rawMatcher.(func(any) string)
	if !ok || matcher == nil {
		return nil, nil, nil, fmt.Errorf("matcher: expected func(any) string, got %T", rawMatcher)
	}

	rawCases, ok := op.Options["cases"]
	if !ok || rawCases == nil {
		return nil, nil, nil, fmt.Errorf("cases not set")
	}
	cases, ok = rawCases.(map[string]Handler)
	if !ok {
		return nil, nil, nil, fmt.Errorf("cases: expected map[string]Handler, got %T", rawCases)
	}

	if rawDefault, ok := op.Options["default"]; ok && rawDefault != nil {
		defaultHandler, ok = rawDefault.(Handler)
		if !ok {
			return nil, nil, nil, fmt.Errorf("default: expected Handler, got %T", rawDefault)
		}
	}

	return matcher, cases, defaultHandler, nil
}

// ============================================================================
// ForEachSplitHandler (OpForEachSplit)
// 将 input []any 拆分为每项一个 BatchItem[i] 信号，返回 nil。
// 下游 ForEachCollectHandler 通过等待 BatchItem[i] 信号合并结果。
// ============================================================================

// ForEachSplitHandler 迭代拆分算子。
type ForEachSplitHandler struct{}

// Kind 返回 OpForEachSplit。
func (h *ForEachSplitHandler) Kind() OperatorKind { return OpForEachSplit }

// Execute 将 input []any 拆分为每项一个 BatchItem[i] 信号，
// 并通过 oc.EmitSignal 发射 + oc.SignalNet 接受。
// 返回 (nil, nil)：本算子不直接返回结果，由下游 ForEachCollect 收集。
func (h *ForEachSplitHandler) Execute(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("for_each_split handler: nil operator context")
	}
	input, ok := oc.Input.([]any)
	if !ok {
		return nil, fmt.Errorf("for_each_split handler: input expected []any, got %T", oc.Input)
	}
	for i, v := range input {
		sig := Signal{
			ID:           "BatchItem[" + strconv.Itoa(i) + "]",
			TriggerEvent: "BatchItem[" + strconv.Itoa(i) + "]",
			TriggerType:  SignalEvent,
			Value:        v,
		}
		if oc.EmitSignal != nil {
			oc.EmitSignal(sig)
		}
		if oc.SignalNet != nil {
			oc.SignalNet.AcceptSignal(&sig)
		}
	}
	return nil, nil
}

// ============================================================================
// ForEachCollectHandler (OpForEachCollect)
// 等待所有 BatchItem[0..N-1] 信号被接受后，按 index 顺序合并 Value 为 []any。
// 与 BatchCollectHandler 行为类似，但语义上配合 ForEachSplit 使用。
// ============================================================================

// ForEachCollectHandler 迭代收集算子。
type ForEachCollectHandler struct{}

// Kind 返回 OpForEachCollect。
func (h *ForEachCollectHandler) Kind() OperatorKind { return OpForEachCollect }

// Execute 等待 N 个 BatchItem 信号并按顺序合并结果。
func (h *ForEachCollectHandler) Execute(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("for_each_collect handler: nil operator context")
	}
	if oc.SignalNet == nil {
		return nil, fmt.Errorf("for_each_collect handler: nil signal net")
	}
	expected, err := readExpectedCount(oc.Operator)
	if err != nil {
		return nil, fmt.Errorf("for_each_collect handler: %w", err)
	}
	if expected <= 0 {
		return []any{}, nil
	}

	signalIDs := make([]string, expected)
	for i := 0; i < expected; i++ {
		signalIDs[i] = "BatchItem[" + strconv.Itoa(i) + "]"
	}

	// 解析父 context（缺省 context.Background()）
	ctx := context.Background()
	if oc.Ctx != nil {
		ctx = oc.Ctx
	}

	const pollInterval = 10 * time.Millisecond
	const maxWait = 30 * time.Second
	deadline := time.Now().Add(maxWait)

	for {
		allAccepted := true
		for _, id := range signalIDs {
			if !oc.SignalNet.IsAccepted(id) {
				allAccepted = false
				break
			}
		}
		if allAccepted {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("for_each_collect handler: timed out waiting for %d items", expected)
		}
		// 等待 pollInterval 或 ctx 取消，二者先到先返回
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("for_each_collect handler: context cancelled: %w", ctx.Err())
		case <-time.After(pollInterval):
		}
	}

	results := make([]any, expected)
	for i, id := range signalIDs {
		sig := oc.SignalNet.GetAcceptedSignal(id)
		if sig == nil {
			return nil, fmt.Errorf("for_each_collect handler: signal %q disappeared after accept", id)
		}
		results[i] = sig.Value
	}
	return results, nil
}

// ============================================================================
// MatchCaseHandler (OpMatchCase)
// 从 Options["cases"] 读取 []CaseSpec，选择第一个命中的 case 执行并透传结果。
// ============================================================================

// CaseSpec 定义 MatchCase/MatchCollect 的单个 case。
type CaseSpec struct {
	ID        string                         // case 标识
	Predicate func(input any) bool           // 命中判断
	Handler   func(rd *TriggerFlowRuntimeData) (any, error)
}

// MatchCaseHandler 透传 case 结果算子。
type MatchCaseHandler struct{}

// Kind 返回 OpMatchCase。
func (h *MatchCaseHandler) Kind() OperatorKind { return OpMatchCase }

// Execute 选择第一个 Predicate 命中的 case 执行，透传结果。
// 若无 case 命中且 Options["default"] 存在，执行 default。
// 否则返回 error。
func (h *MatchCaseHandler) Execute(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("match_case handler: nil operator context")
	}
	cases, err := readCaseSpecs(oc.Operator)
	if err != nil {
		return nil, fmt.Errorf("match_case handler: %w", err)
	}
	for _, c := range cases {
		if c.Predicate == nil {
			continue
		}
		if c.Predicate(oc.Input) {
			if c.Handler == nil {
				return oc.Input, nil
			}
			rd := &TriggerFlowRuntimeData{
				RuntimeData: map[string]any{},
				FlowData:    map[string]any{},
				Signal: &Signal{
					ID:           "MatchCase[" + c.ID + "]",
					TriggerEvent: "MatchCase[" + c.ID + "]",
					TriggerType:  SignalEvent,
					Value:        oc.Input,
				},
				Result: oc.Input,
			}
			return c.Handler(rd)
		}
	}
	// 无命中：尝试 default
	if def, ok := oc.Operator.Options["default"]; ok && def != nil {
		if defHandler, ok := def.(func(*TriggerFlowRuntimeData) (any, error)); ok && defHandler != nil {
			rd := &TriggerFlowRuntimeData{
				RuntimeData: map[string]any{},
				FlowData:    map[string]any{},
				Signal: &Signal{
					ID:           "MatchCase[default]",
					TriggerEvent: "MatchCase[default]",
					TriggerType:  SignalEvent,
					Value:        oc.Input,
				},
				Result: oc.Input,
			}
			return defHandler(rd)
		}
	}
	return nil, fmt.Errorf("match_case handler: no case matched and no default provided")
}

// readCaseSpecs 从 Options["cases"] 读取 []CaseSpec。
// 接受 []CaseSpec 或 []any（每个元素需为 CaseSpec）。
func readCaseSpecs(op *Operator) ([]CaseSpec, error) {
	if op == nil || op.Options == nil {
		return nil, fmt.Errorf("options not set")
	}
	raw, ok := op.Options["cases"]
	if !ok || raw == nil {
		return nil, fmt.Errorf("cases not set")
	}
	switch v := raw.(type) {
	case []CaseSpec:
		return v, nil
	case []any:
		out := make([]CaseSpec, 0, len(v))
		for i, item := range v {
			cs, ok := item.(CaseSpec)
			if !ok {
				return nil, fmt.Errorf("cases[%d]: expected CaseSpec, got %T", i, item)
			}
			out = append(out, cs)
		}
		return out, nil
	}
	return nil, fmt.Errorf("cases: expected []CaseSpec or []any, got %T", raw)
}

// ============================================================================
// MatchCollectHandler (OpMatchCollect)
// 执行所有 Predicate 命中的 case，返回 map[caseID]result。
// ============================================================================

// MatchCollectHandler 收集 match 分支结果算子。
type MatchCollectHandler struct{}

// Kind 返回 OpMatchCollect。
func (h *MatchCollectHandler) Kind() OperatorKind { return OpMatchCollect }

// Execute 执行所有命中的 case，返回 map[caseID]result。
// 至少有一个 case 命中；全部不命中时返回空 map（不报错）。
func (h *MatchCollectHandler) Execute(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("match_collect handler: nil operator context")
	}
	cases, err := readCaseSpecs(oc.Operator)
	if err != nil {
		return nil, fmt.Errorf("match_collect handler: %w", err)
	}
	out := make(map[string]any, len(cases))
	for _, c := range cases {
		if c.Predicate == nil || c.Handler == nil {
			continue
		}
		if !c.Predicate(oc.Input) {
			continue
		}
		rd := &TriggerFlowRuntimeData{
			RuntimeData: map[string]any{},
			FlowData:    map[string]any{},
			Signal: &Signal{
				ID:           "MatchCollect[" + c.ID + "]",
				TriggerEvent: "MatchCollect[" + c.ID + "]",
				TriggerType:  SignalEvent,
				Value:        oc.Input,
			},
			Result: oc.Input,
		}
		result, err := c.Handler(rd)
		if err != nil {
			return nil, fmt.Errorf("match_collect handler: case %q failed: %w", c.ID, err)
		}
		out[c.ID] = result
	}
	return out, nil
}

// ============================================================================
// CollectBranchHandler (OpCollectBranch)
// 从 Options["branches"] 读取 map[branchID]Handler，并行执行所有分支，
// 返回 map[branchID]result。
// ============================================================================

// CollectBranchHandler 多分支收集算子。
type CollectBranchHandler struct{}

// Kind 返回 OpCollectBranch。
func (h *CollectBranchHandler) Kind() OperatorKind { return OpCollectBranch }

// Execute 并行执行所有分支，聚合结果为 map[branchID]any。
// 任一分支失败则整体失败。并发度限制为 8（WorkerPool）；父 context 取消时
// 立即返回 ctx.Err()，所有未完成的分支任务在 WorkerPool.Stop 后退出。
func (h *CollectBranchHandler) Execute(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("collect_branch handler: nil operator context")
	}
	branches, err := readBranchHandlers(oc.Operator)
	if err != nil {
		return nil, fmt.Errorf("collect_branch handler: %w", err)
	}
	if len(branches) == 0 {
		return map[string]any{}, nil
	}

	// 解析父 context（缺省 context.Background()）
	parentCtx := context.Background()
	if oc.Ctx != nil {
		parentCtx = oc.Ctx
	}
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	// 收集 branch id（排序后保证可确定性）
	ids := make([]string, 0, len(branches))
	for id := range branches {
		ids = append(ids, id)
	}

	type branchResult struct {
		id     string
		result any
		err    error
	}

	// 使用 WorkerPool 限制并发度为 8
	const maxConcurrency = 8
	workers := maxConcurrency
	if workers > len(ids) {
		workers = len(ids)
	}
	pool := NewWorkerPool(workers, len(ids))
	pool.Start()

	resultsCh := make(chan branchResult, len(ids))
	submitted := 0
	for _, id := range ids {
		idx := id
		handler := branches[idx]
		// 提交前先检查 ctx 是否已取消
		if err := ctx.Err(); err != nil {
			break
		}
		subErr := pool.Submit(func() {
			// 二次检查 ctx
			if err := ctx.Err(); err != nil {
				resultsCh <- branchResult{id: idx, result: nil, err: err}
				return
			}
			rd := &TriggerFlowRuntimeData{
				RuntimeData: map[string]any{},
				FlowData:    map[string]any{},
				Signal: &Signal{
					ID:           "CollectBranch[" + idx + "]",
					TriggerEvent: "CollectBranch[" + idx + "]",
					TriggerType:  SignalEvent,
					Value:        oc.Input,
				},
				Result: oc.Input,
			}
			r, err := handler(rd)
			// 检查 ctx 是否在 handler 执行期间被取消
			if ctxErr := ctx.Err(); ctxErr != nil {
				resultsCh <- branchResult{id: idx, result: nil, err: ctxErr}
				return
			}
			resultsCh <- branchResult{id: idx, result: r, err: err}
		})
		if subErr != nil {
			// pool 已被停止（通常因 ctx 取消）
			resultsCh <- branchResult{id: idx, result: nil, err: ctx.Err()}
			continue
		}
		submitted++
	}

	// 监听 ctx 取消 + 结果到达
	out := make(map[string]any, len(ids))
	var firstErr error
	collected := 0
	ctxCancelled := false
	for collected < submitted {
		select {
		case <-ctx.Done():
			// 父 ctx 已取消，标记后跳出循环。不直接 return 是因为需要
			// 在后台启动 pool.Stop() 清理 goroutine。
			ctxCancelled = true
			goto collectDone
		case br := <-resultsCh:
			collected++
			if br.err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("branch %q failed: %w", br.id, br.err)
				}
				continue
			}
			out[br.id] = br.result
		}
	}
collectDone:
	if ctxCancelled {
		// 在后台清理 pool（等待 in-flight 任务完成），避免阻塞 Execute 返回
		go pool.Stop()
		return nil, ctx.Err()
	}
	// 正常完成：同步等待 pool.Stop() 退出所有 worker
	pool.Stop()
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// readBranchHandlers 从 Options["branches"] 读取 map[string]Handler。
func readBranchHandlers(op *Operator) (map[string]Handler, error) {
	if op == nil || op.Options == nil {
		return nil, fmt.Errorf("options not set")
	}
	raw, ok := op.Options["branches"]
	if !ok || raw == nil {
		return nil, fmt.Errorf("branches not set")
	}
	branches, ok := raw.(map[string]Handler)
	if !ok {
		return nil, fmt.Errorf("branches: expected map[string]Handler, got %T", raw)
	}
	return branches, nil
}

// ============================================================================
// InterventionPointHandler (OpIntervention)
// 暂停执行等待外部干预恢复。
// 通过 Options["resume_signal"] 指定恢复信号 ID（缺省 "Resume[<name>]"）。
// 在 SignalNet 上等待该信号被接受后，返回信号的 Value（若 nil 则返回原 Input）。
// 同时发射 "Intervention[<name>]" 信号通知外部已暂停。
// ============================================================================

// InterventionPointHandler 干预点暂停算子。
type InterventionPointHandler struct{}

// Kind 返回 OpIntervention。
func (h *InterventionPointHandler) Kind() OperatorKind { return OpIntervention }

// Execute 发射 Intervention 信号，然后等待 Resume 信号被接受。
func (h *InterventionPointHandler) Execute(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("intervention_point handler: nil operator context")
	}
	if oc.SignalNet == nil {
		return nil, fmt.Errorf("intervention_point handler: nil signal net")
	}
	name := oc.Operator.Name
	interventionSigID := "Intervention[" + name + "]"
	resumeSigID := readResumeSignalID(oc.Operator, name)

	// 1. 发射暂停信号通知外部
	if oc.EmitSignal != nil {
		oc.EmitSignal(Signal{
			ID:           interventionSigID,
			TriggerEvent: interventionSigID,
			TriggerType:  SignalEvent,
			Value:        oc.Input,
		})
	}

	// 2. 等待恢复信号
	// BUG-22: 超时从 Options["timeout"] 读取（默认 5 分钟）。
	// 支持 time.Duration 或 string（time.ParseDuration 解析）。
	const pollInterval = 10 * time.Millisecond
	maxWait := readInterventionTimeout(oc.Operator)

	deadline := time.Now().Add(maxWait)
	for {
		if oc.SignalNet.IsAccepted(resumeSigID) {
			break
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("intervention_point handler: timed out waiting for resume signal %q", resumeSigID)
		}
		// 检查 context 是否已取消
		if oc.Ctx != nil {
			select {
			case <-oc.Ctx.Done():
				return nil, fmt.Errorf("intervention_point handler: context cancelled: %w", oc.Ctx.Err())
			default:
			}
		}
		time.Sleep(pollInterval)
	}

	// 3. 恢复后返回 resume 信号的 Value（若 nil 则返回原 Input）
	resumeSig := oc.SignalNet.GetAcceptedSignal(resumeSigID)
	if resumeSig != nil && resumeSig.Value != nil {
		return resumeSig.Value, nil
	}
	return oc.Input, nil
}

// readInterventionTimeout 从 Options["timeout"] 读取超时。
// 接受 time.Duration 或 string（time.ParseDuration 解析）。
// 默认 5 分钟。非法值回退到默认。
// BUG-22: 让 InterventionPoint 超时可配置。
func readInterventionTimeout(op *Operator) time.Duration {
	const defaultTimeout = 5 * time.Minute
	if op == nil || op.Options == nil {
		return defaultTimeout
	}
	raw, ok := op.Options["timeout"]
	if !ok || raw == nil {
		return defaultTimeout
	}
	switch v := raw.(type) {
	case time.Duration:
		if v <= 0 {
			return defaultTimeout
		}
		return v
	case string:
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return defaultTimeout
		}
		return d
	case int:
		if v <= 0 {
			return defaultTimeout
		}
		return time.Duration(v)
	case int64:
		if v <= 0 {
			return defaultTimeout
		}
		return time.Duration(v)
	case float64:
		if v <= 0 {
			return defaultTimeout
		}
		return time.Duration(v)
	default:
		return defaultTimeout
	}
}

// readResumeSignalID 从 Options["resume_signal"] 读取恢复信号 ID。
// 缺省返回 "Resume[<name>]"。
func readResumeSignalID(op *Operator, name string) string {
	if op != nil && op.Options != nil {
		if v, ok := op.Options["resume_signal"].(string); ok && v != "" {
			return v
		}
	}
	return "Resume[" + name + "]"
}

// ============================================================================
// SubFlowHandler (OpSubFlow)
// 子流嵌套：从 Options["child_flow_executor"] 或 Options["child_flow"] 读取子流引用，
// 调用它并返回结果。完整实现（SubFlowFrame + 注册表）见 subflow.go。
// 若两者都未提供，原样返回 Input。
// ============================================================================

// SubFlowHandler 子流嵌套算子。
type SubFlowHandler struct{}

// Kind 返回 OpSubFlow。
func (h *SubFlowHandler) Kind() OperatorKind { return OpSubFlow }

// Execute 调用子流处理 input，返回其结果。
// 完整逻辑（SubFlowFrame 创建/注册/清理）由 subflow.go 的 executeSubFlow 实现。
func (h *SubFlowHandler) Execute(oc *OperatorContext) (any, error) {
	return executeSubFlow(oc)
}

// readChildFlowExecutor 从 Options["child_flow_executor"] 读取 func(any) (any, error)。
func readChildFlowExecutor(op *Operator) func(any) (any, error) {
	if op == nil || op.Options == nil {
		return nil
	}
	raw, ok := op.Options["child_flow_executor"]
	if !ok || raw == nil {
		return nil
	}
	fn, ok := raw.(func(any) (any, error))
	if !ok || fn == nil {
		return nil
	}
	return fn
}

// ============================================================================
// ResultSinkHandler (OpResultSink)
// 结果汇聚算子：将 Input 标记为最终结果。
// 通过 EmitSignal 发射 "ResultSink[<name>]" 信号（含 Result=Input）。
// 同时将结果存入 SignalNet（便于 ResultSink 下游聚合）。
// 返回 Input。
// ============================================================================

// ResultSinkHandler 结果汇聚算子。
type ResultSinkHandler struct{}

// Kind 返回 OpResultSink。
func (h *ResultSinkHandler) Kind() OperatorKind { return OpResultSink }

// Execute 标记最终结果：发射 ResultSink 信号并返回 Input。
func (h *ResultSinkHandler) Execute(oc *OperatorContext) (any, error) {
	if oc == nil {
		return nil, fmt.Errorf("result_sink handler: nil operator context")
	}
	sig := Signal{
		ID:           "ResultSink[" + oc.Operator.Name + "]",
		TriggerEvent: "ResultSink[" + oc.Operator.Name + "]",
		TriggerType:  SignalEvent,
		Value:        oc.Input,
	}
	if oc.EmitSignal != nil {
		oc.EmitSignal(sig)
	}
	if oc.SignalNet != nil {
		oc.SignalNet.AcceptSignal(&sig)
	}
	return oc.Input, nil
}
