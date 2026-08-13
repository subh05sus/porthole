//go:build linux

package proc

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/subh05sus/porthole/internal/scan/procfmt"
)

// clockTicksPerSecond assumes the near-universal Linux USER_HZ value of 100
// (sysconf(_SC_CLK_TCK)). Reading the real value requires cgo or a raw
// auxv/sysconf syscall wrapper this project deliberately avoids; 100 is
// correct on essentially every x86/x86_64 distribution. Documented as a
// known limitation for exotic kernels/architectures in TODO.md.
const clockTicksPerSecond = 100

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
		info.Cmdline = strings.Join(procfmt.ParseCmdline(cmdData), " ")
	}

	if cwd, err := os.Readlink(filepath.Join(dir, "cwd")); err == nil {
		info.CWD = cwd
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
	age := sysUptime - float64(startTicks)/float64(clockTicksPerSecond)
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
