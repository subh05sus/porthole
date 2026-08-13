//go:build windows

package cli

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func platformDoctorChecks() []doctorCheck {
	elevated := windows.GetCurrentProcessToken().IsElevated()
	return []doctorCheck{
		{Name: "elevated (Administrator)", OK: true, Detail: fmt.Sprintf("%v", elevated)},
	}
}
