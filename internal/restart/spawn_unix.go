//go:build linux || darwin

package restart

import (
	"fmt"
	"os/exec"
	"syscall"
)

// spawn respawns plan using its Unix argv[], detached into its own session
// (Setsid) so it isn't tied to porthole's controlling terminal and survives
// porthole exiting.
func spawn(plan Plan) error {
	if len(plan.Argv) == 0 {
		return fmt.Errorf("restart: no argv available to respawn with")
	}
	cmd := exec.Command(plan.Argv[0], plan.Argv[1:]...)
	cmd.Dir = plan.CWD
	cmd.Env = plan.Env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
