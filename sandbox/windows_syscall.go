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
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// Windows DLL handles loaded lazily and cached.
var (
	advapi32DLL sync.Once
	advapi32    *syscall.LazyDLL
	userenvDLL  sync.Once
	userenv     *syscall.LazyDLL
	kernel32DLL sync.Once
	kernel32    *syscall.LazyDLL

	// advapi32.dll procedures
	procOpenProcessToken    *syscall.LazyProc
	procDuplicateTokenEx    *syscall.LazyProc
	procCreateRestrictedToken *syscall.LazyProc
	procAdjustTokenPrivileges *syscall.LazyProc
	procLookupPrivilegeValue  *syscall.LazyProc
	procGetTokenInformation   *syscall.LazyProc

	// kernelbase.dll procedures (modern Windows forwards many advapi32
	// exports here; CreateAppContainerToken is not exported by advapi32)
	kernelbaseDLL sync.Once
	kernelbase    *syscall.LazyDLL
	procCreateAppContainerToken *syscall.LazyProc
	procCreateWellKnownSid      *syscall.LazyProc

	// userenv.dll procedures
	procCreateAppContainerProfile   *syscall.LazyProc
	procDeleteAppContainerProfile   *syscall.LazyProc
	procDeriveAppContainerSid       *syscall.LazyProc

	// kernel32.dll procedures
	procCreateProcessAsUser *syscall.LazyProc
	procCreatePipe          *syscall.LazyProc
	procLocalFree           *syscall.LazyProc
	procCreateJobObjectW    *syscall.LazyProc
	procSetInformationJobObject *syscall.LazyProc
	procAssignProcessToJobObject *syscall.LazyProc
)

// loadAdvapi32 loads advapi32.dll and resolves required procedures.
func loadAdvapi32() {
	advapi32DLL.Do(func() {
		advapi32 = syscall.NewLazyDLL("advapi32.dll")
		procOpenProcessToken = advapi32.NewProc("OpenProcessToken")
		procDuplicateTokenEx = advapi32.NewProc("DuplicateTokenEx")
		procCreateRestrictedToken = advapi32.NewProc("CreateRestrictedToken")
		procAdjustTokenPrivileges = advapi32.NewProc("AdjustTokenPrivileges")
		procLookupPrivilegeValue = advapi32.NewProc("LookupPrivilegeValueW")
		procGetTokenInformation = advapi32.NewProc("GetTokenInformation")
	})
}

// loadKernelbase loads kernelbase.dll and resolves procedures that modern
// Windows no longer exports from advapi32.dll (e.g. CreateAppContainerToken).
func loadKernelbase() {
	kernelbaseDLL.Do(func() {
		kernelbase = syscall.NewLazyDLL("kernelbase.dll")
		procCreateAppContainerToken = kernelbase.NewProc("CreateAppContainerToken")
		procCreateWellKnownSid = kernelbase.NewProc("CreateWellKnownSid")
	})
}

// loadUserenv loads userenv.dll and resolves required procedures.
func loadUserenv() {
	userenvDLL.Do(func() {
		userenv = syscall.NewLazyDLL("userenv.dll")
		procCreateAppContainerProfile = userenv.NewProc("CreateAppContainerProfile")
		procDeleteAppContainerProfile = userenv.NewProc("DeleteAppContainerProfile")
		procDeriveAppContainerSid = userenv.NewProc("DeriveAppContainerSidFromAppContainerName")
	})
}

// loadKernel32Procs resolves kernel32.dll procedures needed for process
// creation with output capture. It is guarded by sync.Once so concurrent
// Execute calls never race on the proc variables.
func loadKernel32Procs() {
	kernel32DLL.Do(func() {
		kernel32 = syscall.NewLazyDLL("kernel32.dll")
		procCreateProcessAsUser = kernel32.NewProc("CreateProcessAsUserW")
		procCreatePipe = kernel32.NewProc("CreatePipe")
		procLocalFree = kernel32.NewProc("LocalFree")
		procCreateJobObjectW = kernel32.NewProc("CreateJobObjectW")
		procSetInformationJobObject = kernel32.NewProc("SetInformationJobObject")
		procAssignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	})
}

// Token access rights.
const (
	tokenAssignPrimary = 0x0001
	tokenDuplicate     = 0x0002
	tokenQuery         = 0x0008
	tokenAdjustDefault = 0x0080
	tokenAdjustPrivileges = 0x0020
)

// Token types for DuplicateTokenEx.
const (
	tokenPrimary       = 1
	tokenImpersonation = 2
)

// TokenInformationClass values used by GetTokenInformation.
const (
	// TokenIsAppContainer (29) returns a DWORD that is nonzero when the
	// token is an AppContainer token.
	tokenIsAppContainer = 29
	// TokenAppContainerSid (31) returns a TOKEN_APPCONTAINER_INFORMATION
	// structure holding the AppContainer SID (or NULL for non-AC tokens).
	tokenAppContainerSid = 31
)

// Security impersonation levels.
const (
	securityAnonymous      = 0
	securityIdentification = 2
)

// CreateRestrictedToken flags.
const (
	disableMaxPrivileges  = 0x00000001
	sidTypeWellKnownGroup = 0x00000002
)

// TOKEN_PRIVILEGES structure for AdjustTokenPrivileges.
type tokenPrivileges struct {
	privilegeCount uint32
	privileges     [1]luidAndAttributes
}

// luidAndAttributes pairs a LUID with attributes.
type luidAndAttributes struct {
	luid       luid
	attributes uint32
}

// luid is a locally unique identifier.
type luid struct {
	lowPart  uint32
	highPart int32
}

// SE_PRIVILEGE_REMOVED is the attribute to remove a privilege.
const sePrivilegeRemoved = 0x00000004

// SE_PRIVILEGE_ENABLED is the attribute to enable a privilege.
const sePrivilegeEnabled = 0x00000002

// STARTUPINFO flags.
const startfUseStdHandles = 0x00000100

// SetHandleInformation flags.
const handleFlagInherit = 0x00000001

// Job object limits and information classes.
const (
	// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE terminates every process in the
	// job when the last job handle is closed.
	jobObjectLimitKillOnJobClose = 0x00002000
	// JobObjectExtendedLimitInformation carries the limit flags.
	jobObjectExtendedLimitInformation = 9
)

// highPrivileges is the list of high-privilege privilege names to remove
// from the restricted token.
var highPrivileges = []string{
	"SeDebugPrivilege",
	"SeTcbPrivilege",
	"SeBackupPrivilege",
	"SeRestorePrivilege",
	"SeTakeOwnershipPrivilege",
	"SeImpersonatePrivilege",
	"SeLoadDriverPrivilege",
	"SeSystemProfilePrivilege",
	"SeSystemtimePrivilege",
	"SeProfileSingleProcessPrivilege",
	"SeIncreaseBasePriorityPrivilege",
	"SeCreatePagefilePrivilege",
	"SeShutdownPrivilege",
}

// openCurrentProcessToken opens the current process token with the given access.
func openCurrentProcessToken(access uint32) (syscall.Token, error) {
	loadAdvapi32()
	p, _ := syscall.GetCurrentProcess()
	var token syscall.Token
	r1, _, err := procOpenProcessToken.Call(
		uintptr(p),
		uintptr(access),
		uintptr(unsafe.Pointer(&token)),
	)
	if r1 == 0 {
		return 0, fmt.Errorf("OpenProcessToken: %w", err)
	}
	return token, nil
}

// createRestrictedTokenFromCurrent creates a restricted token by duplicating
// the current process token and removing high-privilege privileges.
func createRestrictedTokenFromCurrent() (syscall.Token, error) {
	loadAdvapi32()

	// Step 1: Open current process token with enough access for
	// CreateProcessAsUserW (TOKEN_ASSIGN_PRIMARY is required on the token
	// passed to it) plus the privileges we need to strip.
	currentToken, err := openCurrentProcessToken(tokenDuplicate | tokenQuery | tokenAdjustPrivileges | tokenAssignPrimary)
	if err != nil {
		return 0, fmt.Errorf("open current token: %w", err)
	}
	defer currentToken.Close()

	// Step 2: Duplicate the token as a primary token.
	var dupToken syscall.Token
	r1, _, err := procDuplicateTokenEx.Call(
		uintptr(currentToken),
		0,
		0,
		uintptr(securityIdentification),
		uintptr(tokenPrimary),
		uintptr(unsafe.Pointer(&dupToken)),
	)
	if r1 == 0 {
		return 0, fmt.Errorf("DuplicateTokenEx: %w", err)
	}

	// Step 3: Remove high-privilege privileges.
	for _, privName := range highPrivileges {
		removePrivilege(dupToken, privName)
	}

	return dupToken, nil
}

// removePrivilege removes a single privilege from the token.
// Errors are silently ignored — the privilege may not exist.
func removePrivilege(token syscall.Token, privName string) {
	loadAdvapi32()

	// LookupPrivilegeValue to get the LUID.
	var luidVal luid
	privNamePtr, _ := syscall.UTF16PtrFromString(privName)
	r1, _, _ := procLookupPrivilegeValue.Call(
		0,
		uintptr(unsafe.Pointer(privNamePtr)),
		uintptr(unsafe.Pointer(&luidVal)),
	)
	if r1 == 0 {
		return // privilege not found, skip
	}

	// AdjustTokenPrivileges with SE_PRIVILEGE_REMOVED.
	tp := tokenPrivileges{
		privilegeCount: 1,
		privileges: [1]luidAndAttributes{
			{luid: luidVal, attributes: sePrivilegeRemoved},
		},
	}
	procAdjustTokenPrivileges.Call(
		uintptr(token),
		0,
		uintptr(unsafe.Pointer(&tp)),
		uintptr(unsafe.Sizeof(tp)),
		0,
		0,
	)
}

// createAppContainerProfile creates a new AppContainer profile with the given name.
// Returns the SID of the created profile.
//
// CreateAppContainerProfile returns an HRESULT: zero (S_OK) means success,
// any non-zero value is a failure code.
func createAppContainerProfile(name string) (*syscall.SID, error) {
	loadUserenv()

	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("encode name: %w", err)
	}
	displayNamePtr, _ := syscall.UTF16PtrFromString(name)
	descPtr, _ := syscall.UTF16PtrFromString("InferGlow sandbox profile")

	var sid *syscall.SID
	hr, _, callErr := procCreateAppContainerProfile.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(displayNamePtr)),
		uintptr(unsafe.Pointer(descPtr)),
		0, // pCapabilities: no capability SIDs at profile creation
		0, // dwCapabilityCount
		uintptr(unsafe.Pointer(&sid)),
	)
	if hr != 0 {
		return nil, fmt.Errorf("CreateAppContainerProfile: hr=0x%x: %w", uint32(hr), callErr)
	}
	return sid, nil
}

// deleteAppContainerProfile removes an AppContainer profile by name.
func deleteAppContainerProfile(name string) error {
	loadUserenv()

	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return fmt.Errorf("encode name: %w", err)
	}
	r1, _, callErr := procDeleteAppContainerProfile.Call(
		uintptr(unsafe.Pointer(namePtr)),
		0,
		0,
	)
	if r1 == 0 {
		return fmt.Errorf("DeleteAppContainerProfile: %w", callErr)
	}
	return nil
}

// deriveAppContainerSid derives the SID for an existing AppContainer profile.
//
// DeriveAppContainerSidFromAppContainerName returns an HRESULT: zero (S_OK)
// means success, any non-zero value is a failure code.
func deriveAppContainerSid(name string) (*syscall.SID, error) {
	loadUserenv()

	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("encode name: %w", err)
	}
	var sid *syscall.SID
	hr, _, callErr := procDeriveAppContainerSid.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&sid)),
	)
	if hr != 0 {
		return nil, fmt.Errorf("DeriveAppContainerSid: hr=0x%x: %w", uint32(hr), callErr)
	}
	return sid, nil
}

// sidAndAttributes pairs a SID with attribute flags (SID_AND_ATTRIBUTES).
type sidAndAttributes struct {
	sid        *syscall.SID
	attributes uint32
}

// securityCapabilities carries the AppContainer SID and optional capability
// list for CreateAppContainerToken (SECURITY_CAPABILITIES). A nil capability
// list yields a fully restricted container with no network, registry, or
// device access (deny-by-default).
type securityCapabilities struct {
	appContainerSid *syscall.SID
	capabilities    *sidAndAttributes
	capabilityCount uint32
	reserved        uint32
}

// createAppContainerToken derives an AppContainer token from the current
// process token for the given profile SID. The returned token is a primary
// token that CreateProcessAsUserW can start processes under, giving the
// child the AppContainer identity instead of the caller identity.
func createAppContainerToken(sid *syscall.SID) (syscall.Token, error) {
	loadKernelbase()

	baseToken, err := openCurrentProcessToken(tokenDuplicate | tokenQuery | tokenAssignPrimary)
	if err != nil {
		return 0, fmt.Errorf("open current token: %w", err)
	}
	defer baseToken.Close()

	secCaps := securityCapabilities{appContainerSid: sid}
	var appToken syscall.Token
	r1, _, callErr := procCreateAppContainerToken.Call(
		uintptr(baseToken),
		uintptr(unsafe.Pointer(&secCaps)),
		uintptr(unsafe.Pointer(&appToken)),
	)
	if r1 == 0 {
		return 0, fmt.Errorf("CreateAppContainerToken: %w", callErr)
	}
	return appToken, nil
}

// freeSID releases a SID allocated by an AppContainer API such as
// CreateAppContainerProfile. Win32 allocates these with LocalAlloc, so the
// caller must release them with LocalFree (see MSDN).
func freeSID(sid *syscall.SID) {
	if sid == nil {
		return
	}
	loadKernel32Procs()
	_, _, _ = procLocalFree.Call(uintptr(unsafe.Pointer(sid)))
}

// tokenAppContainerInfo mirrors TOKEN_APPCONTAINER_INFORMATION, the output
// layout of GetTokenInformation(TokenAppContainerSid). The returned SID
// points into the token itself and must not be freed by the caller.
type tokenAppContainerInfo struct {
	tokenAppContainerSid *syscall.SID
}

// tokenAppContainerSID queries the AppContainer SID of a token, or nil when
// the token is not an AppContainer token. It first confirms the token is an
// AppContainer token via TokenIsAppContainer, then reads the SID via
// TokenAppContainerSid.
//
// The TOKEN_APPCONTAINER_INFORMATION layout varies across Windows versions
// (8 bytes on older releases, extended on newer ones); the AppContainer SID
// is always the leading PSID member, so the query uses the two-stage
// size-then-allocate pattern and reads the first pointer-sized slot.
func tokenAppContainerSID(token syscall.Token) (*syscall.SID, error) {
	loadAdvapi32()

	// Step 1: TokenIsAppContainer (DWORD, nonzero = AppContainer token).
	var isAC uint32
	var returnLength uint32
	r1, _, callErr := procGetTokenInformation.Call(
		uintptr(token),
		uintptr(tokenIsAppContainer),
		uintptr(unsafe.Pointer(&isAC)),
		uintptr(unsafe.Sizeof(isAC)),
		uintptr(unsafe.Pointer(&returnLength)),
	)
	if r1 == 0 {
		return nil, fmt.Errorf("GetTokenInformation(TokenIsAppContainer): %w", callErr)
	}
	if isAC == 0 {
		return nil, nil
	}

	// Step 2: two-stage TokenAppContainerSid query.
	var need uint32
	var tiny [8]byte
	_, _, _ = procGetTokenInformation.Call(
		uintptr(token),
		uintptr(tokenAppContainerSid),
		uintptr(unsafe.Pointer(&tiny[0])),
		uintptr(len(tiny)),
		uintptr(unsafe.Pointer(&need)),
	)
	buf := make([]byte, need)
	r1, _, callErr = procGetTokenInformation.Call(
		uintptr(token),
		uintptr(tokenAppContainerSid),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&returnLength)),
	)
	if r1 == 0 {
		return nil, fmt.Errorf("GetTokenInformation(TokenAppContainerSid): %w", callErr)
	}
	if len(buf) < int(unsafe.Sizeof(uintptr(0))) {
		return nil, nil
	}
	return *(* *syscall.SID)(unsafe.Pointer(&buf[0])), nil
}

// grantDirectoryAccess grants full access to the given SID on path by
// invoking the built-in icacls utility with the SID-literal syntax (*SID).
//
// The * prefix makes icacls treat the argument as a SID instead of an
// account name, which is required for AppContainer SIDs that have no
// resolvable name. icacls merges the new ACE into the existing DACL, so
// pre-existing entries are preserved.
func grantDirectoryAccess(path string, sid *syscall.SID) error {
	sidStr, err := sid.String()
	if err != nil {
		return fmt.Errorf("format SID: %w", err)
	}
	cmd := exec.Command("icacls", path, "/grant", "*"+sidStr+":(OI)(CI)F")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("icacls grant %q: %w: %s", path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// enableCurrentPrivilege enables a privilege on the current process token
// (e.g. SeIncreaseQuotaPrivilege, required by CreateProcessAsUserW). The
// privilege must be present but disabled; enabling it is scoped to the
// calling process token and does not require elevation.
func enableCurrentPrivilege(name string) error {
	loadAdvapi32()

	token, err := openCurrentProcessToken(tokenAdjustPrivileges | tokenQuery)
	if err != nil {
		return fmt.Errorf("open current token: %w", err)
	}
	defer token.Close()

	// LookupPrivilegeValue to get the LUID.
	var luidVal luid
	namePtr, _ := syscall.UTF16PtrFromString(name)
	r1, _, _ := procLookupPrivilegeValue.Call(
		0,
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&luidVal)),
	)
	if r1 == 0 {
		return fmt.Errorf("LookupPrivilegeValue %q: privilege not found", name)
	}

	tp := tokenPrivileges{
		privilegeCount: 1,
		privileges: [1]luidAndAttributes{
			{luid: luidVal, attributes: sePrivilegeEnabled},
		},
	}
	r1, _, callErr := procAdjustTokenPrivileges.Call(
		uintptr(token),
		0,
		uintptr(unsafe.Pointer(&tp)),
		uintptr(unsafe.Sizeof(tp)),
		0,
		0,
	)
	if r1 == 0 {
		return fmt.Errorf("AdjustTokenPrivileges %q: %w", name, callErr)
	}
	return nil
}

// featureDetection checks if a specific DLL procedure exists.
// Returns true if the procedure can be found.
func featureDetection(dll *syscall.LazyDLL, procName string) bool {
	proc := dll.NewProc(procName)
	return proc.Find() == nil
}
