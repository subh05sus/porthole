package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/subh05sus/porthole/internal/config"
)

type doctorCheck struct {
	Name   string
	OK     bool
	Detail string
}

func newDoctorCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose common permission and environment issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(app)
		},
	}
	return cmd
}

func runDoctor(app *App) error {
	checks := append(commonDoctorChecks(), platformDoctorChecks()...)

	allOK := true
	for _, c := range checks {
		status := "OK"
		if !c.OK {
			status = "FAIL"
			allOK = false
		}
		line := fmt.Sprintf("[%s] %s", status, c.Name)
		if c.Detail != "" {
			line += " — " + c.Detail
		}
		fmt.Fprintln(app.Stdout, line)
	}

	if !allOK {
		return exitErr(ExitPermissionDenied, fmt.Errorf("doctor found one or more issues"))
	}
	return nil
}

// commonDoctorChecks are informational (never fail doctor's exit code by
// themselves) — they help explain *why* rows might show locked/unresolved,
// not report a broken installation.
func commonDoctorChecks() []doctorCheck {
	_, noColorSet := os.LookupEnv("NO_COLOR")

	return []doctorCheck{
		{Name: "NO_COLOR", OK: true, Detail: fmt.Sprintf("set=%v", noColorSet)},
		{Name: "config file", OK: true, Detail: configStatus()},
	}
}

func configStatus() string {
	path, err := config.DefaultPath()
	if err != nil {
		return "cannot resolve home directory: " + err.Error()
	}
	if _, err := os.Stat(path); err != nil {
		return path + " not present (optional)"
	}
	return path + " present"
}
