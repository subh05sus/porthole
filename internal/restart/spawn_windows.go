//go:build windows

package restart

import (
	"fmt"
	"os/exec"
	"syscall"
)

// createNewProcessGroup detaches the respawned process from porthole's own
// console/process group, so it survives porthole exiting and doesn't
// receive Ctrl+C events aimed at porthole's console.
const createNewProcessGroup = 0x00000200

// spawn respawns plan on Windows using the exact original command line
// verbatim (via SysProcAttr.CmdLine) rather than reconstructing one from
// split arguments, since Windows has no native argv[] and re-quoting could
// subtly differ from what the process originally received.
func spawn(plan Plan) error {
	if plan.ExePath == "" || plan.Cmdline == "" {
		return fmt.Errorf("restart: no executable path/command line available to respawn with")
	}
	cmd := &exec.Cmd{
		Path: plan.ExePath,
		Dir:  plan.CWD,
		Env:  plan.Env,
		SysProcAttr: &syscall.SysProcAttr{
			CmdLine:       plan.Cmdline,
			CreationFlags: createNewProcessGroup,
		},
	}
	return cmd.Start()
}
