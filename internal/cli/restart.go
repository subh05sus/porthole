package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/restart"
	"github.com/subh05sus/porthole/internal/scan"
)

func newRestartCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restart <port>",
		Short: "Kill the process listening on a port, then respawn it with the same command",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRestart(app, args[0])
		},
	}
	return cmd
}

func runRestart(app *App, portArg string) error {
	ports, err := parsePorts([]string{portArg})
	if err != nil || len(ports) != 1 {
		return exitErr(ExitNotFound, fmt.Errorf("invalid port %q", portArg))
	}
	port := ports[0]

	ctx := context.Background()
	services, err := app.Lister.List(ctx)
	if err != nil {
		return err
	}

	var target *scan.Service
	for i := range services {
		if services[i].Port == port {
			target = &services[i]
			break
		}
	}
	if target == nil {
		return exitErr(ExitNotFound, fmt.Errorf("nothing found on port %d", port))
	}
	if !target.Owned {
		return exitErr(ExitPermissionDenied, fmt.Errorf("port :%d: needs elevated permissions, try: sudo porthole", port))
	}

	// Capture the respawn plan *before* killing — once the process is
	// dead, its cmdline/cwd/env are gone.
	plan, err := restart.Capture(app.Lookup, *target)
	if err != nil {
		return exitErr(ExitKillFailed, err)
	}

	killTarget := kill.Target{PID: target.PID, StartTime: target.StartTime, Owned: target.Owned}
	res, err := app.Killer.Execute(ctx, killTarget, kill.Options{AutoEscalate: true})
	if err != nil {
		if errors.Is(err, kill.ErrPIDReused) {
			return exitErr(ExitPIDChanged, fmt.Errorf("port :%d: process changed since scan, refreshed", port))
		}
		if errors.Is(err, kill.ErrNotOwned) {
			return exitErr(ExitPermissionDenied, fmt.Errorf("port :%d: needs elevated permissions, try: sudo porthole", port))
		}
		return exitErr(ExitKillFailed, err)
	}
	if res.Status != kill.StatusKilled && res.Status != kill.StatusAlreadyDead {
		return exitErr(ExitKillFailed, fmt.Errorf("port :%d: process ignored kill signal, not restarting", port))
	}

	if err := app.Spawner.Spawn(plan); err != nil {
		return exitErr(ExitKillFailed, fmt.Errorf("killed %s on :%d but failed to respawn it: %w", target.Process, port, err))
	}

	fmt.Fprintf(app.Stdout, "restarted %s on :%d\n", target.Process, port)
	return nil
}
