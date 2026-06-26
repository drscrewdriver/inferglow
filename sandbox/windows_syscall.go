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

	// advapi32.dll procedures
	procOpenProcessToken    *syscall.LazyProc
	procDuplicateTokenEx    *syscall.LazyProc
	procCreateRestrictedToken *syscall.LazyProc
	procAdjustTokenPrivileges *syscall.LazyProc
	procLookupPrivilegeValue  *syscall.LazyProc

	// userenv.dll procedures
	procCreateAppContainerProfile   *syscall.LazyProc
	procDeleteAppContainerProfile   *syscall.LazyProc
	procDeriveAppContainerSid       *syscall.LazyProc

	// kernel32.dll procedures (already available via syscall package)
	procCreateProcessAsUser *syscall.LazyProc
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

// loadKernel32Procs resolves kernel32.dll procedures needed for CreateProcessAsUser.
func loadKernel32Procs() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	procCreateProcessAsUser = kernel32.NewProc("CreateProcessAsUserW")
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

	// Step 1: Open current process token.
	currentToken, err := openCurrentProcessToken(tokenDuplicate | tokenQuery | tokenAdjustPrivileges)
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
func createAppContainerProfile(name string) (*syscall.SID, error) {
	loadUserenv()

	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("encode name: %w", err)
	}
	displayNamePtr, _ := syscall.UTF16PtrFromString(name)
	descPtr, _ := syscall.UTF16PtrFromString("InferGlow sandbox profile")

	var sid *syscall.SID
	r1, _, callErr := procCreateAppContainerProfile.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(displayNamePtr)),
		uintptr(unsafe.Pointer(descPtr)),
		uintptr(unsafe.Pointer(&sid)),
	)
	if r1 == 0 {
		return nil, fmt.Errorf("CreateAppContainerProfile: %w", callErr)
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
func deriveAppContainerSid(name string) (*syscall.SID, error) {
	loadUserenv()

	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("encode name: %w", err)
	}
	var sid *syscall.SID
	r1, _, callErr := procDeriveAppContainerSid.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&sid)),
	)
	if r1 == 0 {
		return nil, fmt.Errorf("DeriveAppContainerSid: %w", callErr)
	}
	return sid, nil
}

// featureDetection checks if a specific DLL procedure exists.
// Returns true if the procedure can be found.
func featureDetection(dll *syscall.LazyDLL, procName string) bool {
	proc := dll.NewProc(procName)
	return proc.Find() == nil
}
