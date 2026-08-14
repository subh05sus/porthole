//go:build linux

package proc

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/subh05sus/porthole/internal/scan/procfmt"
)

var _ ResourceQuerier = linuxLookup{}

// cpuSampleWindow is how long Resources waits between its two /proc/[pid]/
// stat reads to compute a CPU% delta — short enough to feel instant when
// the detail pane opens, long enough for the tick-granularity CPU-time
// counter to move measurably.
const cpuSampleWindow = 150 * time.Millisecond

// Resources implements ResourceQuerier by sampling /proc/[pid]/stat's
// utime+stime twice, cpuSampleWindow apart, the same way `top` computes a
// live CPU% — and reading /proc/[pid]/status's VmRSS for memory.
func (linuxLookup) Resources(pid int) (ResourceStats, error) {
	before, err := readCPUTicks(pid)
	if err != nil {
		return ResourceStats{}, err
	}
	start := time.Now()
	time.Sleep(cpuSampleWindow)
	after, err := readCPUTicks(pid)
	if err != nil {
		return ResourceStats{}, err
	}
	elapsed := time.Since(start)

	deltaSeconds := float64(after-before) / float64(clockTicksPerSecond())
	cpuPercent := 0.0
	if elapsed > 0 {
		cpuPercent = deltaSeconds / elapsed.Seconds() * 100
	}

	rss, err := readVmRSS(pid)
	if err != nil {
		return ResourceStats{}, err
	}

	return ResourceStats{CPUPercent: cpuPercent, RSSBytes: rss}, nil
}

func readCPUTicks(pid int) (uint64, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	stat, err := procfmt.ParseStat(data)
	if err != nil {
		return 0, err
	}
	return stat.UTimeTicks + stat.STimeTicks, nil
}

func readVmRSS(pid int) (uint64, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return 0, err
	}
	return procfmt.ParseVmRSS(data)
}
