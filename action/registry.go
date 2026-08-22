// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
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

import "sort"

// SetSpiller attaches an optional post-execute spill policy to the registry.
// A nil spiller detaches any previous policy.
func (r *ActionRegistry) SetSpiller(sp OutputSpiller) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.spiller = sp
}

// Unregister removes a registered Action by name.
// Returns true if the action was found and removed, false otherwise.
func (r *ActionRegistry) Unregister(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.actions[name]; !ok {
		return false
	}
	delete(r.actions, name)
	return true
}

// Has reports whether an Action with the given name is registered.
func (r *ActionRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.actions[name]
	return ok
}

// Tag appends the given tags to the specified Actions.
// Actions that don't exist are silently skipped.
func (r *ActionRegistry) Tag(names []string, tags []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, name := range names {
		if action, ok := r.actions[name]; ok {
			for _, tag := range tags {
				action.Tags = append(action.Tags, tag)
			}
		}
	}
}

// ListActionNames returns the names of Actions whose Tags include
// all of the given tags. An empty tags slice matches every Action.
// Results are sorted alphabetically.
func (r *ActionRegistry) ListActionNames(tags []string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(tags) == 0 {
		ids := make([]string, 0, len(r.actions))
		for name := range r.actions {
			ids = append(ids, name)
		}
		return ids
	}

	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[tag] = true
	}

	var result []string
	for name, action := range r.actions {
		if hasAllTags(action.Tags, tagSet) {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

// GetAction returns the Action pointer for the given name, or nil if not found.
func (r *ActionRegistry) GetAction(name string) *Action {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.actions[name]
}

// GetTags returns a set of all tags on the named Action, or nil if not found.
func (r *ActionRegistry) GetTags(name string) map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	action, ok := r.actions[name]
	if !ok {
		return nil
	}
	tags := make(map[string]bool)
	for _, tag := range action.Tags {
		tags[tag] = true
	}
	return tags
}

// hasAllTags checks whether actionTags contains every tag in requiredTags.
func hasAllTags(actionTags []string, requiredTags map[string]bool) bool {
	for tag := range requiredTags {
		found := false
		for _, t := range actionTags {
			if t == tag {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
