package flowdef

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFile reads and parses a single YAML flow definition file. It
// auto-detects the format: if the YAML contains a "spec" field it is parsed
// as a structured FlowDef; if it contains a "stages" field (without "spec")
// it is treated as a simplified workflow and converted to FlowDef via
// ConvertSimpleToFlowDef.
func LoadFile(path string) (*FlowDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("flowdef: read %s: %w", path, err)
	}

	// Format detection: peek at top-level keys.
	var probe map[string]any
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("flowdef: probe %s: %w", path, err)
	}

	_, hasSpec := probe["spec"]
	_, hasStages := probe["stages"]

	if hasStages && !hasSpec {
		// Simplified workflow format (stages/next linked-list style).
		sw := &SimpleWorkflow{}
		if err := yaml.Unmarshal(data, sw); err != nil {
			return nil, fmt.Errorf("flowdef: parse simple %s: %w", path, err)
		}
		def, err := ConvertSimpleToFlowDef(sw)
		if err != nil {
			return nil, fmt.Errorf("flowdef: convert simple %s: %w", path, err)
		}
		return def, nil
	}

	// Structured FlowDef format (api_version/kind/metadata/spec).
	def := &FlowDef{}
	if err := yaml.Unmarshal(data, def); err != nil {
		return nil, fmt.Errorf("flowdef: parse %s: %w", path, err)
	}
	return def, nil
}

// LoadDir loads all *.yaml and *.yml files from a directory (non-recursively)
// and returns them keyed by metadata.name.
func LoadDir(dir string) (map[string]*FlowDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("flowdef: read dir %s: %w", dir, err)
	}
	// Sort entries by name for deterministic ordering.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	defs := make(map[string]*FlowDef)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		def, err := LoadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		key := def.Metadata.Name
		if key == "" {
			return nil, fmt.Errorf("flowdef: file %s has empty metadata.name", name)
		}
		defs[key] = def
	}
	return defs, nil
}
