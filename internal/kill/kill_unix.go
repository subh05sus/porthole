//go:build linux || darwin

package kill

import (
	"errors"
	"syscall"
)

type unixSignaler struct{}

// NewDefaultKiller returns the Unix killer: SIGTERM/SIGKILL via
// syscall.Kill, driven through the portable Ladder from kill.go.
func NewDefaultKiller() Killer { return &Ladder{Signaler: unixSignaler{}} }

// StillAlive uses the standard "kill -0" probe — the same mechanism
// scan/permcheck_unix.go uses for Owned detection, and the same syscall the
// real signal will use — rather than a separate liveness heuristic.
func (unixSignaler) StillAlive(pid int) (startTime uint64, alive bool, err error) {
	if killErr := syscall.Kill(pid, 0); killErr != nil {
		if errors.Is(killErr, syscall.ESRCH) {
			return 0, false, nil
		}
		if errors.Is(killErr, syscall.EPERM) {
			// Signal-0 confirms it's alive even if some other permission
			// change now blocks us; don't fail the poll over that.
			return 0, true, nil
		}
		return 0, false, killErr
	}

	st, err := currentStartTime(pid)
	if err != nil {
		// Alive, but a transient read error shouldn't fail the whole
		// check — the caller only uses start time for the one comparison
		// immediately before signaling, not on every poll tick.
		return 0, true, nil
	}
	return st, true, nil
}

func (unixSignaler) Terminate(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

func (unixSignaler) Kill(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}
