//go:build linux || darwin

package cli

import (
	"fmt"
	"os"
)

// rootCheck is shared by linux and darwin's platformDoctorChecks — purely
// informational (a non-root user is completely normal), it explains why
// some rows might show locked rather than reporting a real problem.
func rootCheck() doctorCheck {
	uid := os.Geteuid()
	return doctorCheck{Name: "running as root", OK: true, Detail: fmt.Sprintf("uid=%d root=%v", uid, uid == 0)}
}
