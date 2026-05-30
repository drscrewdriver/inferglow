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

package skill

import "context"

// SourceProvider is the interface for skill package sources.
type SourceProvider interface {
	// Name returns the source type (e.g. "local", "git").
	Name() string
	// Fetch retrieves the skill package from the source.
	Fetch(ctx context.Context, source string) (*SkillPackage, error)
}

// SkillPackage is a fetched skill package before installation.
type SkillPackage struct {
	// Source is the original source identifier.
	Source string `json:"source"`
	// Name is the package name.
	Name string `json:"name"`
	// Version is the package version.
	Version string `json:"version"`
	// Path is the local path where the package is stored.
	Path string `json:"path"`
}

// LocalSourceProvider loads skills from local filesystem paths.
type LocalSourceProvider struct{}

// Name returns "local".
func (LocalSourceProvider) Name() string { return "local" }

// Fetch loads a skill package from a local path.
func (LocalSourceProvider) Fetch(_ context.Context, source string) (*SkillPackage, error) {
	return &SkillPackage{
		Source:  source,
		Name:    source,
		Version: "local",
		Path:    source,
	}, nil
}
