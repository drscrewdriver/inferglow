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

#include "textflag.h"

// Assembly trampolines for the pure-Go seatbelt loader (option B).
// They follow the darwin/arm64 calling convention: arguments in R0/R1/R2,
// return value in R0. The libc_sandbox_* symbols are dynamic imports
// declared in sandbox_ld.go; CALLing a dynamic import is resolved through
// the dyld GOT by cmd/link.

// func callSandboxInit(profile, flags, errbuf uintptr) uintptr
TEXT ·callSandboxInit(SB), NOSPLIT, $0-32
	MOVD profile+0(FP), R0
	MOVD flags+8(FP), R1
	MOVD errbuf+16(FP), R2
	CALL libc_sandbox_init(SB)
	MOVD R0, ret+24(FP)
	RET

// func callSandboxFreeError(errbuf uintptr)
TEXT ·callSandboxFreeError(SB), NOSPLIT, $0-8
	MOVD errbuf+0(FP), R0
	CALL libc_sandbox_free_error(SB)
	RET
