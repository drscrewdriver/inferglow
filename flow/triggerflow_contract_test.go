package flow

import (
	"strings"
	"testing"
)

// ============================================================================
// 测试用类型
// ============================================================================

// userInput / userResult / userStream 是测试用的具体类型。
type userInput struct {
	Name string
}
type userResult struct {
	Greeting string
}
type userStream struct {
	Chunk string
}

// stringResult 用于测试接口类型契约。
type greeter interface {
	Greet() string
}

type englishGreeter struct{ msg string }

func (g englishGreeter) Greet() string { return g.msg }

// ============================================================================
// Contract 构造与类型查询
// ============================================================================

func TestNewContract(t *testing.T) {
	c := NewContract[userInput, userStream, userResult]()
	if c == nil {
		t.Fatal("NewContract returned nil")
	}
	if c.InputType() == nil {
		t.Error("InputType() is nil")
	}
	if c.StreamType() == nil {
		t.Error("StreamType() is nil")
	}
	if c.ResultType() == nil {
		t.Error("ResultType() is nil")
	}
}

func TestContractTypeNames(t *testing.T) {
	c := NewContract[userInput, userStream, userResult]()
	if c.InputType().Name() != "userInput" {
		t.Errorf("InputType = %q, want userInput", c.InputType().Name())
	}
	if c.StreamType().Name() != "userStream" {
		t.Errorf("StreamType = %q, want userStream", c.StreamType().Name())
	}
	if c.ResultType().Name() != "userResult" {
		t.Errorf("ResultType = %q, want userResult", c.ResultType().Name())
	}
}

func TestContractString(t *testing.T) {
	c := NewContract[userInput, userStream, userResult]()
	s := c.String()
	if !strings.Contains(s, "userInput") {
		t.Errorf("String() = %q, want contains 'userInput'", s)
	}
	if !strings.Contains(s, "userStream") {
		t.Errorf("String() = %q, want contains 'userStream'", s)
	}
	if !strings.Contains(s, "userResult") {
		t.Errorf("String() = %q, want contains 'userResult'", s)
	}
}

// ============================================================================
// ValidateInput
// ============================================================================

func TestContractValidateInputCorrect(t *testing.T) {
	c := NewContract[userInput, userStream, userResult]()
	err := c.ValidateInput(userInput{Name: "Alice"})
	if err != nil {
		t.Errorf("ValidateInput correct type: %v", err)
	}
}

func TestContractValidateInputWrongType(t *testing.T) {
	c := NewContract[userInput, userStream, userResult]()
	err := c.ValidateInput(userResult{Greeting: "hi"})
	if err == nil {
		t.Error("ValidateInput with wrong type should fail")
	}
	if !strings.Contains(err.Error(), "type mismatch") {
		t.Errorf("error should mention type mismatch, got: %v", err)
	}
}

func TestContractValidateInputNil(t *testing.T) {
	c := NewContract[userInput, userStream, userResult]()
	err := c.ValidateInput(nil)
	if err == nil {
		t.Error("ValidateInput(nil) should fail")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("error should mention nil, got: %v", err)
	}
}

// ============================================================================
// ValidateOutput
// ============================================================================

func TestContractValidateOutputCorrect(t *testing.T) {
	c := NewContract[userInput, userStream, userResult]()
	err := c.ValidateOutput(userResult{Greeting: "hello"})
	if err != nil {
		t.Errorf("ValidateOutput correct type: %v", err)
	}
}

func TestContractValidateOutputWrongType(t *testing.T) {
	c := NewContract[userInput, userStream, userResult]()
	err := c.ValidateOutput(userInput{Name: "Bob"})
	if err == nil {
		t.Error("ValidateOutput with wrong type should fail")
	}
}

// ============================================================================
// ValidateStream
// ============================================================================

func TestContractValidateStreamCorrect(t *testing.T) {
	c := NewContract[userInput, userStream, userResult]()
	err := c.ValidateStream(userStream{Chunk: "data"})
	if err != nil {
		t.Errorf("ValidateStream correct type: %v", err)
	}
}

func TestContractValidateStreamWrongType(t *testing.T) {
	c := NewContract[userInput, userStream, userResult]()
	err := c.ValidateStream(userInput{Name: "wrong"})
	if err == nil {
		t.Error("ValidateStream with wrong type should fail")
	}
}

// ============================================================================
// 接口类型契约
// ============================================================================

func TestContractWithInterfaceType(t *testing.T) {
	// ResultT 为接口类型 greeter。
	c := NewContract[userInput, userStream, greeter]()
	// englishGreeter 实现了 greeter 接口，应通过校验。
	err := c.ValidateOutput(englishGreeter{msg: "hi"})
	if err != nil {
		t.Errorf("ValidateOutput with interface impl: %v", err)
	}
}

func TestContractWithInterfaceTypeRejectsNonImpl(t *testing.T) {
	c := NewContract[userInput, userStream, greeter]()
	// userInput 不实现 greeter 接口，应失败。
	err := c.ValidateOutput(userInput{Name: "no greet"})
	if err == nil {
		t.Error("ValidateOutput with non-implementing type should fail")
	}
	if !strings.Contains(err.Error(), "does not implement") {
		t.Errorf("error should mention 'does not implement', got: %v", err)
	}
}

// ============================================================================
// TriggerFlow 契约集成
// ============================================================================

func TestTriggerFlowContract(t *testing.T) {
	f := NewTriggerFlow[userInput, userStream, userResult]()
	inType, stType, rsType := f.Contract()
	if inType == nil || inType.Name() != "userInput" {
		t.Errorf("Contract input = %v, want userInput", inType)
	}
	if stType == nil || stType.Name() != "userStream" {
		t.Errorf("Contract stream = %v, want userStream", stType)
	}
	if rsType == nil || rsType.Name() != "userResult" {
		t.Errorf("Contract result = %v, want userResult", rsType)
	}
}

func TestTriggerFlowValidateResult(t *testing.T) {
	f := NewTriggerFlow[userInput, userStream, userResult]()
	// 正确类型。
	err := f.ValidateResult(userResult{Greeting: "hi"})
	if err != nil {
		t.Errorf("ValidateResult correct: %v", err)
	}
	// 错误类型。
	err = f.ValidateResult(userInput{Name: "wrong"})
	if err == nil {
		t.Error("ValidateResult wrong type should fail")
	}
}

func TestTriggerFlowValidateStream(t *testing.T) {
	f := NewTriggerFlow[userInput, userStream, userResult]()
	// 正确类型。
	err := f.ValidateStream(userStream{Chunk: "data"})
	if err != nil {
		t.Errorf("ValidateStream correct: %v", err)
	}
	// 错误类型。
	err = f.ValidateStream("wrong")
	if err == nil {
		t.Error("ValidateStream wrong type should fail")
	}
}

func TestTriggerFlowValidateInput(t *testing.T) {
	f := NewTriggerFlow[userInput, userStream, userResult]()
	// ValidateInput 接收 InputT，编译期已保证类型安全。
	err := f.ValidateInput(userInput{Name: "Alice"})
	if err != nil {
		t.Errorf("ValidateInput: %v", err)
	}
}

// ============================================================================
// 基本类型契约
// ============================================================================

func TestContractWithBasicTypes(t *testing.T) {
	c := NewContract[string, int, bool]()
	if err := c.ValidateInput("hello"); err != nil {
		t.Errorf("ValidateInput string: %v", err)
	}
	if err := c.ValidateInput(42); err == nil {
		t.Error("ValidateInput int should fail for string contract")
	}
	if err := c.ValidateStream(42); err != nil {
		t.Errorf("ValidateStream int: %v", err)
	}
	if err := c.ValidateOutput(true); err != nil {
		t.Errorf("ValidateOutput bool: %v", err)
	}
}

// ============================================================================
// 指针类型契约
// ============================================================================

func TestContractWithPointerTypes(t *testing.T) {
	c := NewContract[*userInput, *userStream, *userResult]()
	in := &userInput{Name: "Alice"}
	if err := c.ValidateInput(in); err != nil {
		t.Errorf("ValidateInput *userInput: %v", err)
	}
	// 值类型不应匹配指针契约。
	if err := c.ValidateInput(userInput{Name: "Alice"}); err == nil {
		t.Error("ValidateInput value should fail for pointer contract")
	}
}
