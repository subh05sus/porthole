//go:build windows

package proc

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// readProcessParameters reads a target process's command line and current
// working directory out of its PEB (Process Environment Block) via
// NtQueryInformationProcess + ReadProcessMemory.
//
// This walks undocumented, version-dependent Windows internals (the
// RTL_USER_PROCESS_PARAMETERS field offsets below are not part of any
// stable public API — they're long-standing but not contractual). It is
// deliberately defensive: every read is bounds- and sanity-checked, and any
// failure returns an error rather than a best-guess value, so a bad offset
// on some future Windows version degrades to an empty Cmdline/CWD (PRD
// §7.3's own documented fallback) instead of surfacing garbage. Verified
// empirically against a real spawned child process on this dev machine —
// both the native 64-bit-host-reading-64-bit-target path here, and the
// WOW64 path (32-bit target on this 64-bit host) in peb_wow64_windows.go.
func readProcessParameters(h windows.Handle) (cmdline, cwd string, err error) {
	// A 32-bit target process on this 64-bit host has an entirely separate
	// WOW64 PEB (a 32-bit structure, different offsets throughout) — the
	// native PEB read below would silently return wrong/empty data for it
	// rather than erroring, since the native PEB technically still exists
	// for a WOW64 process but isn't populated with real process parameters.
	if wow64, wowErr := isWow64Process(h); wowErr == nil && wow64 {
		return readProcessParametersWow64(h)
	}

	peb, err := getPEBAddress(h)
	if err != nil {
		return "", "", err
	}

	// PEB.ProcessParameters: pointer at offset 0x20 on 64-bit Windows.
	paramsAddr, err := readPointer(h, peb+0x20)
	if err != nil {
		return "", "", fmt.Errorf("proc: reading ProcessParameters pointer: %w", err)
	}
	if paramsAddr == 0 {
		return "", "", fmt.Errorf("proc: null ProcessParameters pointer")
	}

	// RTL_USER_PROCESS_PARAMETERS: CurrentDirectory.DosPath at 0x38,
	// CommandLine at 0x70 (both UNICODE_STRING).
	cwd, err = readUnicodeString(h, paramsAddr+0x38)
	if err != nil {
		return "", "", fmt.Errorf("proc: reading CurrentDirectory: %w", err)
	}
	cmdline, err = readUnicodeString(h, paramsAddr+0x70)
	if err != nil {
		return "", "", fmt.Errorf("proc: reading CommandLine: %w", err)
	}
	return cmdline, cwd, nil
}

var (
	modNtdll                      = windows.NewLazySystemDLL("ntdll.dll")
	procNtQueryInformationProcess = modNtdll.NewProc("NtQueryInformationProcess")
)

const processBasicInformationClass = 0

// processBasicInformation mirrors PROCESS_BASIC_INFORMATION. Reserved
// fields are collapsed to plain uintptr slots since only PebBaseAddress is
// used; this is the same simplified layout widely used across the Go
// ecosystem for this exact query.
type processBasicInformation struct {
	Reserved1       uintptr
	PebBaseAddress  uintptr
	Reserved2       [2]uintptr
	UniqueProcessID uintptr
	Reserved3       uintptr
}

func getPEBAddress(h windows.Handle) (uintptr, error) {
	var pbi processBasicInformation
	var retLen uint32
	r, _, _ := procNtQueryInformationProcess.Call(
		uintptr(h),
		processBasicInformationClass,
		uintptr(unsafe.Pointer(&pbi)),
		unsafe.Sizeof(pbi),
		uintptr(unsafe.Pointer(&retLen)),
	)
	if r != 0 {
		return 0, fmt.Errorf("proc: NtQueryInformationProcess failed: status 0x%x", uint32(r))
	}
	if pbi.PebBaseAddress == 0 {
		return 0, fmt.Errorf("proc: null PEB address")
	}
	return pbi.PebBaseAddress, nil
}

func readPointer(h windows.Handle, addr uintptr) (uintptr, error) {
	var buf [unsafe.Sizeof(uintptr(0))]byte
	var n uintptr
	if err := windows.ReadProcessMemory(h, addr, &buf[0], uintptr(len(buf)), &n); err != nil {
		return 0, err
	}
	if n != uintptr(len(buf)) {
		return 0, fmt.Errorf("proc: short read (%d of %d bytes)", n, len(buf))
	}
	return *(*uintptr)(unsafe.Pointer(&buf[0])), nil
}

// unicodeStringLayout mirrors the UNICODE_STRING struct layout on 64-bit
// Windows: 2 uint16 length fields, 4 bytes of padding to align Buffer, then
// an 8-byte pointer.
type unicodeStringLayout struct {
	Length        uint16
	MaximumLength uint16
	_             uint32
	Buffer        uintptr
}

// maxUnicodeStringBytes bounds how much we'll ever trust a UNICODE_STRING
// to claim, and how much we'll ever read from it — a sanity ceiling against
// misinterpreted offsets pointing at unrelated memory.
const maxUnicodeStringBytes = 32 * 1024

func readUnicodeString(h windows.Handle, addr uintptr) (string, error) {
	var raw [unsafe.Sizeof(unicodeStringLayout{})]byte
	var n uintptr
	if err := windows.ReadProcessMemory(h, addr, &raw[0], uintptr(len(raw)), &n); err != nil {
		return "", err
	}
	if n != uintptr(len(raw)) {
		return "", fmt.Errorf("proc: short read on UNICODE_STRING header (%d of %d bytes)", n, len(raw))
	}
	us := *(*unicodeStringLayout)(unsafe.Pointer(&raw[0]))

	if us.Length == 0 {
		return "", nil
	}
	if us.Length > us.MaximumLength || us.Length%2 != 0 || us.Length > maxUnicodeStringBytes || us.Buffer == 0 {
		return "", fmt.Errorf("proc: implausible UNICODE_STRING (length=%d max=%d buffer=0x%x)", us.Length, us.MaximumLength, us.Buffer)
	}

	strBuf := make([]byte, us.Length)
	if err := windows.ReadProcessMemory(h, us.Buffer, &strBuf[0], uintptr(len(strBuf)), &n); err != nil {
		return "", err
	}
	if n != uintptr(len(strBuf)) {
		return "", fmt.Errorf("proc: short read on UNICODE_STRING buffer (%d of %d bytes)", n, len(strBuf))
	}

	u16 := make([]uint16, len(strBuf)/2)
	for i := range u16 {
		u16[i] = uint16(strBuf[2*i]) | uint16(strBuf[2*i+1])<<8
	}
	return windows.UTF16ToString(u16), nil
}
