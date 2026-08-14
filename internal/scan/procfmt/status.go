package procfmt

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseVmRSS extracts the "VmRSS:" line from the contents of
// /proc/[pid]/status and returns it in bytes (the file reports kB).
func ParseVmRSS(data []byte) (uint64, error) {
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("procfmt: malformed VmRSS line %q", line)
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("procfmt: bad VmRSS value %q: %w", fields[1], err)
		}
		return kb * 1024, nil
	}
	return 0, fmt.Errorf("procfmt: VmRSS not found")
}
