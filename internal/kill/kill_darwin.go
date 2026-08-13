//go:build darwin

package kill

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/subh05sus/porthole/internal/scan/lsoffmt"
)

// currentStartTime reads a process's current start time via `ps -o lstart=`
// — lighter weight than a full proc.Lookup (which also shells out to lsof
// for cwd), since this runs on every poll tick during a kill's liveness
// wait. Carries the same second-granularity residual risk documented on
// lsoffmt.ParseLstart.
func currentStartTime(pid int) (uint64, error) {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "lstart=")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("kill: ps -o lstart=: %w", err)
	}
	t, err := lsoffmt.ParseLstart(strings.TrimSpace(out.String()))
	if err != nil {
		return 0, err
	}
	return uint64(t.Unix()), nil
}
