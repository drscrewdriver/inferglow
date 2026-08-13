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

//go:build windows

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// maxOutputBytesPerStream caps captured stdout/stderr per stream to bound
// memory usage when a sandboxed command produces excessive output. Data
// beyond the cap is still drained (so the child never blocks on a full
// pipe) but is discarded.
const maxOutputBytesPerStream = 16 << 20 // 16 MiB

// securityAttributes mirrors the Win32 SECURITY_ATTRIBUTES structure used
// by CreatePipe to mark the child-writable pipe end inheritable.
type securityAttributes struct {
	length             uint32
	securityDescriptor uintptr
	inheritHandle      uint32
}

// pipePair holds the read (parent-owned) and write (child-inherited) ends
// of one anonymous pipe.
type pipePair struct {
	read  syscall.Handle
	write syscall.Handle
}

// createPipePair creates an anonymous pipe and marks the write end as
// inheritable by child processes. The read end stays non-inheritable so it
// cannot leak into the child's handle table.
func createPipePair() (*pipePair, error) {
	var pair pipePair
	sa := securityAttributes{
		length:        uint32(unsafe.Sizeof(securityAttributes{})),
		inheritHandle: 1,
	}
	r1, _, err := procCreatePipe.Call(
		uintptr(unsafe.Pointer(&pair.read)),
		uintptr(unsafe.Pointer(&pair.write)),
		uintptr(unsafe.Pointer(&sa)),
		0,
	)
	if r1 == 0 {
		return nil, fmt.Errorf("CreatePipe: %w", err)
	}
	// Belt and braces: the read end must never be inherited.
	_ = syscall.SetHandleInformation(pair.read, handleFlagInherit, 0)
	return &pair, nil
}

// close closes both ends of the pipe, ignoring errors (already-closed ends
// are fine on this path).
func (p *pipePair) close() {
	if p.read != 0 {
		_ = syscall.CloseHandle(p.read)
		p.read = 0
	}
	if p.write != 0 {
		_ = syscall.CloseHandle(p.write)
		p.write = 0
	}
}

// closeWrite closes only the write end. The parent must release its copy
// right after CreateProcessAsUserW returns; otherwise the read ends never
// observe EOF once the child exits.
func (p *pipePair) closeWrite() {
	if p.write != 0 {
		_ = syscall.CloseHandle(p.write)
		p.write = 0
	}
}

// readPipe drains handle into buf until EOF, discarding anything beyond
// maxBytes so the child never blocks on a full pipe while memory stays
// bounded. It owns and closes handle.
func readPipe(handle syscall.Handle, buf *bytes.Buffer, maxBytes int) {
	defer syscall.CloseHandle(handle)
	tmp := make([]byte, 8192)
	var bytesRead uint32
	for {
		err := syscall.ReadFile(handle, tmp, &bytesRead, nil)
		n := int(bytesRead)
		if n > 0 {
			if remaining := maxBytes - buf.Len(); remaining > 0 {
				if n > remaining {
					n = remaining
				}
				buf.Write(tmp[:n])
			}
		}
		if err != nil {
			return // EOF or broken pipe
		}
	}
}

// jobObjectBasicLimitInformation mirrors the Win32
// JOBOBJECT_BASIC_LIMIT_INFORMATION fields needed to set the limit flags.
type jobObjectBasicLimitInformation struct {
	perProcessUserTimeLimit int64
	perJobUserTimeLimit     int64
	limitFlags              uint32
	minimumWorkingSetSize   uintptr
	maximumWorkingSetSize   uintptr
	activeProcessLimit      uint32
	affinity                uintptr
	priorityClass           uint32
	schedulingClass         uint32
}

// jobObjectExtendedLimitInfo mirrors the Win32
// JOBOBJECT_EXTENDED_LIMIT_INFORMATION layout up to the limit flags; the
// trailing counters are padding we never read.
type jobObjectExtendedLimitInfo struct {
	basicLimitInformation jobObjectBasicLimitInformation
	ioInfo                [6]uintptr
	processMemoryLimit    uintptr
	jobMemoryLimit        uintptr
	peakProcessMemoryUsed uintptr
	peakJobMemoryUsed     uintptr
}

// createKillOnCloseJob creates a job object configured with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE: closing the returned handle (or the
// process exiting) terminates every process assigned to the job. This is
// what makes cancellation kill the whole child process tree, not just the
// directly spawned process.
func createKillOnCloseJob() (syscall.Handle, error) {
	loadKernel32Procs()

	jobHandle, _, callErr := procCreateJobObjectW.Call(0, 0)
	if jobHandle == 0 {
		return 0, fmt.Errorf("CreateJobObjectW: %w", callErr)
	}

	info := jobObjectExtendedLimitInfo{}
	info.basicLimitInformation.limitFlags = jobObjectLimitKillOnJobClose
	r1, _, callErr := procSetInformationJobObject.Call(
		jobHandle,
		uintptr(jobObjectExtendedLimitInformation),
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Sizeof(info)),
	)
	if r1 == 0 {
		_ = syscall.CloseHandle(syscall.Handle(jobHandle))
		return 0, fmt.Errorf("SetInformationJobObject: %w", callErr)
	}
	return syscall.Handle(jobHandle), nil
}

// launchProcessWithIO launches argv under token with captured stdout/stderr.
//
// It wires anonymous pipes to the child's stdout/stderr
// (STARTF_USESTDHANDLES), reads both streams concurrently to avoid
// pipe-buffer deadlock, waits for the process with ctx cancellation
// (terminating the process on timeout), and returns the captured output,
// exit code, and wall-clock duration.
//
// token must be a primary token suitable for CreateProcessAsUserW; the
// calling token needs the SE_INCREASE_QUOTA_NAME privilege. cmd may be nil
// for an argv-only invocation; defaultDir is used as the working directory
// when cmd does not specify one.
func launchProcessWithIO(ctx context.Context, token syscall.Token, argv []string, cmd *Command, defaultDir string) (*ExecutionResult, error) {
	loadKernel32Procs()

	// CreateProcessAsUserW requires the caller to hold SeIncreaseQuotaPrivilege
	// ENABLED; it is present but disabled on normal user tokens. Fail closed
	// when the privilege cannot be enabled.
	if err := enableCurrentPrivilege("SeIncreaseQuotaPrivilege"); err != nil {
		return nil, fmt.Errorf("enable SeIncreaseQuotaPrivilege: %w", err)
	}

	if token == 0 {
		return nil, fmt.Errorf("launch process: %w: no token provided", ErrHandleNotRunning)
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("launch process: empty argv")
	}

	// Resolve the executable path and build the command line.
	name := argv[0]
	args := argv[1:]
	exePath, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("lookpath %q: %w", name, err)
	}
	cmdLine := exePath
	for _, a := range args {
		cmdLine += " " + syscall.EscapeArg(a)
	}
	cmdLinePtr, _ := syscall.UTF16PtrFromString(cmdLine)

	// Working directory.
	workDir := ""
	if cmd != nil {
		workDir = cmd.Workdir
	}
	if workDir == "" {
		workDir = defaultDir
	}
	var workDirPtr *uint16
	if workDir != "" {
		workDirPtr, _ = syscall.UTF16PtrFromString(workDir)
	}

	// Environment block.
	var envPtr *uint16
	if cmd != nil && len(cmd.Env) > 0 {
		envStr := strings.Join(cmd.Env, "\x00") + "\x00\x00"
		envPtr, _ = syscall.UTF16PtrFromString(envStr)
	}

	// stdout/stderr pipes.
	outPipe, err := createPipePair()
	if err != nil {
		return nil, err
	}
	defer outPipe.close()
	errPipe, err := createPipePair()
	if err != nil {
		return nil, err
	}
	defer errPipe.close()

	// STARTUPINFO with inherited std handles.
	var si syscall.StartupInfo
	var pi syscall.ProcessInformation
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags |= startfUseStdHandles
	si.StdOutput = outPipe.write
	si.StdErr = errPipe.write

	start := time.Now()
	r1, _, callErr := procCreateProcessAsUser.Call(
		uintptr(token),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(exePath))),
		uintptr(unsafe.Pointer(cmdLinePtr)),
		0, // lpProcessAttributes
		0, // lpThreadAttributes
		1, // bInheritHandles: TRUE so the child inherits the pipe write ends
		0, // dwCreationFlags
		uintptr(unsafe.Pointer(envPtr)),
		uintptr(unsafe.Pointer(workDirPtr)),
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if r1 == 0 {
		return nil, fmt.Errorf("CreateProcessAsUser: %w", callErr)
	}
	defer syscall.CloseHandle(pi.Process)
	defer syscall.CloseHandle(pi.Thread)

	// Assign the child to a kill-on-close job so cancellation terminates the
	// whole process tree (cmd.exe and its children like ping), not just the
	// directly spawned process. Assignment can legitimately fail when the
	// parent is already inside a job that forbids nesting; in that case we
	// degrade to terminating only the direct child. The job handle is closed
	// exactly once (see closeJob); on the cancellation path it is closed
	// BEFORE waiting for the readers so the whole tree dies and the pipes
	// reach EOF promptly.
	var job syscall.Handle
	jobClosed := false
	closeJob := func() {
		if job != 0 && !jobClosed {
			_ = syscall.CloseHandle(job)
			jobClosed = true
		}
	}
	if j, err := createKillOnCloseJob(); err == nil {
		job = j
		_, _, _ = procAssignProcessToJobObject.Call(uintptr(job), uintptr(pi.Process))
		defer closeJob()
	}

	// The parent must release its copies of the write ends immediately;
	// otherwise the read ends never observe EOF after the child exits.
	outPipe.closeWrite()
	errPipe.closeWrite()

	// Read both streams concurrently to avoid pipe-buffer deadlock.
	var stdoutBuf, stderrBuf bytes.Buffer
	readDone := make(chan struct{}, 2)
	go func() {
		readPipe(outPipe.read, &stdoutBuf, maxOutputBytesPerStream)
		readDone <- struct{}{}
	}()
	go func() {
		readPipe(errPipe.read, &stderrBuf, maxOutputBytesPerStream)
		readDone <- struct{}{}
	}()

	// Wait for the process with context support.
	done := make(chan error, 1)
	go func() {
		_, waitErr := syscall.WaitForSingleObject(pi.Process, syscall.INFINITE)
		done <- waitErr
	}()

	select {
	case <-ctx.Done():
		// Kill the child; the readers then hit EOF once its inherited
		// write-end copies close. TerminateProcess is synchronous (the
		// process is marked terminated before it returns), so the wait
		// goroutine always wakes promptly; we must NOT return before it
		// does, because the deferred CloseHandle would then race with the
		// still-blocked WaitForSingleObject over the same handle value.
		_ = syscall.TerminateProcess(pi.Process, 1)
		<-done
		// Closing the job kills the whole remaining process tree (children
		// of the direct child that still hold the pipe write ends), which
		// is what lets the readers below reach EOF promptly.
		closeJob()
		<-readDone
		<-readDone
		return nil, ctx.Err()
	case waitErr := <-done:
		if waitErr != nil {
			return nil, fmt.Errorf("WaitForSingleObject: %w", waitErr)
		}
	}

	// Get exit code.
	var exitCode uint32
	_ = syscall.GetExitCodeProcess(pi.Process, &exitCode)

	// Wait for both readers to finish draining the remaining output.
	<-readDone
	<-readDone

	return &ExecutionResult{
		ExitCode: int(exitCode),
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		Duration: time.Since(start),
	}, nil
}
