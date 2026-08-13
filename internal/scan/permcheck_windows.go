//go:build windows

package scan

import (
	"errors"

	"golang.org/x/sys/windows"
)

// checkOwned reports whether the current user has permission to terminate
// pid. This deliberately probes PROCESS_TERMINATE specifically (not just
// the PROCESS_QUERY_LIMITED_INFORMATION used elsewhere for metadata reads),
// since Windows can grant query rights without terminate rights — e.g. for
// a SYSTEM service — and this is the actual right the kill path will need.
// This is the Windows-side equivalent of the Unix "kill -0" probe in
// permcheck_unix.go, giving both platforms the same Owned/ResolveErr
// contract (PRD §8.2) instead of Windows surfacing a raw access-denied
// error only at kill time.
func checkOwned(pid int) (owned bool, resolveErr error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return false, errors.New("scan: needs elevated permissions")
		}
		return false, err
	}
	windows.CloseHandle(h)
	return true, nil
}
