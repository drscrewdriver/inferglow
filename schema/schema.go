package schema

import "reflect"

// DataType 定义支持的字段类型
type DataType string

const (
	TypeString   DataType = "str"
	TypeInt      DataType = "int"
	TypeFloat    DataType = "float"
	TypeBool     DataType = "bool"
	TypeDict     DataType = "dict"
	TypeList     DataType = "list"
	TypeModel    DataType = "model"
	TypeOptional DataType = "optional"
)

// EnsurePolicy 定义字段存在性策略
type EnsurePolicy string

const (
	EnsurePresence EnsurePolicy = "presence"
	EnsureNotNull  EnsurePolicy = "not_null"
)

// OutputFormat 定义输出格式
type OutputFormat string

const (
	OutputJSON       OutputFormat = "json"
	OutputMarkdown   OutputFormat = "markdown"
	OutputText       OutputFormat = "text"
	OutputFlatMarkdown OutputFormat = "flat_markdown"
	OutputHybrid     OutputFormat = "hybrid"
	OutputXMLField   OutputFormat = "xml_field"
	OutputYAMLLiteral OutputFormat = "yaml_literal"
	OutputAuto       OutputFormat = "auto"
)

// FieldDef 定义单个字段
type FieldDef struct {
	Type          DataType
	Description   string
	Ensure        EnsurePolicy
	Required      bool
	RequiredFields []string
	Children      map[string]*FieldDef
	ItemDef       *FieldDef
}

// OutputSchema 定义输出契约
type OutputSchema struct {
	Format    OutputFormat
	EnsureAll bool
	Fields    map[string]*FieldDef
}

// DefineOutput 泛型方法，用于定义输出 Schema。
// 当 T 为 Go struct 类型时，自动通过 reflect 推导字段：
//   - json tag 作为 field name（缺省时跳过该字段）
//   - description tag 作为字段描述
//   - json tag 含 ",omitempty" 时 Required=false，否则 Required=true
//   - 嵌套 struct 递归推导为 TypeDict + Children
//   - slice/array 推导为 TypeList，元素若为 struct 则写入 ItemDef.Children
//
// 当 T 为 map[string]any 等非 struct 类型时，返回空 Schema（保留旧行为）。
func DefineOutput[T any]() *OutputSchema {
	return DefineOutputFromType(reflect.TypeOf((*T)(nil)).Elem())
}
