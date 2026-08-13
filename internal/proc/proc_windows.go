//go:build windows

package proc

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

type windowsLookup struct{}

// NewDefaultLookup returns the Windows metadata resolver: process name via
// QueryFullProcessImageName, start time via GetProcessTimes, owner via the
// process token, and a best-effort PEB read for cmdline/cwd (undocumented
// internals — see readProcessParameters — so it fails closed to "" rather
// than risk surfacing garbage).
func NewDefaultLookup() Lookup { return windowsLookup{} }

func (windowsLookup) Lookup(pid int) (Info, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		return Info{}, fmt.Errorf("proc: OpenProcess: %w", err)
	}
	defer windows.CloseHandle(h)

	info := Info{}

	if name, err := queryImageName(h); err == nil {
		info.Process = name
	}

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err == nil {
		info.StartTime = filetimeToUint64(creation)
		info.Uptime = time.Since(time.Unix(0, creation.Nanoseconds()))
	}

	info.User = lookupUser(h)

	if cmdline, cwd, err := readProcessParameters(h); err == nil {
		info.Cmdline = cmdline
		info.CWD = cwd
	}

	return info, nil
}

func queryImageName(h windows.Handle) (string, error) {
	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", err
	}
	full := windows.UTF16ToString(buf[:size])
	base := filepath.Base(full)
	return strings.TrimSuffix(base, filepath.Ext(base)), nil
}

func filetimeToUint64(ft windows.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}

func lookupUser(h windows.Handle) string {
	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return ""
	}
	defer token.Close()

	tu, err := token.GetTokenUser()
	if err != nil {
		return ""
	}

	account, domain, _, err := lookupAccountSid(tu.User.Sid)
	if err != nil {
		return ""
	}
	if domain != "" {
		return domain + `\` + account
	}
	return account
}

func lookupAccountSid(sid *windows.SID) (account, domain string, sidType uint32, err error) {
	var accountLen, domainLen uint32
	_ = windows.LookupAccountSid(nil, sid, nil, &accountLen, nil, &domainLen, &sidType)
	if accountLen == 0 {
		return "", "", 0, fmt.Errorf("proc: LookupAccountSid sizing failed")
	}

	accountBuf := make([]uint16, accountLen)
	domainBuf := make([]uint16, domainLen)
	if err := windows.LookupAccountSid(nil, sid, &accountBuf[0], &accountLen, &domainBuf[0], &domainLen, &sidType); err != nil {
		return "", "", 0, err
	}
	return windows.UTF16ToString(accountBuf), windows.UTF16ToString(domainBuf), sidType, nil
}
