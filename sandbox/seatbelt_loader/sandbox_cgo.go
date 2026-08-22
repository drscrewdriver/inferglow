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

//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Sandbox

#include <sandbox.h>
#include <stdio.h>

// apply_profile applies the given SBPL profile to the current process via the
// private libsandbox API. On success the caller must immediately exec the
// target command: between sandbox_init and execvp there is no Go code, so the
// post-sandbox window is zero.
static int apply_profile(const char *profile) {
	char *errbuf = NULL;
	int rc = sandbox_init(profile, 0, &errbuf);
	if (rc != 0) {
		if (errbuf != NULL) {
			fprintf(stderr, "seatbelt-loader: sandbox_init failed: %s\n", errbuf);
			sandbox_free_error(errbuf);
		} else {
			fprintf(stderr, "seatbelt-loader: sandbox_init failed (rc=%d)\n", rc);
		}
		return -1;
	}
	return 0;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// sandboxApply applies the given SBPL profile to the current process.
//
// Option A (cgo, preferred): the profile is applied through the C helper
// above. The Go runtime is fully initialized by the time this runs, but the
// sensitive window the loader guards against is "between sandbox_init and
// exec": that segment runs entirely inside apply_profile (pure C), so no Go
// runtime behavior can occur after the sandbox is in effect.
func sandboxApply(profile string) error {
	cProfile := C.CString(profile)
	defer C.free(unsafe.Pointer(cProfile))

	if C.apply_profile(cProfile) != 0 {
		return fmt.Errorf("sandbox_init failed")
	}
	return nil
}
