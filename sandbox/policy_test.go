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

package sandbox

import (
	"reflect"
	"testing"
	"time"
)

func TestSandboxModeConstants(t *testing.T) {
	cases := []struct {
		name string
		got  SandboxMode
		want string
	}{
		{"ModeTrustedLocal", ModeTrustedLocal, "trusted_local"},
		{"ModeLocal", ModeLocal, "local"},
		{"ModeDocker", ModeDocker, "docker"},
		{"ModeGVisor", ModeGVisor, "gvisor"},
		{"ModeAuto", ModeAuto, "auto"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if string(c.got) != c.want {
				t.Errorf("%s = %q, want %q", c.name, string(c.got), c.want)
			}
			if c.got.String() != c.want {
				t.Errorf("%s.String() = %q, want %q", c.name, c.got.String(), c.want)
			}
		})
	}
}

func TestIsolationLevelConstants(t *testing.T) {
	cases := []struct {
		name string
		got  IsolationLevel
		want string
	}{
		{"LevelProcess", LevelProcess, "process"},
		{"LevelContainer", LevelContainer, "container"},
		{"LevelVM", LevelVM, "vm"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if string(c.got) != c.want {
				t.Errorf("%s = %q, want %q", c.name, string(c.got), c.want)
			}
		})
	}
}

func TestDefaultPolicy(t *testing.T) {
	p := DefaultPolicy()
	if p.SandboxMode != ModeTrustedLocal {
		t.Errorf("DefaultPolicy().SandboxMode = %q, want %q", p.SandboxMode, ModeTrustedLocal)
	}
	if p.IsolationLevel != LevelProcess {
		t.Errorf("DefaultPolicy().IsolationLevel = %q, want %q", p.IsolationLevel, LevelProcess)
	}
	// 其余字段应为零值
	if !reflect.DeepEqual(p.ResourceLimit, ResourceLimit{}) {
		t.Errorf("DefaultPolicy().ResourceLimit = %+v, want zero value", p.ResourceLimit)
	}
	if !reflect.DeepEqual(p.NetworkAccess, NetworkPolicy{}) {
		t.Errorf("DefaultPolicy().NetworkAccess = %+v, want zero value", p.NetworkAccess)
	}
	if !reflect.DeepEqual(p.FilesystemAccess, FilesystemPolicy{}) {
		t.Errorf("DefaultPolicy().FilesystemAccess = %+v, want zero value", p.FilesystemAccess)
	}
	if p.AllowedCommands != nil {
		t.Errorf("DefaultPolicy().AllowedCommands = %+v, want nil", p.AllowedCommands)
	}
	if p.Timeout != 0*time.Second {
		t.Errorf("DefaultPolicy().Timeout = %v, want 0", p.Timeout)
	}
}
