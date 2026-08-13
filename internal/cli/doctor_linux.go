//go:build linux

package cli

import "os"

func platformDoctorChecks() []doctorCheck {
	checks := []doctorCheck{rootCheck()}

	if _, err := os.ReadDir("/proc"); err != nil {
		checks = append(checks, doctorCheck{Name: "/proc readable", OK: false, Detail: err.Error()})
	} else {
		checks = append(checks, doctorCheck{Name: "/proc readable", OK: true})
	}
	return checks
}
