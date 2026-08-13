//go:build darwin

package cli

import "os/exec"

func platformDoctorChecks() []doctorCheck {
	checks := []doctorCheck{rootCheck()}

	if _, err := exec.LookPath("lsof"); err != nil {
		checks = append(checks, doctorCheck{Name: "lsof present", OK: false, Detail: "not found in PATH — the scanner needs it"})
	} else {
		checks = append(checks, doctorCheck{Name: "lsof present", OK: true})
	}
	return checks
}
