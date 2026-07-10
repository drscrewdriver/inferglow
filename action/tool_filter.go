// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// to deal in the Software without restriction, including without limitation the rights
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

package action

// ToolFilter provides dynamic per-request tool filtering.
//
// Unlike static ActionSpec (per-action metadata) and Policy (registration-time
// selection), ToolFilter operates at request time. This enables use cases like:
//   - Plan mode: only read-only tools available (MaxLevel = SideEffectRead)
//   - Server API: X-Tool-Profile header selects predefined filter
//   - CLI: --tool-profile flag
//   - Skill: per-skill tool whitelist
//
// Filter precedence: Forbidden > Allowed > MaxLevel.
// An empty Allowed list means "allow all" (subject to Forbidden and MaxLevel).
type ToolFilter struct {
	// Allowed is a whitelist of tool names. Empty means "allow all".
	Allowed []string
	// Forbidden is a blacklist of tool names. Takes precedence over Allowed.
	Forbidden []string
	// MaxLevel is the maximum allowed side effect level.
	// Tools with SideEffectLevel > MaxLevel are excluded.
	// Zero value means "no limit".
	MaxLevel SideEffectLevel
}

// sideEffectOrder defines the severity ordering for side effect levels.
var sideEffectOrder = map[SideEffectLevel]int{
	"":              0, // unset = no limit
	SideEffectNone:    1,
	SideEffectRead:    2,
	SideEffectWrite:   3,
	SideEffectNetwork: 4,
	SideEffectExec:    5,
}

// IsAllowed reports whether the given action passes the filter.
//
// The check follows this logic:
//  1. If name is in Forbidden → rejected
//  2. If Allowed is non-empty and name is not in Allowed → rejected
//  3. If MaxLevel is set and action's SideEffectLevel exceeds it → rejected
//  4. Otherwise → allowed
func (f *ToolFilter) IsAllowed(name string, spec *ActionSpec) bool {
	if f == nil {
		return true
	}

	// 1. Forbidden blacklist takes precedence.
	for _, n := range f.Forbidden {
		if n == name {
			return false
		}
	}

	// 2. Allowed whitelist (empty = allow all).
	if len(f.Allowed) > 0 {
		found := false
		for _, n := range f.Allowed {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// 3. MaxLevel side effect check.
	if f.MaxLevel != "" && spec != nil {
		maxOrder := sideEffectOrder[f.MaxLevel]
		actionOrder := sideEffectOrder[spec.SideEffectLevel]
		if actionOrder > maxOrder {
			return false
		}
	}

	return true
}

// Apply returns the list of tool names from the registry that pass the filter.
// The returned slice is sorted alphabetically.
func (f *ToolFilter) Apply(registry *ActionRegistry) []string {
	if registry == nil {
		return nil
	}

	all := registry.List()
	if f == nil {
		return all
	}

	// Build a set of specs for side effect level checking.
	var result []string
	for _, name := range all {
		// Action doesn't carry ActionSpec directly; callers should
		// use ApplyWithSpecs for full side-effect filtering.
		if f.IsAllowed(name, nil) {
			result = append(result, name)
		}
	}
	return result
}

// ApplyWithSpecs filters tools using both the registry and a spec map.
// specs maps tool name → ActionSpec for side effect level checking.
func (f *ToolFilter) ApplyWithSpecs(registry *ActionRegistry, specs map[string]*ActionSpec) []string {
	if registry == nil {
		return nil
	}

	all := registry.List()
	if f == nil {
		return all
	}

	var result []string
	for _, name := range all {
		spec := specs[name]
		if f.IsAllowed(name, spec) {
			result = append(result, name)
		}
	}
	return result
}

// --- Predefined tool profiles ---

// ReadOnlyProfile returns a ToolFilter that only allows read-only tools.
// Suitable for plan mode and preview scenarios.
func ReadOnlyProfile() *ToolFilter {
	return &ToolFilter{
		MaxLevel: SideEffectRead,
	}
}

// BalancedProfile returns a ToolFilter that allows read + write tools
// but blocks network and exec tools.
func BalancedProfile() *ToolFilter {
	return &ToolFilter{
		MaxLevel: SideEffectWrite,
	}
}

// PermissiveProfile returns a ToolFilter that allows all tools.
// This is equivalent to a nil filter.
func PermissiveProfile() *ToolFilter {
	return &ToolFilter{}
}

// CustomProfile creates a ToolFilter with explicit allowed/forbidden lists.
func CustomProfile(allowed, forbidden []string) *ToolFilter {
	return &ToolFilter{
		Allowed:   allowed,
		Forbidden: forbidden,
	}
}

// Merge combines two filters. The result is the intersection of both filters'
// constraints. A nil filter is treated as "allow all".
func (f *ToolFilter) Merge(other *ToolFilter) *ToolFilter {
	if f == nil {
		return other
	}
	if other == nil {
		return f
	}

	result := &ToolFilter{
		MaxLevel: f.MaxLevel,
	}

	// Merge MaxLevel: take the stricter (lower) one.
	if other.MaxLevel != "" {
		if result.MaxLevel == "" || sideEffectOrder[other.MaxLevel] < sideEffectOrder[result.MaxLevel] {
			result.MaxLevel = other.MaxLevel
		}
	}

	// Merge Allowed: intersection if both are non-empty, else union.
	if len(f.Allowed) > 0 && len(other.Allowed) > 0 {
		allowedSet := make(map[string]bool, len(f.Allowed))
		for _, n := range f.Allowed {
			allowedSet[n] = true
		}
		for _, n := range other.Allowed {
			if allowedSet[n] {
				result.Allowed = append(result.Allowed, n)
			}
		}
	} else if len(f.Allowed) > 0 {
		result.Allowed = append(result.Allowed, f.Allowed...)
	} else if len(other.Allowed) > 0 {
		result.Allowed = append(result.Allowed, other.Allowed...)
	}

	// Merge Forbidden: union.
	forbiddenSet := make(map[string]bool)
	for _, n := range f.Forbidden {
		forbiddenSet[n] = true
		result.Forbidden = append(result.Forbidden, n)
	}
	for _, n := range other.Forbidden {
		if !forbiddenSet[n] {
			result.Forbidden = append(result.Forbidden, n)
		}
	}

	return result
}
