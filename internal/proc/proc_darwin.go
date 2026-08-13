//go:build darwin

package proc

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/subh05sus/porthole/internal/scan/lsoffmt"
)

type darwinLookup struct{}

// NewDefaultLookup returns the macOS metadata resolver: ps for
// lstart/user/command, plus a second lsof shellout for cwd rather than a
// cgo/libproc dependency (see FUTURE_PLANS.md tech debt: libproc would be
// faster but is a meaningful rewrite, deferred past v1).
func NewDefaultLookup() Lookup { return darwinLookup{} }

func (darwinLookup) Lookup(pid int) (Info, error) {
	lstartOut, err := runPS(pid, "lstart=")
	if err != nil {
		return Info{}, err
	}
	startedAt, err := lsoffmt.ParseLstart(strings.TrimSpace(lstartOut))
	if err != nil {
		return Info{}, err
	}

	userOut, err := runPS(pid, "user=")
	if err != nil {
		return Info{}, err
	}
	commOut, err := runPS(pid, "comm=")
	if err != nil {
		return Info{}, err
	}
	// Best-effort: a missing full command line shouldn't fail the whole
	// lookup, since the process name and start time are already known.
	argsOut, _ := runPS(pid, "args=")

	return Info{
		Process:   strings.TrimSpace(commOut),
		Cmdline:   strings.TrimSpace(argsOut),
		User:      strings.TrimSpace(userOut),
		CWD:       lookupCWD(pid),
		StartTime: uint64(startedAt.Unix()),
		Uptime:    time.Since(startedAt),
	}, nil
}

func runPS(pid int, format string) (string, error) {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", format)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("proc: ps -o %s -p %d: %w", format, pid, err)
	}
	return out.String(), nil
}

func lookupCWD(pid int) string {
	cmd := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-F", "n")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return ""
	}
	for _, line := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(line, "n") {
			return line[1:]
		}
	}
	return ""
}
