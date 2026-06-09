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

func TestNetworkAccessLevelRank(t *testing.T) {
	cases := []struct {
		level NetworkAccessLevel
		want  int
	}{
		{NetworkAccessNone, 0},
		{NetworkAccessEgressOnly, 1},
		{NetworkAccessFull, 2},
		{"", 0},            // empty → most restrictive
		{"bogus", 0},       // unknown → most restrictive (deny-by-default)
	}
	for _, c := range cases {
		if got := c.level.Rank(); got != c.want {
			t.Errorf("%q.Rank() = %d, want %d", c.level, got, c.want)
		}
	}
}

func TestMoreRestrictiveNetwork(t *testing.T) {
	cases := []struct {
		name string
		a, b NetworkAccessLevel
		want NetworkAccessLevel
	}{
		{"baseline none, llm full → none", NetworkAccessNone, NetworkAccessFull, NetworkAccessNone},
		{"baseline full, llm none → none", NetworkAccessFull, NetworkAccessNone, NetworkAccessNone},
		{"baseline egress_only, llm none → none", NetworkAccessEgressOnly, NetworkAccessNone, NetworkAccessNone},
		{"baseline egress_only, llm full → egress_only", NetworkAccessEgressOnly, NetworkAccessFull, NetworkAccessEgressOnly},
		{"baseline none, llm unspecified → none", NetworkAccessNone, "", NetworkAccessNone},
		{"baseline unspecified, llm full → none (deny default)", "", NetworkAccessFull, NetworkAccessNone},
		{"baseline full, llm unspecified → full", NetworkAccessFull, "", NetworkAccessFull},
		{"both unspecified → none", "", "", NetworkAccessNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MoreRestrictiveNetwork(c.a, c.b); got != c.want {
				t.Errorf("MoreRestrictiveNetwork(%q, %q) = %q, want %q", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestDefaultDenyBaseline(t *testing.T) {
	b := DefaultDenyBaseline()
	if b.NetworkAccess != NetworkAccessNone {
		t.Errorf("NetworkAccess = %q, want %q", b.NetworkAccess, NetworkAccessNone)
	}
	if b.ApprovalRequired != true {
		t.Errorf("ApprovalRequired = %v, want true", b.ApprovalRequired)
	}
	if b.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", b.Timeout)
	}
	if b.MaxOutputBytes <= 0 {
		t.Errorf("MaxOutputBytes = %d, want > 0", b.MaxOutputBytes)
	}
	if b.PathAllowlist != nil {
		t.Errorf("PathAllowlist = %v, want nil", b.PathAllowlist)
	}
	if b.IsZero() {
		t.Errorf("DefaultDenyBaseline().IsZero() = true, want false (baseline is configured)")
	}
}

func TestServerPolicyBaselineIsZero(t *testing.T) {
	if !(ServerPolicyBaseline{}).IsZero() {
		t.Errorf("zero-value ServerPolicyBaseline.IsZero() = false, want true")
	}
	// Setting any field makes it non-zero.
	if (ServerPolicyBaseline{ApprovalRequired: true}).IsZero() {
		t.Errorf("ServerPolicyBaseline{ApprovalRequired:true}.IsZero() = true, want false")
	}
	if (ServerPolicyBaseline{NetworkAccess: NetworkAccessFull}).IsZero() {
		t.Errorf("ServerPolicyBaseline{NetworkAccess:Full}.IsZero() = true, want false")
	}
}
