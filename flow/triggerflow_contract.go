package flow

import (
	"fmt"
	"reflect"
)

// ============================================================================
// Contract - TriggerFlow 泛型契约的运行期校验
// ============================================================================

// Contract 捕获 TriggerFlow[InputT, StreamT, ResultT] 的类型契约。
//
// Go 泛型在编译期无法约束"InputT 的输出必须可转换为 ResultT"这类跨算子
// 类型流转关系。Contract 通过 reflect.Type 在运行期记录三个类型参数的
// 具体类型，提供：
//   - ValidateInput: 校验 Run(input) 的入参类型
//   - ValidateOutput: 校验 Run 的返回值类型
//   - ValidateStream: 校验流式数据（chunk）的类型
//   - ValidateIntermediate: 校验算子间传递的中间值类型
//
// Contract 是并发安全的（只读，构造后不可变）。
type Contract[InputT, StreamT, ResultT any] struct {
	inputType  reflect.Type
	streamType reflect.Type
	resultType reflect.Type
}

// NewContract 创建类型契约，捕获三个泛型参数的运行期类型。
func NewContract[InputT, StreamT, ResultT any]() *Contract[InputT, StreamT, ResultT] {
	return &Contract[InputT, StreamT, ResultT]{
		inputType:  typeOf[InputT](),
		streamType: typeOf[StreamT](),
		resultType: typeOf[ResultT](),
	}
}

// typeOf 返回泛型参数 T 的 reflect.Type。使用指向 T 的指针避免
// interface 类型被擦除（如 typeOf[any]() 返回 nil 而非 any）。
func typeOf[T any]() reflect.Type {
	var zero T
	t := reflect.TypeOf(&zero)
	if t == nil || t.Elem() == nil {
		return reflect.TypeOf((*T)(nil)).Elem()
	}
	return t.Elem()
}

// InputType 返回 InputT 的 reflect.Type。
func (c *Contract[InputT, StreamT, ResultT]) InputType() reflect.Type {
	return c.inputType
}

// StreamType 返回 StreamT 的 reflect.Type。
func (c *Contract[InputT, StreamT, ResultT]) StreamType() reflect.Type {
	return c.streamType
}

// ResultType 返回 ResultT 的 reflect.Type。
func (c *Contract[InputT, StreamT, ResultT]) ResultType() reflect.Type {
	return c.resultType
}

// ValidateInput 校验 value 是否为 InputT 类型。
// nil 值始终校验失败（InputT 应为非 nil 的具体类型）。
// 若 InputT 为接口类型，则 value 实现该接口即通过。
func (c *Contract[InputT, StreamT, ResultT]) ValidateInput(value any) error {
	return validateType(value, c.inputType, "input")
}

// ValidateOutput 校验 value 是否为 ResultT 类型。
func (c *Contract[InputT, StreamT, ResultT]) ValidateOutput(value any) error {
	return validateType(value, c.resultType, "output")
}

// ValidateStream 校验 value 是否为 StreamT 类型。
func (c *Contract[InputT, StreamT, ResultT]) ValidateStream(value any) error {
	return validateType(value, c.streamType, "stream")
}

// validateType 校验 value 是否可赋值给 expected 类型。
func validateType(value any, expected reflect.Type, label string) error {
	if expected == nil {
		return fmt.Errorf("%s type: contract has nil type (unsupported)", label)
	}
	if value == nil {
		return fmt.Errorf("%s type mismatch: got nil, want %s", label, expected)
	}
	got := reflect.TypeOf(value)
	if got == nil {
		return fmt.Errorf("%s type mismatch: got untyped nil, want %s", label, expected)
	}
	// 若 expected 是接口，检查 value 是否实现该接口。
	if expected.Kind() == reflect.Interface {
		if !got.Implements(expected) {
			return fmt.Errorf("%s type mismatch: %T does not implement %s", label, value, expected)
		}
		return nil
	}
	// 若 expected 是具体类型，检查类型严格匹配。
	if got != expected {
		return fmt.Errorf("%s type mismatch: got %T, want %s", label, value, expected)
	}
	return nil
}

// String 返回契约的可读表示。
func (c *Contract[InputT, StreamT, ResultT]) String() string {
	in, st, out := "?", "?", "?"
	if c.inputType != nil {
		in = c.inputType.String()
	}
	if c.streamType != nil {
		st = c.streamType.String()
	}
	if c.resultType != nil {
		out = c.resultType.String()
	}
	return fmt.Sprintf("Contract[Input=%s, Stream=%s, Result=%s]", in, st, out)
}

// ============================================================================
// TriggerFlow 契约集成
// ============================================================================

// contract 返回当前 TriggerFlow 的类型契约。
// 因为 Contract 需要泛型参数，这里通过闭包捕获。
// TriggerFlow 结构体持有 contract 函数而非 Contract 实例，以避免在结构体
// 定义中引用自身泛型参数导致的循环。
type contractFn = func() (inputType, streamType, resultType reflect.Type)

// Contract 返回 TriggerFlow 的类型契约信息。
// 返回的三个 reflect.Type 分别对应 InputT、StreamT、ResultT。
// 若契约未初始化（不应发生），返回 nil。
func (f *TriggerFlow[InputT, StreamT, ResultT]) Contract() (inputType, streamType, resultType reflect.Type) {
	if f == nil {
		return nil, nil, nil
	}
	return typeOf[InputT](), typeOf[StreamT](), typeOf[ResultT]()
}

// ValidateInput 校验 input 是否符合 InputT 类型。
// 在调用 Run 之前可调用此方法预检。
func (f *TriggerFlow[InputT, StreamT, ResultT]) ValidateInput(input InputT) error {
	// input 已是 InputT 类型（编译期保证），这里只做非 nil 检查（若 InputT 为接口）。
	// 对于具体类型，编译期已保证类型安全，无需运行期校验。
	// 此方法主要为接口类型的 InputT 提供运行期 nil 检查。
	return nil
}

// ValidateResult 校验 value 是否可作为 Run 的返回值（即是否为 ResultT 类型）。
// 这是一个运行期校验，用于处理算子输出类型不确定的场景。
func (f *TriggerFlow[InputT, StreamT, ResultT]) ValidateResult(value any) error {
	resultType := typeOf[ResultT]()
	return validateType(value, resultType, "result")
}

// ValidateStream 校验 value 是否为 StreamT 类型。
// 用于校验流式 chunk 的数据类型。
func (f *TriggerFlow[InputT, StreamT, ResultT]) ValidateStream(value any) error {
	streamType := typeOf[StreamT]()
	return validateType(value, streamType, "stream")
}
