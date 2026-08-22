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

//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// These assertions run on real macOS and verify the loader's two-way sandbox
// semantics. sandbox_init is irreversible, so each assertion runs in a child
// process (the standard test-subprocess pattern) to keep the test binary
// itself un-sandboxed.

const (
	childDenyWriteEnv    = "SEATBELT_LOADER_DENY_WRITE_CHILD"
	childWorkspaceEnv    = "SEATBELT_LOADER_WORKSPACE_CHILD"
	childSelfTestEnv     = "SEATBELT_LOADER_SELFTEST_CHILD"
	childDenyWriteTarget = "/tmp/seatbelt-loader-pwned"
	childWorkspaceDir    = "/tmp/seatbelt-loader-workspace"
)

// TestSeatbeltLoaderDenyWriteAssertion verifies that a deny-write profile
// blocks writes outside the sandbox: the child must fail to create the target
// file and report that via its exit code.
func TestSeatbeltLoaderDenyWriteAssertion(t *testing.T) {
	if os.Getenv(childDenyWriteEnv) == "1" {
		runDenyWriteChild()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSeatbeltLoaderDenyWriteAssertion$")
	cmd.Env = append(os.Environ(), childDenyWriteEnv+"=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("deny-write assertion failed: %v (sandbox did not block the write)", err)
	}
	if _, err := os.Stat(childDenyWriteTarget); err == nil {
		t.Fatal("deny-write target file was created despite the deny profile")
	}
}

func runDenyWriteChild() {
	profile := "(version 1)\n(deny default)\n(allow process-exec)\n(deny file-write*)\n"
	if err := sandboxApply(profile); err != nil {
		os.Exit(2)
	}
	// Writing /tmp must be rejected with an error → assertion passes.
	if err := os.WriteFile(childDenyWriteTarget, []byte("x"), 0o644); err == nil {
		os.Exit(1)
	}
	os.Exit(0)
}

// TestSeatbeltLoaderWorkspaceWriteAssertion verifies that a workspace-write
// profile allows writes inside the sandboxed directory: the child must
// succeed in creating a file under the allowed path.
func TestSeatbeltLoaderWorkspaceWriteAssertion(t *testing.T) {
	if os.Getenv(childWorkspaceEnv) == "1" {
		runWorkspaceWriteChild()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSeatbeltLoaderWorkspaceWriteAssertion$")
	cmd.Env = append(os.Environ(), childWorkspaceEnv+"=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("workspace-write assertion failed: %v", err)
	}
}

func runWorkspaceWriteChild() {
	profile := "(version 1)\n(deny default)\n(allow process-exec)\n" +
		"(allow file-write* (subpath \"" + childWorkspaceDir + "\"))\n"
	if err := sandboxApply(profile); err != nil {
		os.Exit(2)
	}
	if err := os.MkdirAll(childWorkspaceDir, 0o755); err != nil {
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(childWorkspaceDir, "ok.txt"), []byte("x"), 0o644); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

// TestSeatbeltLoaderSelfTest verifies that applying the empty self-test
// profile succeeds, i.e. the private libsandbox API is usable (equivalent to
// the --self-test mode).
func TestSeatbeltLoaderSelfTest(t *testing.T) {
	if os.Getenv(childSelfTestEnv) == "1" {
		if err := sandboxApply(selfTestProfile); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSeatbeltLoaderSelfTest$")
	cmd.Env = append(os.Environ(), childSelfTestEnv+"=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("self-test assertion failed: %v", err)
	}
}
