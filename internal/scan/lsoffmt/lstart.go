package lsoffmt

import (
	"fmt"
	"strings"
	"time"
)

// ParseLstart parses macOS's `ps -o lstart=` output for a single process,
// e.g. "Wed Aug 13 20:15:00 2026" (a single-digit day pads with an extra
// space, which strings.Fields absorbs before reformatting).
//
// Known residual risk (not fully closeable on macOS): lstart is only
// second-granularity. In a tight dev loop — e.g. nodemon respawning on the
// same port inside one second, a very common workflow for exactly the kind
// of tool this is — two different processes can land on the same lstart
// second, which defeats the PID-reuse guard in internal/kill that compares
// this value against a fresh read taken immediately before signaling. There
// is no higher-resolution wall-clock start time available without root-only
// APIs, so this is the best available guard on macOS, not an airtight one.
func ParseLstart(s string) (time.Time, error) {
	fields := strings.Fields(s)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("lsoffmt: unexpected lstart format %q", s)
	}
	normalized := strings.Join(fields, " ")

	t, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", normalized, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("lsoffmt: parsing lstart %q: %w", s, err)
	}
	return t, nil
}
