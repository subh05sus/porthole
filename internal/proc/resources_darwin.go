//go:build darwin

package proc

import (
	"fmt"
	"strconv"
	"strings"
)

var _ ResourceQuerier = darwinLookup{}

// Resources implements ResourceQuerier via ps, which already computes a
// live CPU% itself (unlike Windows, there's no need for our own two-sample
// delta here) — reuses the same runPS helper Lookup uses.
func (darwinLookup) Resources(pid int) (ResourceStats, error) {
	cpuOut, err := runPS(pid, "%cpu=")
	if err != nil {
		return ResourceStats{}, err
	}
	cpuPercent, err := strconv.ParseFloat(strings.TrimSpace(cpuOut), 64)
	if err != nil {
		return ResourceStats{}, fmt.Errorf("proc: parsing %%cpu output %q: %w", cpuOut, err)
	}

	rssOut, err := runPS(pid, "rss=")
	if err != nil {
		return ResourceStats{}, err
	}
	rssKB, err := strconv.ParseUint(strings.TrimSpace(rssOut), 10, 64)
	if err != nil {
		return ResourceStats{}, fmt.Errorf("proc: parsing rss output %q: %w", rssOut, err)
	}

	return ResourceStats{CPUPercent: cpuPercent, RSSBytes: rssKB * 1024}, nil
}
