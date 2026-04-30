package flow

import (
	"fmt"
	"reflect"
	"runtime"
	"sync"
)

// HandlerRegistry 维护 name → handler 的注册表。
// 用于 CallableRef.Kind == CallableRegistered 的解析。
type HandlerRegistry struct {
	mu       sync.RWMutex
	handlers map[string]any
}

// NewHandlerRegistry 创建空注册表。
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[string]any),
	}
}

// Register 注册一个 handler 到指定 name。覆盖同名。
func (r *HandlerRegistry) Register(name string, handler any) error {
	if name == "" {
		return fmt.Errorf("handler name cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[name] = handler
	return nil
}

// Get 按 name 查找 handler。未找到返回 nil。
func (r *HandlerRegistry) Get(name string) any {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handlers[name]
}

// Has 检查 name 是否已注册。
func (r *HandlerRegistry) Has(name string) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.handlers[name]
	return ok
}

// Unregister 移除已注册的 handler。返回是否成功移除。
func (r *HandlerRegistry) Unregister(name string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.handlers[name]; !ok {
		return false
	}
	delete(r.handlers, name)
	return true
}

// Names 返回所有已注册的 handler 名称（无序）。
func (r *HandlerRegistry) Names() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		out = append(out, name)
	}
	return out
}

// GlobalHandlerRegistry 是全局共享的 handler 注册表。
// CallableRef.ResolveHandler 在 Kind == CallableRegistered 时使用它。
// 测试可通过 SetGlobalHandlerRegistry 替换为自定义实例以隔离测试状态。
var (
	globalRegistryMu sync.RWMutex
	globalRegistry   = NewHandlerRegistry()
)

// GlobalHandlerRegistry 返回当前全局 handler 注册表。
func GlobalHandlerRegistry() *HandlerRegistry {
	globalRegistryMu.RLock()
	defer globalRegistryMu.RUnlock()
	return globalRegistry
}

// SetGlobalHandlerRegistry 替换全局 handler 注册表。
// 主要用于测试隔离。传入 nil 会重置为新的空注册表。
func SetGlobalHandlerRegistry(r *HandlerRegistry) {
	globalRegistryMu.Lock()
	defer globalRegistryMu.Unlock()
	if r == nil {
		globalRegistry = NewHandlerRegistry()
		return
	}
	globalRegistry = r
}

// RegisterGlobalHandler 在全局注册表注册 handler。
// 等价于 GlobalHandlerRegistry().Register(name, handler)，但返回 error 便于链式使用。
func RegisterGlobalHandler(name string, handler any) error {
	return GlobalHandlerRegistry().Register(name, handler)
}

// ResetGlobalHandlerRegistry 重置全局注册表为新的空实例。
// 测试在每个用例开始时调用以隔离状态。
func ResetGlobalHandlerRegistry() {
	SetGlobalHandlerRegistry(NewHandlerRegistry())
}

// ============================================================================
// CallableRef 方法
// ============================================================================

// ResolveHandler 从 CallableRef 解析为实际 handler。
// 三种 Kind 行为：
//   - CallableRegistered: 从 GlobalHandlerRegistry 查找 ref.Name
//   - CallableInspected: 通过 reflect 加载 ref.Module/Qualname（当前实现有限支持，见下方说明）
//   - CallableAnonymous: 返回 error（匿名 handler 无法跨进程解析）
//
// inspected 解析说明：
// Go 反射无法像 Python 那样 import 模块并 getattr 函数。
// 当前实现：如果 GlobalHandlerRegistry 中存在 ref.Qualname 对应的 handler，则返回；
// 否则返回 error。这为编译期注册的 handler 提供了 inspected 通道的统一访问入口。
func (ref *CallableRef) ResolveHandler() (any, error) {
	if ref == nil {
		return nil, fmt.Errorf("callable ref is nil")
	}
	switch ref.Kind {
	case CallableRegistered:
		h := GlobalHandlerRegistry().Get(ref.Name)
		if h == nil {
			return nil, fmt.Errorf("handler %q not found in global registry", ref.Name)
		}
		return h, nil
	case CallableInspected:
		// Go 不支持运行时按模块路径反射加载函数；
		// 我们通过 Qualname 在全局注册表查找（编译期注册的 handler 也走 inspected 通道）
		if ref.Qualname != "" {
			if h := GlobalHandlerRegistry().Get(ref.Qualname); h != nil {
				return h, nil
			}
		}
		if ref.Name != "" && ref.Name != ref.Qualname {
			if h := GlobalHandlerRegistry().Get(ref.Name); h != nil {
				return h, nil
			}
		}
		return nil, fmt.Errorf("inspected handler %q (module %q) not resolvable: not in registry", ref.Qualname, ref.Module)
	case CallableAnonymous:
		return nil, fmt.Errorf("anonymous handler cannot be resolved across processes")
	default:
		return nil, fmt.Errorf("unknown CallableRef kind: %q", ref.Kind)
	}
}

// BuildCallableRef 从 handler 构建 CallableRef。
// 通过 reflect/runtime 提取函数信息。
// 如果 explicitName 非空，使用它作为 ref.Name；否则使用函数名。
// 如果 handler 已在 GlobalHandlerRegistry 中注册（按 explicitName 或函数名），
// 则 Kind=CallableRegistered；否则 Kind=CallableInspected。
//
// handler 必须是 func 类型，否则返回 error。
func BuildCallableRef(handler any, explicitName string) (*CallableRef, error) {
	if handler == nil {
		return nil, fmt.Errorf("handler cannot be nil")
	}
	v := reflect.ValueOf(handler)
	if v.Kind() != reflect.Func {
		return nil, fmt.Errorf("handler must be func, got %T", handler)
	}

	t := v.Type()
	// runtime.FuncForPC 需要 v 的指针（仅对 func 值有效）
	funcInfo := runtime.FuncForPC(v.Pointer())
	name := explicitName
	module := ""
	qualname := t.String()
	line := 0

	if funcInfo != nil {
		fullName := funcInfo.Name()
		// runtime 函数名格式：package.FuncName 或 package.Type.Method
		qualname = fullName
		module, name = splitRuntimeFuncName(fullName)
		if explicitName != "" {
			name = explicitName
		}
		// file:line 提取
		_, line = funcInfo.FileLine(v.Pointer())
	}

	kind := determineKind(handler, name)

	return &CallableRef{
		Kind:     kind,
		Name:     name,
		Module:   module,
		Qualname: qualname,
		Line:     line,
	}, nil
}

// splitRuntimeFuncName 从 runtime 函数全名中提取 package path 和 func name。
// 例如 "github.com/foo/bar.Baz" → ("github.com/foo/bar", "Baz")
// 例如 "github.com/foo/bar.(*Type).Method" → ("github.com/foo/bar", "(*Type).Method")
func splitRuntimeFuncName(fullName string) (pkgPath, funcName string) {
	if fullName == "" {
		return "", ""
	}
	// 查找最后一个 "/" 之后的部分作为 package name + func name
	// 然后查找第一个 "." 分隔 package 和 func
	lastSlash := -1
	for i := len(fullName) - 1; i >= 0; i-- {
		if fullName[i] == '/' {
			lastSlash = i
			break
		}
	}
	pkgAndFunc := fullName
	if lastSlash >= 0 {
		pkgAndFunc = fullName[lastSlash+1:]
	}
	// 第一个 "." 分隔 package name 和 func name
	dotIdx := -1
	for i, c := range pkgAndFunc {
		if c == '.' {
			dotIdx = i
			break
		}
	}
	if dotIdx < 0 {
		return "", pkgAndFunc
	}
	// pkgPath 是 fullName 去掉最后的 .FuncName 部分
	pkgPath = fullName[:len(fullName)-(len(pkgAndFunc)-dotIdx)]
	funcName = pkgAndFunc[dotIdx+1:]
	// 去掉 pkgPath 末尾的 "."
	if len(pkgPath) > 0 && pkgPath[len(pkgPath)-1] == '.' {
		pkgPath = pkgPath[:len(pkgPath)-1]
	}
	return pkgPath, funcName
}

// determineKind 判断 handler 的 CallableRef Kind。
// 如果 handler 在全局注册表中以 name 注册（且指针相同），则返回 CallableRegistered；
// 否则返回 CallableInspected。
func determineKind(handler any, name string) string {
	if name == "" {
		return CallableInspected
	}
	if GlobalHandlerRegistry().Has(name) {
		registered := GlobalHandlerRegistry().Get(name)
		if sameFuncPointer(registered, handler) {
			return CallableRegistered
		}
	}
	return CallableInspected
}

// sameFuncPointer 比较两个 any 是否为同一个 func 值（按指针）。
// 非 func 类型或任一为 nil 时返回 false。
func sameFuncPointer(a, b any) bool {
	if a == nil || b == nil {
		return false
	}
	va := reflect.ValueOf(a)
	vb := reflect.ValueOf(b)
	if va.Kind() != reflect.Func || vb.Kind() != reflect.Func {
		return false
	}
	return va.Pointer() == vb.Pointer()
}
