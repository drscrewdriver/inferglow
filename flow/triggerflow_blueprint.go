package flow

import (
	"context"
	"fmt"
	"sync"
)

// ============================================================================
// TriggerFlowDefinition 数据结构
// ============================================================================

// FlowTriggerFlowDefinition 是 flow 包内部的 TriggerFlow 定义数据结构。
// 与 schema.TriggerFlowDefinition 不同，这里使用 flow.Operator 直接持有
// ListenSignals/EmitSignals/HandlerRef 等运行时字段，便于编译期到执行期的转换。
// 序列化时通过 schema.TriggerFlowDefinition 转换。
type FlowTriggerFlowDefinition struct {
	Version   string              `json:"version" yaml:"version"`
	Name      string              `json:"name" yaml:"name"`
	Operators []*Operator         `json:"operators" yaml:"operators"`
	Signals   map[string]string   `json:"signals,omitempty" yaml:"signals,omitempty"`
}

// NewFlowTriggerFlowDefinition 创建空的 FlowTriggerFlowDefinition。
func NewFlowTriggerFlowDefinition(name string) *FlowTriggerFlowDefinition {
	return &FlowTriggerFlowDefinition{
		Version:   "trigger_flow/v1",
		Name:      name,
		Operators: make([]*Operator, 0),
		Signals:   make(map[string]string),
	}
}

// AddOperator 添加算子到定义。
func (d *FlowTriggerFlowDefinition) AddOperator(op *Operator) {
	if d.Operators == nil {
		d.Operators = make([]*Operator, 0)
	}
	d.Operators = append(d.Operators, op)
}

// ============================================================================
// ResolveOperatorHandler - OperatorKind → OperatorHandler 默认映射
// ============================================================================

// ResolveOperatorHandler 根据 OperatorKind 返回对应的默认 OperatorHandler。
// 未注册的 Kind 返回 ErrOperatorNotImplemented。
func ResolveOperatorHandler(kind OperatorKind) (OperatorHandler, error) {
	switch kind {
	case OpChunk:
		return &ChunkHandler{}, nil
	case OpSignalGate:
		return &SignalGateHandler{}, nil
	case OpBatchFanout:
		return &BatchFanoutHandler{}, nil
	case OpBatchCollect:
		return &BatchCollectHandler{}, nil
	case OpForEachSplit:
		return &ForEachSplitHandler{}, nil
	case OpForEachCollect:
		return &ForEachCollectHandler{}, nil
	case OpMatchRoute:
		return &MatchRouteHandler{}, nil
	case OpMatchCase:
		return &MatchCaseHandler{}, nil
	case OpMatchCollect:
		return &MatchCollectHandler{}, nil
	case OpCollectBranch:
		return &CollectBranchHandler{}, nil
	case OpIntervention:
		return &InterventionPointHandler{}, nil
	case OpSubFlow:
		return &SubFlowHandler{}, nil
	case OpResultSink:
		return &ResultSinkHandler{}, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrOperatorNotImplemented, kind)
}

// ============================================================================
// TriggerFlowChunk - 封装 Handler 的可调用单元
// ============================================================================

// TriggerFlowChunk 封装一个用户注册的 handler，可异步调用。
// handler 字段支持以下类型：
//   - Handler (func(*TriggerFlowRuntimeData) (any, error))
//   - OperatorHandler (实现 Kind/Execute 接口)
//   - func(any) (any, error)（接收 Signal.Value）
//   - func(any) any（接收 Signal.Value，无 error）
type TriggerFlowChunk struct {
	ID       string
	Name     string
	handler  any
	trigger  string
	callable *CallableRef
}

// AsyncCall 异步调用 chunk 的 handler。
// 根据 handler 类型分发：
//   - Handler: 直接调用 handler(data)
//   - OperatorHandler: 构造 OperatorContext 后调用 Execute
//   - func(any) (any, error): 调用 handler(data.Signal.Value)
//   - func(any) any: 调用 handler(data.Signal.Value)
//   - 其他: 返回 error
func (c *TriggerFlowChunk) AsyncCall(data *TriggerFlowRuntimeData) (any, error) {
	if c == nil {
		return nil, fmt.Errorf("chunk is nil")
	}
	if data == nil {
		data = &TriggerFlowRuntimeData{
			RuntimeData: map[string]any{},
			FlowData:    map[string]any{},
		}
	}
	if data.RuntimeData == nil {
		data.RuntimeData = map[string]any{}
	}
	if data.FlowData == nil {
		data.FlowData = map[string]any{}
	}
	return c.invokeHandler(data)
}

// invokeHandler 根据 handler 类型分发调用。
func (c *TriggerFlowChunk) invokeHandler(data *TriggerFlowRuntimeData) (any, error) {
	h := c.handler
	if h == nil {
		return nil, fmt.Errorf("chunk %q: nil handler", c.Name)
	}
	switch fn := h.(type) {
	case Handler:
		return fn(data)
	case OperatorHandler:
		ctx := &OperatorContext{
			Ctx:      context.Background(),
			Operator: &Operator{ID: c.ID, Kind: fn.Kind(), Name: c.Name},
			Input:    extractInput(data),
		}
		return fn.Execute(ctx)
	case func(*TriggerFlowRuntimeData) (any, error):
		return fn(data)
	case func(any) (any, error):
		return fn(extractInput(data))
	case func(any) any:
		return fn(extractInput(data)), nil
	}
	return nil, fmt.Errorf("chunk %q: unsupported handler type %T", c.Name, h)
}

// extractInput 从 TriggerFlowRuntimeData 提取输入值。
// 优先使用 Signal.Value，其次使用 Result。
func extractInput(data *TriggerFlowRuntimeData) any {
	if data == nil {
		return nil
	}
	if data.Signal != nil && data.Signal.Value != nil {
		return data.Signal.Value
	}
	return data.Result
}

// ============================================================================
// TriggerFlowBlueprint - 定义期 Blueprint
// ============================================================================

// TriggerFlowBlueprint 是 flow 包的 TriggerFlow 定义期表示。
// 持有 chunks/handlers/chunkRegistry/definition 四类数据：
//   - chunks: 用户通过 CreateChunk 注册的 chunk 列表
//   - handlers: 编译后的三层 handler 映射（kind -> opID -> layer -> Handler）
//   - chunkRegistry: 可导出 handler 注册表
//   - definition: 算子列表定义
type TriggerFlowBlueprint struct {
	mu            sync.RWMutex
	chunks        map[string]*TriggerFlowChunk
	definition    *FlowTriggerFlowDefinition
	handlers      map[string]map[string]map[string]Handler // kind -> opID -> layer -> Handler
	chunkRegistry map[string]any                           // 可导出 handler 注册表
	compiled      bool
}

// NewTriggerFlowBlueprint 创建空的 TriggerFlowBlueprint。
func NewTriggerFlowBlueprint() *TriggerFlowBlueprint {
	return &TriggerFlowBlueprint{
		chunks:        make(map[string]*TriggerFlowChunk),
		handlers:      make(map[string]map[string]map[string]Handler),
		chunkRegistry: make(map[string]any),
		definition:    NewFlowTriggerFlowDefinition(""),
	}
}

// CreateChunk 注册一个 handler 为 chunk，返回 *TriggerFlowChunk。
// 同名 chunk 会被覆盖。
func (bp *TriggerFlowBlueprint) CreateChunk(handler any, name string) *TriggerFlowChunk {
	if bp == nil {
		return nil
	}
	bp.mu.Lock()
	defer bp.mu.Unlock()
	if bp.chunks == nil {
		bp.chunks = make(map[string]*TriggerFlowChunk)
	}
	chunk := &TriggerFlowChunk{
		ID:      fmt.Sprintf("chunk_%d", len(bp.chunks)),
		Name:    name,
		handler: handler,
	}
	bp.chunks[name] = chunk
	// 同步到 chunkRegistry（可导出）
	bp.chunkRegistry[name] = handler
	return chunk
}

// GetChunk 按 name 查找 chunk。
func (bp *TriggerFlowBlueprint) GetChunk(name string) (*TriggerFlowChunk, bool) {
	if bp == nil {
		return nil, false
	}
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	c, ok := bp.chunks[name]
	return c, ok
}

// ListChunks 返回所有已注册的 chunk。
func (bp *TriggerFlowBlueprint) ListChunks() []*TriggerFlowChunk {
	if bp == nil {
		return nil
	}
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	out := make([]*TriggerFlowChunk, 0, len(bp.chunks))
	for _, c := range bp.chunks {
		out = append(out, c)
	}
	return out
}

// AddOperator 添加算子到 definition。
func (bp *TriggerFlowBlueprint) AddOperator(op *Operator) {
	if bp == nil {
		return
	}
	bp.mu.Lock()
	defer bp.mu.Unlock()
	if bp.definition == nil {
		bp.definition = NewFlowTriggerFlowDefinition("")
	}
	bp.definition.AddOperator(op)
}

// SetDefinition 直接设置 definition。
func (bp *TriggerFlowBlueprint) SetDefinition(def *FlowTriggerFlowDefinition) {
	if bp == nil {
		return
	}
	bp.mu.Lock()
	defer bp.mu.Unlock()
	bp.definition = def
	bp.compiled = false
}

// GetDefinition 返回 definition。
func (bp *TriggerFlowBlueprint) GetDefinition() *FlowTriggerFlowDefinition {
	if bp == nil {
		return nil
	}
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.definition
}

// Compile 编译 Blueprint，将 operators 映射为 handlers。
// 编译后 handlers 三层映射包含每个算子的默认 Handler。
func (bp *TriggerFlowBlueprint) Compile() error {
	if bp == nil {
		return fmt.Errorf("blueprint is nil")
	}
	return bp._compileDefinition()
}

// _compileDefinition 实际编译逻辑。
// 遍历 definition.Operators，为每个 Kind 解析默认 OperatorHandler，
// 然后包装为 Handler 存入三层 handlers 映射。
// 映射结构为 kind -> opID -> layer -> Handler，这样同一 Kind 的多个
// Operator 不会互相覆盖。
func (bp *TriggerFlowBlueprint) _compileDefinition() error {
	if bp == nil {
		return fmt.Errorf("blueprint is nil")
	}
	bp.mu.Lock()
	defer bp.mu.Unlock()
	if bp.definition == nil {
		return fmt.Errorf("no definition to compile")
	}
	// 重置 handlers
	bp.handlers = make(map[string]map[string]map[string]Handler)
	for _, op := range bp.definition.Operators {
		if op == nil {
			continue
		}
		handler, err := ResolveOperatorHandler(op.Kind)
		if err != nil {
			return fmt.Errorf("resolve operator %s: %w", op.Kind, err)
		}
		// 包装 OperatorHandler 为 Handler
		wrapped := wrapOperatorHandlerAsHandler(handler, op)
		kindKey := string(op.Kind)
		if bp.handlers[kindKey] == nil {
			bp.handlers[kindKey] = make(map[string]map[string]Handler)
		}
		// Use op.ID as the composite key. If ID is empty, fall back to op.Name
		// to avoid silently merging distinct operators without IDs.
		opKey := op.ID
		if opKey == "" {
			opKey = op.Name
		}
		if bp.handlers[kindKey][opKey] == nil {
			bp.handlers[kindKey][opKey] = make(map[string]Handler)
		}
		bp.handlers[kindKey][opKey]["event"] = wrapped
		bp.handlers[kindKey][opKey]["flow_data"] = wrapped
		bp.handlers[kindKey][opKey]["runtime_data"] = wrapped
	}
	bp.compiled = true
	return nil
}

// wrapOperatorHandlerAsHandler 将 OperatorHandler 包装为 Handler。
// 包装后的 Handler 会构造 OperatorContext 并调用 Execute。
//
// 透传 TriggerFlowRuntimeData 中的 Ctx/SignalNet/EmitSignal 到 OperatorContext,
// 使得编译期注册的 OperatorHandler 能在运行时访问外层 context 和信号网络。
// 当 TriggerFlowRuntimeData 未携带这些字段（nil）时，Ctx 回退到 context.Background(),
// SignalNet/EmitSignal 保持 nil（与历史行为一致，向后兼容）。
func wrapOperatorHandlerAsHandler(h OperatorHandler, op *Operator) Handler {
	return func(data *TriggerFlowRuntimeData) (any, error) {
		ctx := &OperatorContext{
			Ctx:        context.Background(),
			Operator:   op,
			Input:      extractInput(data),
			SignalNet:  nil,
			EmitSignal: nil,
		}
		if data != nil {
			if data.Ctx != nil {
				ctx.Ctx = data.Ctx
			}
			ctx.SignalNet = data.SignalNet
			ctx.EmitSignal = data.EmitSignal
		}
		return h.Execute(ctx)
	}
}

// GetHandler 按 kind 和 layer 返回编译后的 handler。
// layer 取值："event" / "flow_data" / "runtime_data"。
// 注意：当同一 Kind 存在多个 Operator 时，本方法返回其中任意一个
// （map 迭代顺序不确定）。需要精确查找请使用 GetHandlerByID。
func (bp *TriggerFlowBlueprint) GetHandler(kind OperatorKind, layer string) (Handler, bool) {
	if bp == nil {
		return nil, false
	}
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	ops, ok := bp.handlers[string(kind)]
	if !ok {
		return nil, false
	}
	for _, layers := range ops {
		if h, ok := layers[layer]; ok {
			return h, true
		}
	}
	return nil, false
}

// GetHandlerByID 按 kind、opID 和 layer 返回编译后的 handler。
// opID 对应 Operator.ID（若 ID 为空则编译期回退到 Operator.Name）。
// layer 取值："event" / "flow_data" / "runtime_data"。
// 用于同一 Kind 存在多个 Operator 时精确查找。
func (bp *TriggerFlowBlueprint) GetHandlerByID(kind OperatorKind, opID, layer string) (Handler, bool) {
	if bp == nil {
		return nil, false
	}
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	ops, ok := bp.handlers[string(kind)]
	if !ok {
		return nil, false
	}
	layers, ok := ops[opID]
	if !ok {
		return nil, false
	}
	h, ok := layers[layer]
	return h, ok
}

// IsCompiled 返回编译状态。
func (bp *TriggerFlowBlueprint) IsCompiled() bool {
	if bp == nil {
		return false
	}
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.compiled
}

// ChunkRegistry 返回 chunkRegistry 的副本。
func (bp *TriggerFlowBlueprint) ChunkRegistry() map[string]any {
	if bp == nil {
		return nil
	}
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	out := make(map[string]any, len(bp.chunkRegistry))
	for k, v := range bp.chunkRegistry {
		out[k] = v
	}
	return out
}

// ============================================================================
// TriggerFlow[InputT, StreamT, ResultT] - 用户入口泛型结构
// ============================================================================

// TriggerFlow 是用户入口泛型结构。
// InputT: 流的输入类型
// StreamT: 流式数据类型
// ResultT: 流的最终结果类型
type TriggerFlow[InputT, StreamT, ResultT any] struct {
	blueprint      *TriggerFlowBlueprint
	skipExceptions bool
}

// NewTriggerFlow 创建一个 TriggerFlow 实例。
func NewTriggerFlow[InputT, StreamT, ResultT any]() *TriggerFlow[InputT, StreamT, ResultT] {
	return &TriggerFlow[InputT, StreamT, ResultT]{
		blueprint: NewTriggerFlowBlueprint(),
	}
}

// Blueprint 返回内部 Blueprint。
func (f *TriggerFlow[InputT, StreamT, ResultT]) Blueprint() *TriggerFlowBlueprint {
	if f == nil {
		return nil
	}
	return f.blueprint
}

// SkipExceptions 设置是否跳过异常。
func (f *TriggerFlow[InputT, StreamT, ResultT]) SkipExceptions(skip bool) *TriggerFlow[InputT, StreamT, ResultT] {
	if f == nil {
		return f
	}
	f.skipExceptions = skip
	return f
}

// CreateChunk 代理到 Blueprint.CreateChunk。
func (f *TriggerFlow[InputT, StreamT, ResultT]) CreateChunk(handler any, name string) *TriggerFlowChunk {
	if f == nil || f.blueprint == nil {
		return nil
	}
	return f.blueprint.CreateChunk(handler, name)
}

// AddOperator 代理到 Blueprint.AddOperator。
func (f *TriggerFlow[InputT, StreamT, ResultT]) AddOperator(op *Operator) *TriggerFlow[InputT, StreamT, ResultT] {
	if f == nil || f.blueprint == nil {
		return f
	}
	f.blueprint.AddOperator(op)
	return f
}

// Compile 代理到 Blueprint.Compile。
func (f *TriggerFlow[InputT, StreamT, ResultT]) Compile() error {
	if f == nil || f.blueprint == nil {
		return fmt.Errorf("flow or blueprint is nil")
	}
	return f.blueprint.Compile()
}

// Run 在编译后启动执行：按算子顺序逐个执行，返回最终结果。
// 这是简化的执行入口，完整的执行引擎由 engine.go 提供。
//
// 类型契约校验：
//   - 入参 input 类型由编译期保证（InputT）
//   - 最终结果通过运行期类型断言校验为 ResultT
//   - 若需预检最终结果类型，可在 Run 前调用 ValidateResult
func (f *TriggerFlow[InputT, StreamT, ResultT]) Run(input InputT) (ResultT, error) {
	var zero ResultT
	if f == nil || f.blueprint == nil {
		return zero, fmt.Errorf("flow or blueprint is nil")
	}
	if !f.blueprint.IsCompiled() {
		if err := f.blueprint.Compile(); err != nil {
			// 当 skipExceptions=true 时，编译期无法解析的算子可在运行循环里逐个跳过；
			// 否则直接返回编译错误。
			if !f.skipExceptions {
				return zero, fmt.Errorf("compile: %w", err)
			}
		}
	}
	def := f.blueprint.GetDefinition()
	if def == nil {
		return zero, fmt.Errorf("no definition")
	}
	// 创建本次执行的 SignalNet 和 EmitSignal 函数。
	// EmitSignal 将信号接受到 SignalNet，使 SignalGate/BatchCollect 等算子
	// 能在单进程内观察到上游算子发射的信号。
	signalNet := NewSignalNet()
	emitSignal := func(s Signal) {
		sig := s
		signalNet.AcceptSignal(&sig)
	}
	var currentInput any = input
	for _, op := range def.Operators {
		if op == nil {
			continue
		}
		h, err := ResolveOperatorHandler(op.Kind)
		if err != nil {
			if f.skipExceptions {
				continue
			}
			return zero, fmt.Errorf("operator %s: %w", op.Name, err)
		}
		ctx := &OperatorContext{
			Ctx:        context.Background(),
			Operator:   op,
			Input:      currentInput,
			SignalNet:  signalNet,
			EmitSignal: emitSignal,
		}
		out, err := h.Execute(ctx)
		if err != nil {
			if f.skipExceptions {
				continue
			}
			return zero, fmt.Errorf("operator %s: %w", op.Name, err)
		}
		if out != nil {
			currentInput = out
		}
	}
	result, ok := currentInput.(ResultT)
	if !ok {
		return zero, fmt.Errorf("result type mismatch: got %T, want %s", currentInput, typeOf[ResultT]())
	}
	return result, nil
}
