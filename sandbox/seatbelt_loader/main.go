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

// Command seatbelt-loader applies a Seatbelt (libsandbox) profile to the
// current process and then replaces the process image with the target
// command (loader mode):
//
//	sandbox_init(profile)   ← sandbox the current process (irreversible, once)
//	execvp(cmd, argv)       ← replace the process image; the sandbox is a
//	                           process attribute and survives the exec
//
// The sandbox-exec CLI shipped by Apple has been deprecated since macOS
// 10.10, while the underlying private libsandbox API (sandbox_init) remains
// the core of App Sandbox / sandboxd / launchd sandbox configuration.
// This loader calls that API directly, so a working sandbox backend no
// longer depends on the presence of the deprecated CLI.
//
// Usage:
//
//	seatbelt-loader <profile-file> <cmd> <args...>
//	seatbelt-loader --self-test
//
// The profile is passed by file (avoiding ARG_MAX, aligned with the
// writeSBPLProfile semantics). Exit code convention: any loader failure
// (profile read, sandbox_init, exec) exits 125, mirroring the dsh landlock
// launcher convention; on success the exit code is the target command's.
package main

import (
	"fmt"
	"os"
	"syscall"
)

// exitCodeLoaderFailure is the unified exit code for loader failures,
// aligned with the dsh landlock launcher convention.
const exitCodeLoaderFailure = 125

// selfTestProfile is an empty SBPL profile used by --self-test: applying it
// succeeds iff the private libsandbox API is usable on this system.
const selfTestProfile = "(version 1)"

func main() {
	args := os.Args

	// Self-test mode: apply an empty profile, used by the provider probe.
	if len(args) >= 2 && args[1] == "--self-test" {
		if err := sandboxApply(selfTestProfile); err != nil {
			fatal(err)
		}
		os.Exit(0)
	}

	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: seatbelt-loader <profile-file> <cmd> <args...>")
		os.Exit(exitCodeLoaderFailure)
	}

	profile, err := os.ReadFile(args[1])
	if err != nil {
		fatal(fmt.Errorf("read profile %q: %w", args[1], err))
	}

	// Apply the SBPL profile to this process. Irreversible: after this call
	// the process is sandboxed until it exits.
	if err := sandboxApply(string(profile)); err != nil {
		fatal(err)
	}

	// Replace the process image with the target command. The sandbox is a
	// process attribute and is preserved across exec, so the target command
	// runs sandboxed. On success this call never returns.
	if err := syscall.Exec(args[2], args[2:], os.Environ()); err != nil {
		fatal(fmt.Errorf("exec %s: %w", args[2], err))
	}
}

// fatal prints a seatbelt-loader-prefixed error to stderr and exits with the
// loader failure code.
func fatal(err error) {
	fmt.Fprintf(os.Stderr, "seatbelt-loader: %v\n", err)
	os.Exit(exitCodeLoaderFailure)
}
