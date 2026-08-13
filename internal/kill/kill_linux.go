//go:build linux

package kill

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/subh05sus/porthole/internal/scan/procfmt"
)

// currentStartTime reads a process's current start time (clock ticks since
// boot) directly from /proc/[pid]/stat — lighter weight than a full
// proc.Lookup, which matters here since this is called on every poll tick
// during a kill's liveness wait.
func currentStartTime(pid int) (uint64, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	stat, err := procfmt.ParseStat(data)
	if err != nil {
		return 0, err
	}
	return stat.StartTimeTicks, nil
}
