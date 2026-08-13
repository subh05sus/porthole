//go:build linux

package proc

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/subh05sus/porthole/internal/scan/procfmt"
)

// clockTicksPerSecond reads the kernel's real USER_HZ value (sysconf's
// _SC_CLK_TCK) via `getconf CLK_TCK` — the same shellout pattern already
// used on darwin for start-time lookups — rather than hardcoding the
// near-universal x86/x86_64 value of 100, which this project previously
// assumed to avoid a cgo dependency. Falls back to 100 if the shellout
// fails for any reason (getconf missing, unexpected output), so a
// transient failure degrades to the old behavior instead of breaking
// uptime math. Resolved once per process via sync.OnceValue since the
// value cannot change while porthole is running.
var clockTicksPerSecond = sync.OnceValue(func() int {
	out, err := exec.Command("getconf", "CLK_TCK").Output()
	if err != nil {
		return 100
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || n <= 0 {
		return 100
	}
	return n
})

type linuxLookup struct{}

// NewDefaultLookup returns the Linux metadata resolver, reading everything
// straight from /proc/[pid]/{stat,cmdline,cwd,status} per PRD §7.2.
func NewDefaultLookup() Lookup { return linuxLookup{} }

func (linuxLookup) Lookup(pid int) (Info, error) {
	dir := filepath.Join("/proc", strconv.Itoa(pid))

	statData, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return Info{}, fmt.Errorf("proc: reading stat: %w", err)
	}
	stat, err := procfmt.ParseStat(statData)
	if err != nil {
		return Info{}, err
	}

	info := Info{
		Process:   stat.Comm,
		StartTime: stat.StartTimeTicks,
	}

	if cmdData, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
		info.Argv = procfmt.ParseCmdline(cmdData)
		info.Cmdline = strings.Join(info.Argv, " ")
	}

	if cwd, err := os.Readlink(filepath.Join(dir, "cwd")); err == nil {
		info.CWD = cwd
	}

	if exe, err := os.Readlink(filepath.Join(dir, "exe")); err == nil {
		info.ExePath = exe
	}

	if envData, err := os.ReadFile(filepath.Join(dir, "environ")); err == nil {
		info.Env = procfmt.ParseCmdline(envData) // same NUL-separated format
	} else {
		info.EnvErr = fmt.Errorf("proc: reading environ: %w", err)
	}

	if uid, err := readUID(dir); err == nil {
		if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
			info.User = u.Username
		} else {
			info.User = strconv.Itoa(uid)
		}
	}

	if uptime, err := processUptime(stat.StartTimeTicks); err == nil {
		info.Uptime = uptime
	}

	return info, nil
}

func readUID(procDir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(procDir, "status"))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "Uid:" {
			return strconv.Atoi(fields[1])
		}
	}
	return 0, fmt.Errorf("proc: Uid not found in status")
}

// processUptime combines a process's starttime (clock ticks since boot)
// with the kernel's clock tick rate and current system uptime to produce a
// wall-clock-ish duration, independent of scan.Service.StartTime's opaque
// reuse-detection value.
func processUptime(startTicks uint64) (time.Duration, error) {
	sysUptime, err := readSystemUptime()
	if err != nil {
		return 0, err
	}
	age := sysUptime - float64(startTicks)/float64(clockTicksPerSecond())
	if age < 0 {
		age = 0
	}
	return time.Duration(age * float64(time.Second)), nil
}

func readSystemUptime() (float64, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 1 {
		return 0, fmt.Errorf("proc: unexpected /proc/uptime format")
	}
	return strconv.ParseFloat(fields[0], 64)
}
