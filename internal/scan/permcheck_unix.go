//go:build linux || darwin

package scan

import (
	"errors"
	"syscall"
)

// checkOwned reports whether the current user has permission to signal pid,
// via the standard "kill -0" probe: sending signal 0 does no actual
// signaling but still runs the OS's permission check, so this is the same
// mechanism used later to actually kill the process — not a guess.
func checkOwned(pid int) (owned bool, resolveErr error) {
	err := syscall.Kill(pid, 0)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.EPERM):
		return false, errors.New("scan: needs elevated permissions")
	case errors.Is(err, syscall.ESRCH):
		return false, errors.New("scan: process no longer exists")
	default:
		return false, err
	}
}
