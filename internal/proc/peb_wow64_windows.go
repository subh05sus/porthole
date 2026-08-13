//go:build windows

package proc

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// isWow64Process reports whether the target process is a 32-bit process
// running under WOW64 on this 64-bit host.
func isWow64Process(h windows.Handle) (bool, error) {
	var wow64 uint32 // BOOL: nonzero means true
	r, _, err := procIsWow64Process.Call(uintptr(h), uintptr(unsafe.Pointer(&wow64)))
	if r == 0 {
		return false, err
	}
	return wow64 != 0, nil
}

var (
	modKernel32        = windows.NewLazySystemDLL("kernel32.dll")
	procIsWow64Process = modKernel32.NewProc("IsWow64Process")
)

// processWow64InformationClass is PROCESSINFOCLASS value 26
// (ProcessWow64Information): for a WOW64 process, NtQueryInformationProcess
// returns the address of its 32-bit PEB here — the native PEB address from
// ProcessBasicInformation exists for a WOW64 process too, but isn't
// populated with the real command line/cwd.
const processWow64InformationClass = 26

func getWow64PEBAddress(h windows.Handle) (uintptr, error) {
	var peb32Addr uintptr
	var retLen uint32
	r, _, _ := procNtQueryInformationProcess.Call(
		uintptr(h),
		processWow64InformationClass,
		uintptr(unsafe.Pointer(&peb32Addr)),
		unsafe.Sizeof(peb32Addr),
		uintptr(unsafe.Pointer(&retLen)),
	)
	if r != 0 {
		return 0, fmt.Errorf("proc: NtQueryInformationProcess(Wow64Information) failed: status 0x%x", uint32(r))
	}
	if peb32Addr == 0 {
		return 0, fmt.Errorf("proc: null WOW64 PEB address")
	}
	return peb32Addr, nil
}

// readProcessParametersWow64 mirrors readProcessParameters (peb_windows.go)
// for a 32-bit target process. Everything is 4-byte pointers and a
// different RTL_USER_PROCESS_PARAMETERS layout (PEB32.ProcessParameters at
// a different offset than the 64-bit PEB's), so it can't share the native
// path's offsets. Same defensive discipline: bounds/sanity-checked, fails
// closed to an error (never a best-guess value) on anything implausible.
// Empirically verified on this dev machine against a real 32-bit
// (GOARCH=386) child process — see Learnings.md for what changed from the
// initial offset guesses.
func readProcessParametersWow64(h windows.Handle) (cmdline, cwd string, err error) {
	peb32, err := getWow64PEBAddress(h)
	if err != nil {
		return "", "", err
	}

	// PEB32.ProcessParameters: 4-byte pointer at offset 0x10.
	paramsAddr, err := readPointer32(h, peb32+0x10)
	if err != nil {
		return "", "", fmt.Errorf("proc: reading WOW64 ProcessParameters pointer: %w", err)
	}
	if paramsAddr == 0 {
		return "", "", fmt.Errorf("proc: null WOW64 ProcessParameters pointer")
	}

	// RTL_USER_PROCESS_PARAMETERS32: CurrentDirectory.DosPath at 0x24,
	// CommandLine at 0x40 (both UNICODE_STRING32 — 8 bytes, not 16).
	cwd, err = readUnicodeString32(h, paramsAddr+0x24)
	if err != nil {
		return "", "", fmt.Errorf("proc: reading WOW64 CurrentDirectory: %w", err)
	}
	cmdline, err = readUnicodeString32(h, paramsAddr+0x40)
	if err != nil {
		return "", "", fmt.Errorf("proc: reading WOW64 CommandLine: %w", err)
	}
	return cmdline, cwd, nil
}

func readPointer32(h windows.Handle, addr uintptr) (uintptr, error) {
	var buf [4]byte
	var n uintptr
	if err := windows.ReadProcessMemory(h, addr, &buf[0], 4, &n); err != nil {
		return 0, err
	}
	if n != 4 {
		return 0, fmt.Errorf("proc: short read (%d of 4 bytes)", n)
	}
	return uintptr(binary.LittleEndian.Uint32(buf[:])), nil
}

// unicodeString32Layout mirrors UNICODE_STRING as laid out in a 32-bit
// process: 2 uint16 length fields plus a 4-byte pointer, 8 bytes total, no
// padding needed (already naturally aligned).
type unicodeString32Layout struct {
	Length        uint16
	MaximumLength uint16
	Buffer        uint32
}

func readUnicodeString32(h windows.Handle, addr uintptr) (string, error) {
	var raw [unsafe.Sizeof(unicodeString32Layout{})]byte
	var n uintptr
	if err := windows.ReadProcessMemory(h, addr, &raw[0], uintptr(len(raw)), &n); err != nil {
		return "", err
	}
	if n != uintptr(len(raw)) {
		return "", fmt.Errorf("proc: short read on UNICODE_STRING32 header (%d of %d bytes)", n, len(raw))
	}
	us := *(*unicodeString32Layout)(unsafe.Pointer(&raw[0]))

	if us.Length == 0 {
		return "", nil
	}
	if us.Length > us.MaximumLength || us.Length%2 != 0 || us.Length > maxUnicodeStringBytes || us.Buffer == 0 {
		return "", fmt.Errorf("proc: implausible UNICODE_STRING32 (length=%d max=%d buffer=0x%x)", us.Length, us.MaximumLength, us.Buffer)
	}

	strBuf := make([]byte, us.Length)
	if err := windows.ReadProcessMemory(h, uintptr(us.Buffer), &strBuf[0], uintptr(len(strBuf)), &n); err != nil {
		return "", err
	}
	if n != uintptr(len(strBuf)) {
		return "", fmt.Errorf("proc: short read on UNICODE_STRING32 buffer (%d of %d bytes)", n, len(strBuf))
	}

	u16 := make([]uint16, len(strBuf)/2)
	for i := range u16 {
		u16[i] = uint16(strBuf[2*i]) | uint16(strBuf[2*i+1])<<8
	}
	return windows.UTF16ToString(u16), nil
}
