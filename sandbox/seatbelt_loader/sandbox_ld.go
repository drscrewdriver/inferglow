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

//go:build darwin && !cgo

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

// This file is the pure-Go fallback (option B) for the seatbelt loader. It is
// only compiled when cgo is disabled; the preferred option A (cgo, see
// sandbox_cgo.go) keeps the segment between sandbox_init and execvp entirely
// in C. With option B the Go runtime is active between sandbox_init and
// execvp, which is a documented, accepted risk: a normal Go runtime performs
// no filesystem writes in that window.
//
// The libsandbox symbols are imported at link time via //go:cgo_import_dynamic
// (the same mechanism the syscall package uses for libSystem on darwin, which
// works with CGO_ENABLED=0) and invoked from the assembly trampolines in
// call_darwin_amd64.s / call_darwin_arm64.s, mirroring the LazyProc style of
// sandbox/windows_syscall.go.

//go:cgo_import_dynamic libc_sandbox_init sandbox_init "/usr/lib/libsandbox.1.dylib"
//go:cgo_import_dynamic libc_sandbox_free_error sandbox_free_error "/usr/lib/libsandbox.1.dylib"

// callSandboxInit invokes sandbox_init(profile, flags, &errbuf) through the
// assembly trampoline and returns the raw sandbox_init return value (0 on
// success).
//
//go:noescape
func callSandboxInit(profile, flags, errbuf uintptr) uintptr

// callSandboxFreeError invokes sandbox_free_error(errbuf) through the
// assembly trampoline.
//
//go:noescape
func callSandboxFreeError(errbuf uintptr)

// sandboxApply applies the given SBPL profile to the current process.
func sandboxApply(profile string) error {
	p, err := syscall.BytePtrFromString(profile)
	if err != nil {
		return fmt.Errorf("encode profile: %w", err)
	}

	var errbuf *byte
	rc := callSandboxInit(uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&errbuf)))
	if rc != 0 {
		if errbuf != nil {
			msg := cstrToString(errbuf)
			callSandboxFreeError(uintptr(unsafe.Pointer(errbuf)))
			return fmt.Errorf("sandbox_init failed: %s", msg)
		}
		return fmt.Errorf("sandbox_init failed (rc=%d)", rc)
	}
	return nil
}

// cstrToString converts a NUL-terminated C string to a Go string.
// syscall.BytePtrToString is Windows-only, so this is a local helper.
func cstrToString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return unsafe.String(p, n)
}
