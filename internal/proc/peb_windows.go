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

	paramsAddr, err := getProcessParametersAddr(h)
	if err != nil {
		return "", "", err
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

// getProcessParametersAddr resolves PEB.ProcessParameters (a pointer at
// offset 0x20 on 64-bit Windows) — shared by readProcessParameters and
// readProcessEnvironment since both start from the same struct.
func getProcessParametersAddr(h windows.Handle) (uintptr, error) {
	peb, err := getPEBAddress(h)
	if err != nil {
		return 0, err
	}
	paramsAddr, err := readPointer(h, peb+0x20)
	if err != nil {
		return 0, fmt.Errorf("proc: reading ProcessParameters pointer: %w", err)
	}
	if paramsAddr == 0 {
		return 0, fmt.Errorf("proc: null ProcessParameters pointer")
	}
	return paramsAddr, nil
}

// readProcessEnvironment reads a target process's environment block:
// RTL_USER_PROCESS_PARAMETERS.Environment (a raw pointer at offset 0x80,
// immediately after the 16-byte CommandLine UNICODE_STRING at 0x70 — the
// same "long-standing but not contractual" caveat as readProcessParameters
// applies) points to a sequence of NUL-terminated "KEY=VALUE" UTF-16
// strings ending in an extra NUL (i.e. a double-NUL overall), with no
// length prefix. Bounded by maxEnvironmentBlockBytes as a sanity ceiling
// against a wrong offset walking off into unrelated memory.
func readProcessEnvironment(h windows.Handle) ([]string, error) {
	paramsAddr, err := getProcessParametersAddr(h)
	if err != nil {
		return nil, err
	}

	envAddr, err := readPointer(h, paramsAddr+0x80)
	if err != nil {
		return nil, fmt.Errorf("proc: reading Environment pointer: %w", err)
	}
	if envAddr == 0 {
		return nil, fmt.Errorf("proc: null Environment pointer")
	}

	return readEnvironmentBlock(h, envAddr)
}

const (
	envReadChunk             = 4096
	maxEnvironmentBlockBytes = 1 << 20 // 1 MiB sanity ceiling
)

func readEnvironmentBlock(h windows.Handle, addr uintptr) ([]string, error) {
	var raw []byte
	for len(raw) < maxEnvironmentBlockBytes {
		buf := make([]byte, envReadChunk)
		var n uintptr
		err := windows.ReadProcessMemory(h, addr+uintptr(len(raw)), &buf[0], envReadChunk, &n)
		if err != nil {
			if len(raw) == 0 {
				return nil, fmt.Errorf("proc: reading environment block: %w", err)
			}
			break
		}
		raw = append(raw, buf[:n]...)

		if len(raw)%2 == 0 {
			u16 := bytesToUTF16(raw)
			if idx := findDoubleNulUTF16(u16); idx >= 0 {
				return splitEnvBlock(u16[:idx]), nil
			}
		}
		if n < envReadChunk {
			break
		}
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("proc: empty environment block")
	}
	return splitEnvBlock(bytesToUTF16(raw)), nil
}

func bytesToUTF16(b []byte) []uint16 {
	u16 := make([]uint16, len(b)/2)
	for i := range u16 {
		u16[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return u16
}

func findDoubleNulUTF16(u16 []uint16) int {
	for i := 0; i+1 < len(u16); i++ {
		if u16[i] == 0 && u16[i+1] == 0 {
			return i
		}
	}
	return -1
}

// splitEnvBlock splits a NUL-separated (but not NUL-terminated per
// segment beyond the separator itself) UTF-16 environment block into
// "KEY=VALUE" strings.
func splitEnvBlock(u16 []uint16) []string {
	var out []string
	start := 0
	for i, c := range u16 {
		if c == 0 {
			if i > start {
				out = append(out, windows.UTF16ToString(u16[start:i]))
			}
			start = i + 1
		}
	}
	if start < len(u16) {
		out = append(out, windows.UTF16ToString(u16[start:]))
	}
	return out
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
