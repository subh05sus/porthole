//go:build windows

package kill

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

const stillActive = 259 // STILL_ACTIVE, per GetExitCodeProcess docs

type windowsSignaler struct{}

// NewDefaultKiller returns the Windows killer. There is no distinct
// graceful signal on Windows, so Terminate and Kill both call
// TerminateProcess — documented as a real behavioral difference from Unix
// in PRD §7.4, not an oversight.
func NewDefaultKiller() Killer { return &Ladder{Signaler: windowsSignaler{}} }

func (windowsSignaler) StillAlive(pid int) (startTime uint64, alive bool, err error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return 0, false, nil // no such process
		}
		return 0, false, err
	}
	defer windows.CloseHandle(h)

	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return 0, false, err
	}
	if exitCode != stillActive {
		return 0, false, nil
	}

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		// Alive (exit code confirms it), just couldn't read start time —
		// don't fail the whole liveness check over that.
		return 0, true, nil
	}
	return uint64(creation.HighDateTime)<<32 | uint64(creation.LowDateTime), true, nil
}

func (windowsSignaler) Terminate(pid int) error { return terminateProcess(pid) }
func (windowsSignaler) Kill(pid int) error      { return terminateProcess(pid) }

func terminateProcess(pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("kill: OpenProcess: %w", err)
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, 1)
}
