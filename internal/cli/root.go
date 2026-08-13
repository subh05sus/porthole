// Package cli implements the porthole command tree. Commands take their
// dependencies via App so they can be tested against fakes without a real
// OS or terminal.
package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/subh05sus/porthole/internal/config"
	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/proc"
	"github.com/subh05sus/porthole/internal/restart"
	"github.com/subh05sus/porthole/internal/scan"
)

// Version is overridden at build time via -ldflags.
var Version = "dev"

// Exit codes, per PRD §6.
const (
	ExitSuccess          = 0
	ExitNotFound         = 1
	ExitPermissionDenied = 2
	ExitKillFailed       = 3
	ExitPIDChanged       = 4
)

// ExitError carries the specific process exit code a failure should
// produce, distinct from cobra's default of always exiting 1 on error.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

func exitErr(code int, err error) *ExitError { return &ExitError{Code: code, Err: err} }

// App holds every dependency a subcommand needs.
type App struct {
	Lister      scan.Lister
	Killer      kill.Killer
	Lookup      proc.Lookup     // used by restart to capture cmdline/cwd/env
	Spawner     restart.Spawner // used by restart to respawn
	Config      config.Config   // protected ports, theme/animation preferences
	HistoryPath string          // for `porthole history`; empty disables it
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
}

// NewRootCmd builds the porthole command tree against app's dependencies.
func NewRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:           "porthole",
		Short:         "A port kill switch and service viewer for your terminal.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(app.Stdout)
	root.SetErr(app.Stderr)
	root.SetIn(app.Stdin)

	root.AddCommand(newListCmd(app))
	root.AddCommand(newKillCmd(app))
	root.AddCommand(newWatchCmd(app))
	root.AddCommand(newRestartCmd(app))
	root.AddCommand(newDoctorCmd(app))
	root.AddCommand(newHistoryCmd(app))

	root.RunE = func(cmd *cobra.Command, args []string) error {
		return runTUI(app)
	}

	return root
}

// Execute runs the root command with args (excluding the program name
// itself) and returns the process exit code to use.
func Execute(app *App, args []string) int {
	root := NewRootCmd(app)
	root.SetArgs(args)

	err := root.Execute()
	if err == nil {
		return ExitSuccess
	}

	var ee *ExitError
	if errors.As(err, &ee) {
		fmt.Fprintln(app.Stderr, ee.Error())
		return ee.Code
	}
	fmt.Fprintln(app.Stderr, err.Error())
	return 1
}
