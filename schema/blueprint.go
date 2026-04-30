package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// FLOW_CONFIG_VERSION 是 TriggerFlow 配置的当前版本号。
const FLOW_CONFIG_VERSION = "trigger_flow/v1"

// FlowConfigOperator 描述一个算子的可序列化表示。
// 与 flow.Operator 结构对应，但只保留可序列化字段。
type FlowConfigOperator struct {
	Kind    string         `json:"kind" yaml:"kind"`
	Name    string         `json:"name" yaml:"name"`
	Input   string         `json:"input,omitempty" yaml:"input,omitempty"`
	Output  string         `json:"output,omitempty" yaml:"output,omitempty"`
	Options map[string]any `json:"options,omitempty" yaml:"options,omitempty"`
}

// TriggerFlowDefinition 是 TriggerFlow 的定义期表示。
// 它是 Blueprint 序列化/反序列化的核心数据结构。
type TriggerFlowDefinition struct {
	Version   string                 `json:"version" yaml:"version"`
	Name      string                 `json:"name" yaml:"name"`
	Operators []*FlowConfigOperator  `json:"operators" yaml:"operators"`
	Signals   map[string]string      `json:"signals,omitempty" yaml:"signals,omitempty"`
}

// NewTriggerFlowDefinition 创建空的 TriggerFlowDefinition，版本号自动设置。
func NewTriggerFlowDefinition(name string) *TriggerFlowDefinition {
	return &TriggerFlowDefinition{
		Version:   FLOW_CONFIG_VERSION,
		Name:      name,
		Operators: make([]*FlowConfigOperator, 0),
		Signals:   make(map[string]string),
	}
}

// AddOperator 添加一个算子到定义。
func (d *TriggerFlowDefinition) AddOperator(op *FlowConfigOperator) {
	if d.Operators == nil {
		d.Operators = make([]*FlowConfigOperator, 0)
	}
	d.Operators = append(d.Operators, op)
}

// Validate 校验 TriggerFlowDefinition 的完整性。
// 返回 error 描述第一个发现的问题。
func (d *TriggerFlowDefinition) Validate() error {
	if d == nil {
		return fmt.Errorf("definition is nil")
	}
	if d.Version != FLOW_CONFIG_VERSION {
		return fmt.Errorf("unsupported flow version: %s", d.Version)
	}
	if d.Name == "" {
		return fmt.Errorf("flow name cannot be empty")
	}
	seenNames := make(map[string]bool)
	for i, op := range d.Operators {
		if op == nil {
			return fmt.Errorf("operators[%d]: nil operator", i)
		}
		if op.Kind == "" {
			return fmt.Errorf("operators[%d]: kind cannot be empty", i)
		}
		if op.Name == "" {
			return fmt.Errorf("operators[%d]: name cannot be empty", i)
		}
		if seenNames[op.Name] {
			return fmt.Errorf("operators[%d]: duplicate operator name %q", i, op.Name)
		}
		seenNames[op.Name] = true
	}
	return nil
}

// TriggerFlowBlueprint 是 Blueprint 序列化层的最小表示。
// 它仅持有定义期数据（TriggerFlowDefinition），用于支持序列化/反序列化/导入/导出。
// flow 包会提供更丰富的 TriggerFlowBlueprint（含 handlers/chunks 等）。
type TriggerFlowBlueprint struct {
	Name       string
	Definition *TriggerFlowDefinition
}

// NewTriggerFlowBlueprint 创建空的 TriggerFlowBlueprint。
func NewTriggerFlowBlueprint(name string) *TriggerFlowBlueprint {
	return &TriggerFlowBlueprint{
		Name:       name,
		Definition: NewTriggerFlowDefinition(name),
	}
}

// BlueprintExporter 导出接口：将 Blueprint 转换为字典/JSON/YAML/Mermaid。
type BlueprintExporter interface {
	GetFlowConfig(name string, validate bool) (map[string]any, error)
	GetJSONFlow(name string) (string, error)
	GetYAMLFlow(name string) (string, error)
	ToMermaid(mode string) (string, error)
}

// BlueprintImporter 导入接口：从字典/JSON/YAML 还原 Blueprint。
type BlueprintImporter interface {
	LoadFlowConfig(config map[string]any, replace bool) (*TriggerFlowBlueprint, error)
	LoadJSONFlow(pathOrContent string, replace bool) (*TriggerFlowBlueprint, error)
	LoadYAMLFlow(pathOrContent string, replace bool) (*TriggerFlowBlueprint, error)
}

// DefinitionSerializer 序列化/反序列化核心。
// 提供 TriggerFlowDefinition 与 map[string]any / JSON / YAML / Mermaid 之间的双向转换。
// 同时实现 BlueprintExporter 和 BlueprintImporter 接口。
type DefinitionSerializer struct {
	// store 持有已加载的 Blueprint，按 name 索引。
	// LoadFlowConfig/LoadJSONFlow/LoadYAMLFlow 会将结果写入 store（replace=true 时覆盖）。
	store map[string]*TriggerFlowBlueprint
	// activeName 是当前活动的 Blueprint 名称，用于无 name 参数的方法（如 ToMermaid）。
	activeName string
}

// NewDefinitionSerializer 创建 DefinitionSerializer。
func NewDefinitionSerializer() *DefinitionSerializer {
	return &DefinitionSerializer{
		store: make(map[string]*TriggerFlowBlueprint),
	}
}

// GetBlueprint 按 name 从 store 查找 Blueprint。
func (s *DefinitionSerializer) GetBlueprint(name string) (*TriggerFlowBlueprint, bool) {
	bp, ok := s.store[name]
	return bp, ok
}

// SetActive 设置当前活动的 Blueprint 名称。
// 用于无 name 参数的方法（如 ToMermaid）确定操作目标。
func (s *DefinitionSerializer) SetActive(name string) error {
	if _, ok := s.store[name]; !ok {
		return fmt.Errorf("blueprint %q not found", name)
	}
	s.activeName = name
	return nil
}

// ActiveName 返回当前活动的 Blueprint 名称。
func (s *DefinitionSerializer) ActiveName() string {
	return s.activeName
}

// RegisterBlueprint 将 Blueprint 注册到 store（覆盖同名），并设为活动。
func (s *DefinitionSerializer) RegisterBlueprint(bp *TriggerFlowBlueprint) {
	if bp == nil {
		return
	}
	s.store[bp.Name] = bp
	s.activeName = bp.Name
}

// activeBlueprint 返回当前活动的 Blueprint；若未设置则取 store 中第一个。
func (s *DefinitionSerializer) activeBlueprint() (*TriggerFlowBlueprint, error) {
	if s.activeName != "" {
		if bp, ok := s.store[s.activeName]; ok {
			return bp, nil
		}
	}
	for _, bp := range s.store {
		return bp, nil
	}
	return nil, fmt.Errorf("no blueprint in store")
}

// ToDict 将 TriggerFlowDefinition 序列化为 map[string]any。
// 当 validate=true 时，先校验定义完整性，校验失败返回 error。
// name 用于设置返回字典中的 "name" 字段（覆盖 def.Name）。
func (s *DefinitionSerializer) ToDict(def *TriggerFlowDefinition, validate bool, name string) (map[string]any, error) {
	if def == nil {
		return nil, fmt.Errorf("definition is nil")
	}
	if validate {
		if err := def.Validate(); err != nil {
			return nil, fmt.Errorf("validate definition: %w", err)
		}
	}
	// 序列化 operators 列表
	ops := make([]any, 0, len(def.Operators))
	for _, op := range def.Operators {
		ops = append(ops, operatorToDict(op))
	}
	result := map[string]any{
		"version":   def.Version,
		"name":      name,
		"operators": ops,
	}
	if len(def.Signals) > 0 {
		signals := make(map[string]any, len(def.Signals))
		for k, v := range def.Signals {
			signals[k] = v
		}
		result["signals"] = signals
	}
	return result, nil
}

// operatorToDict 将 FlowConfigOperator 转换为 map[string]any。
func operatorToDict(op *FlowConfigOperator) map[string]any {
	if op == nil {
		return nil
	}
	m := map[string]any{
		"kind": op.Kind,
		"name": op.Name,
	}
	if op.Input != "" {
		m["input"] = op.Input
	}
	if op.Output != "" {
		m["output"] = op.Output
	}
	if len(op.Options) > 0 {
		m["options"] = op.Options
	}
	return m
}

// FromDict 从 map[string]any 反序列化为 TriggerFlowDefinition。
// 校验版本号；版本不匹配返回 error。
func (s *DefinitionSerializer) FromDict(config map[string]any) (*TriggerFlowDefinition, error) {
	if config == nil {
		return nil, fmt.Errorf("config is nil")
	}
	version, _ := config["version"].(string)
	if version != FLOW_CONFIG_VERSION {
		return nil, fmt.Errorf("unsupported flow version: %s", version)
	}
	name, _ := config["name"].(string)
	def := NewTriggerFlowDefinition(name)
	def.Version = version

	// 反序列化 operators
	rawOps, ok := config["operators"]
	if ok && rawOps != nil {
		opsList, err := toAnySlice(rawOps)
		if err != nil {
			return nil, fmt.Errorf("operators: %w", err)
		}
		for i, raw := range opsList {
			opMap, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("operators[%d]: expected map, got %T", i, raw)
			}
			op, err := operatorFromDict(opMap)
			if err != nil {
				return nil, fmt.Errorf("operators[%d]: %w", i, err)
			}
			def.AddOperator(op)
		}
	}

	// 反序列化 signals
	if rawSignals, ok := config["signals"]; ok && rawSignals != nil {
		if sigMap, ok := rawSignals.(map[string]any); ok {
			for k, v := range sigMap {
				if vs, ok := v.(string); ok {
					def.Signals[k] = vs
				}
			}
		}
	}
	return def, nil
}

// operatorFromDict 从 map[string]any 反序列化为 FlowConfigOperator。
func operatorFromDict(m map[string]any) (*FlowConfigOperator, error) {
	op := &FlowConfigOperator{}
	kind, ok := m["kind"].(string)
	if !ok || kind == "" {
		return nil, fmt.Errorf("kind missing or not string")
	}
	op.Kind = kind
	name, ok := m["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("name missing or not string")
	}
	op.Name = name
	if v, ok := m["input"].(string); ok {
		op.Input = v
	}
	if v, ok := m["output"].(string); ok {
		op.Output = v
	}
	if v, ok := m["options"].(map[string]any); ok {
		op.Options = v
	}
	return op, nil
}

// toAnySlice 将 []T 或 []any 转换为 []any。
func toAnySlice(v any) ([]any, error) {
	switch s := v.(type) {
	case []any:
		return s, nil
	case []map[string]any:
		out := make([]any, len(s))
		for i, m := range s {
			out[i] = m
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected slice, got %T", v)
	}
}

// ToJSON 将 TriggerFlowDefinition 序列化为 JSON 字符串。
func (s *DefinitionSerializer) ToJSON(def *TriggerFlowDefinition, validate bool, name string) (string, error) {
	dict, err := s.ToDict(def, validate, name)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(dict, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal to json: %w", err)
	}
	return string(data), nil
}

// ToYAML 将 TriggerFlowDefinition 序列化为 YAML 字符串。
func (s *DefinitionSerializer) ToYAML(def *TriggerFlowDefinition, validate bool, name string) (string, error) {
	dict, err := s.ToDict(def, validate, name)
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(dict)
	if err != nil {
		return "", fmt.Errorf("marshal to yaml: %w", err)
	}
	return string(data), nil
}

// FromJSON 从 JSON 字符串反序列化为 TriggerFlowDefinition。
func (s *DefinitionSerializer) FromJSON(content string) (*TriggerFlowDefinition, error) {
	var dict map[string]any
	if err := json.Unmarshal([]byte(content), &dict); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}
	return s.FromDict(dict)
}

// FromYAML 从 YAML 字符串反序列化为 TriggerFlowDefinition。
func (s *DefinitionSerializer) FromYAML(content string) (*TriggerFlowDefinition, error) {
	var dict map[string]any
	if err := yaml.Unmarshal([]byte(content), &dict); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}
	return s.FromDict(dict)
}

// renderMermaidFlow 渲染流程视图：
// 节点为算子名称，边按 input→output 连接（如果设置）。
func renderMermaidFlow(def *TriggerFlowDefinition) string {
	var sb strings.Builder
	sb.WriteString("flowchart TD\n")
	// 按 name 排序，保证输出稳定
	names := make([]string, 0, len(def.Operators))
	byName := make(map[string]*FlowConfigOperator, len(def.Operators))
	for _, op := range def.Operators {
		names = append(names, op.Name)
		byName[op.Name] = op
	}
	sort.Strings(names)
	for _, n := range names {
		op := byName[n]
		sb.WriteString(fmt.Sprintf("    %s[\"%s<br/>%s\"]\n", sanitizeMermaidID(n), n, op.Kind))
	}
	// 按 input/output 连接：input→output 都引用算子 name
	for _, n := range names {
		op := byName[n]
		if op.Input != "" {
			if _, ok := byName[op.Input]; ok {
				sb.WriteString(fmt.Sprintf("    %s --> %s\n", sanitizeMermaidID(op.Input), sanitizeMermaidID(n)))
			}
		}
	}
	return sb.String()
}

// renderMermaidSignal 渲染信号视图：
// 节点为信号事件，边为信号→算子（监听）和算子→信号（发射）。
func renderMermaidSignal(def *TriggerFlowDefinition) string {
	var sb strings.Builder
	sb.WriteString("flowchart LR\n")
	// 收集所有信号名（来自 Input/Output）
	signals := make(map[string]bool)
	for _, op := range def.Operators {
		if op.Input != "" {
			signals[op.Input] = true
		}
		if op.Output != "" {
			signals[op.Output] = true
		}
	}
	// 信号节点
	sigNames := make([]string, 0, len(signals))
	for s := range signals {
		sigNames = append(sigNames, s)
	}
	sort.Strings(sigNames)
	for _, s := range sigNames {
		sb.WriteString(fmt.Sprintf("    %s((%s))\n", sanitizeMermaidID(s), s))
	}
	// 算子节点 + 边
	for _, op := range def.Operators {
		opID := "op_" + sanitizeMermaidID(op.Name)
		sb.WriteString(fmt.Sprintf("    %s[%s]\n", opID, op.Name))
		if op.Input != "" {
			sb.WriteString(fmt.Sprintf("    %s --> %s\n", sanitizeMermaidID(op.Input), opID))
		}
		if op.Output != "" {
			sb.WriteString(fmt.Sprintf("    %s --> %s\n", opID, sanitizeMermaidID(op.Output)))
		}
	}
	return sb.String()
}

// sanitizeMermaidID 将任意字符串转换为合法的 Mermaid 节点 ID。
// Mermaid 节点 ID 只允许字母、数字、下划线，其他字符替换为下划线。
func sanitizeMermaidID(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	if sb.Len() == 0 {
		return "_"
	}
	return sb.String()
}

// ============================================================================
// BlueprintExporter / BlueprintImporter 接口的默认实现
// ============================================================================

// GetFlowConfig 实现 BlueprintExporter：返回指定 name 的 Blueprint 字典表示。
// validate=true 时校验定义；replace 不影响导出（仅为接口一致性保留）。
func (s *DefinitionSerializer) GetFlowConfig(name string, validate bool) (map[string]any, error) {
	bp, ok := s.store[name]
	if !ok {
		return nil, fmt.Errorf("blueprint %q not found", name)
	}
	return s.ToDict(bp.Definition, validate, name)
}

// GetJSONFlow 实现 BlueprintExporter：返回指定 name 的 Blueprint JSON 字符串。
func (s *DefinitionSerializer) GetJSONFlow(name string) (string, error) {
	bp, ok := s.store[name]
	if !ok {
		return "", fmt.Errorf("blueprint %q not found", name)
	}
	return s.ToJSON(bp.Definition, false, name)
}

// GetYAMLFlow 实现 BlueprintExporter：返回指定 name 的 Blueprint YAML 字符串。
func (s *DefinitionSerializer) GetYAMLFlow(name string) (string, error) {
	bp, ok := s.store[name]
	if !ok {
		return "", fmt.Errorf("blueprint %q not found", name)
	}
	return s.ToYAML(bp.Definition, false, name)
}

// ToMermaid 实现 BlueprintExporter.ToMermaid：
// 返回当前活动 Blueprint 的 Mermaid 图。
// 使用 SetActive 切换活动 Blueprint；默认为最后加载/注册的 Blueprint。
// 若需对任意 TriggerFlowDefinition 渲染 Mermaid，请使用 ToMermaidDef(def, mode)。
func (s *DefinitionSerializer) ToMermaid(mode string) (string, error) {
	bp, err := s.activeBlueprint()
	if err != nil {
		return "", err
	}
	return s.ToMermaidDef(bp.Definition, mode)
}

// ToMermaidDef 将任意 TriggerFlowDefinition 渲染为 Mermaid 流程图。
// 这是 DefinitionSerializer.ToMermaid 的"显式 def"版本。
// mode="flow"（默认）：节点为算子，边为数据流；
// mode="signal"：节点为信号事件，边为信号传递关系。
func (s *DefinitionSerializer) ToMermaidDef(def *TriggerFlowDefinition, mode string) (string, error) {
	if def == nil {
		return "", fmt.Errorf("definition is nil")
	}
	if mode == "" {
		mode = "flow"
	}
	switch mode {
	case "flow":
		return renderMermaidFlow(def), nil
	case "signal":
		return renderMermaidSignal(def), nil
	default:
		return "", fmt.Errorf("unsupported mermaid mode: %s (want 'flow' or 'signal')", mode)
	}
}

// LoadFlowConfig 实现 BlueprintImporter：从字典加载 Blueprint。
// replace=true 时覆盖同名 Blueprint；replace=false 时若已存在则返回 error。
// 加载成功后，该 Blueprint 成为活动 Blueprint。
func (s *DefinitionSerializer) LoadFlowConfig(config map[string]any, replace bool) (*TriggerFlowBlueprint, error) {
	def, err := s.FromDict(config)
	if err != nil {
		return nil, err
	}
	bp := &TriggerFlowBlueprint{
		Name:       def.Name,
		Definition: def,
	}
	if !replace {
		if _, exists := s.store[bp.Name]; exists {
			return nil, fmt.Errorf("blueprint %q already exists (replace=false)", bp.Name)
		}
	}
	s.store[bp.Name] = bp
	s.activeName = bp.Name
	return bp, nil
}

// LoadJSONFlow 实现 BlueprintImporter：从 JSON 字符串或文件加载 Blueprint。
// pathOrContent 以 ".json" 结尾视为文件路径；否则视为 JSON 内容。
// replace=true 时覆盖同名 Blueprint。加载成功后该 Blueprint 成为活动 Blueprint。
func (s *DefinitionSerializer) LoadJSONFlow(pathOrContent string, replace bool) (*TriggerFlowBlueprint, error) {
	content, err := readPathOrContent(pathOrContent, ".json")
	if err != nil {
		return nil, err
	}
	def, err := s.FromJSON(content)
	if err != nil {
		return nil, err
	}
	bp := &TriggerFlowBlueprint{
		Name:       def.Name,
		Definition: def,
	}
	if !replace {
		if _, exists := s.store[bp.Name]; exists {
			return nil, fmt.Errorf("blueprint %q already exists (replace=false)", bp.Name)
		}
	}
	s.store[bp.Name] = bp
	s.activeName = bp.Name
	return bp, nil
}

// LoadYAMLFlow 实现 BlueprintImporter：从 YAML 字符串或文件加载 Blueprint。
// pathOrContent 以 ".yaml" 或 ".yml" 结尾视为文件路径；否则视为 YAML 内容。
// replace=true 时覆盖同名 Blueprint。加载成功后该 Blueprint 成为活动 Blueprint。
func (s *DefinitionSerializer) LoadYAMLFlow(pathOrContent string, replace bool) (*TriggerFlowBlueprint, error) {
	content, err := readPathOrContent(pathOrContent, ".yaml", ".yml")
	if err != nil {
		return nil, err
	}
	def, err := s.FromYAML(content)
	if err != nil {
		return nil, err
	}
	bp := &TriggerFlowBlueprint{
		Name:       def.Name,
		Definition: def,
	}
	if !replace {
		if _, exists := s.store[bp.Name]; exists {
			return nil, fmt.Errorf("blueprint %q already exists (replace=false)", bp.Name)
		}
	}
	s.store[bp.Name] = bp
	s.activeName = bp.Name
	return bp, nil
}

// readPathOrContent 根据 pathOrContent 判断是文件路径还是直接内容。
// 如果以 suffixes 中任一后缀结尾，则视为文件路径并读取；否则视为直接内容。
func readPathOrContent(pathOrContent string, suffixes ...string) (string, error) {
	for _, suf := range suffixes {
		if strings.HasSuffix(pathOrContent, suf) {
			data, err := os.ReadFile(pathOrContent)
			if err != nil {
				return "", fmt.Errorf("read file %s: %w", pathOrContent, err)
			}
			return string(data), nil
		}
	}
	return pathOrContent, nil
}
