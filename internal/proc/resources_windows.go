//go:build windows

package proc

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var _ ResourceQuerier = windowsLookup{}

var (
	modPsapi                 = windows.NewLazySystemDLL("psapi.dll")
	procGetProcessMemoryInfo = modPsapi.NewProc("GetProcessMemoryInfo")
)

// processMemoryCounters mirrors PROCESS_MEMORY_COUNTERS.
type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// cpuSampleWindow mirrors resources_linux.go's — see its comment.
const cpuSampleWindow = 150 * time.Millisecond

// Resources implements ResourceQuerier: CPU% from two GetProcessTimes
// samples cpuSampleWindow apart (Windows has no single-call "current CPU%"
// API, unlike ps), RSS from GetProcessMemoryInfo's WorkingSetSize — the
// closest Windows equivalent to RSS (physical memory currently mapped in,
// as opposed to PagefileUsage's committed-but-possibly-paged-out view).
func (windowsLookup) Resources(pid int) (ResourceStats, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ResourceStats{}, fmt.Errorf("proc: OpenProcess: %w", err)
	}
	defer windows.CloseHandle(h)

	before, err := processCPUTime(h)
	if err != nil {
		return ResourceStats{}, err
	}
	start := time.Now()
	time.Sleep(cpuSampleWindow)
	after, err := processCPUTime(h)
	if err != nil {
		return ResourceStats{}, err
	}
	elapsed := time.Since(start)

	cpuPercent := 0.0
	if elapsed > 0 {
		cpuPercent = (after - before).Seconds() / elapsed.Seconds() * 100
	}

	rss, err := workingSetSize(h)
	if err != nil {
		return ResourceStats{}, err
	}

	return ResourceStats{CPUPercent: cpuPercent, RSSBytes: rss}, nil
}

func processCPUTime(h windows.Handle) (time.Duration, error) {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return 0, fmt.Errorf("proc: GetProcessTimes: %w", err)
	}
	return filetimeToDuration(kernel) + filetimeToDuration(user), nil
}

// filetimeToDuration converts a FILETIME (100-nanosecond intervals) used
// as a duration (kernel/user CPU time) rather than a point in time — do
// not confuse with filetimeToUint64 in proc_windows.go, which preserves
// the raw value for PID-reuse comparison instead.
func filetimeToDuration(ft windows.Filetime) time.Duration {
	ticks := uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
	return time.Duration(ticks) * 100 * time.Nanosecond
}

func workingSetSize(h windows.Handle) (uint64, error) {
	var counters processMemoryCounters
	counters.Cb = uint32(unsafe.Sizeof(counters))
	r, _, _ := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&counters)), uintptr(counters.Cb))
	if r == 0 {
		return 0, fmt.Errorf("proc: GetProcessMemoryInfo failed")
	}
	return uint64(counters.WorkingSetSize), nil
}
