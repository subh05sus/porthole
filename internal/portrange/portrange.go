// Package portrange parses a single port-or-range token, shared between
// internal/cli's --dev/kill-args handling and internal/config's validation
// of kill.dev_port_range — a leaf package so internal/config (which cli
// already imports) never needs to import internal/cli.
package portrange

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse expands one token — a single port ("3000") or an inclusive range
// ("3000-3010") — into the ports it represents. Deduplication and
// multi-token/comma handling are the caller's job; this only ever sees one
// token at a time.
func Parse(token string) ([]int, error) {
	if lo, hi, ok := strings.Cut(token, "-"); ok {
		loN, errLo := strconv.Atoi(strings.TrimSpace(lo))
		hiN, errHi := strconv.Atoi(strings.TrimSpace(hi))
		if errLo == nil && errHi == nil {
			if loN > hiN {
				return nil, fmt.Errorf("invalid port range %q: start greater than end", token)
			}
			ports := make([]int, 0, hiN-loN+1)
			for p := loN; p <= hiN; p++ {
				ports = append(ports, p)
			}
			return ports, nil
		}
	}
	p, err := strconv.Atoi(strings.TrimSpace(token))
	if err != nil {
		return nil, fmt.Errorf("invalid port %q", token)
	}
	return []int{p}, nil
}
