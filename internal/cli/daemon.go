package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/subh05sus/porthole/internal/daemon"
)

func newDaemonCmd(app *App) *cobra.Command {
	var (
		live     bool
		interval time.Duration
	)

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Auto-kill daemon: watches for allow-listed (port, process) matches and kills them (see auto_kill in ~/.porthole.yaml)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !app.Config.AutoKill.Enabled {
				return exitErr(1, fmt.Errorf("daemon: auto_kill.enabled is not set to true in ~/.porthole.yaml — refusing to start"))
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			resolved := resolveInterval(cmd.Flags().Changed("interval"), interval, time.Duration(app.Config.AutoKill.Interval))
			return runDaemon(ctx, app, live, resolved, nil)
		},
	}
	cmd.Flags().BoolVar(&live, "live", false, "actually kill matches instead of dry-run/notify-only (default: dry-run)")
	cmd.Flags().DurationVar(&interval, "interval", 5*time.Second, "how often to poll (overrides auto_kill.interval in ~/.porthole.yaml)")
	return cmd
}

// runDaemon drives daemon.Daemon and prints every action to app.Stdout/
// Stderr until ctx is cancelled. Takes ticks directly (mirroring
// runWatch) so tests can drive multiple poll cycles deterministically.
func runDaemon(ctx context.Context, app *App, live bool, interval time.Duration, ticks <-chan time.Time) error {
	d := &daemon.Daemon{Lister: app.Lister, Killer: app.Killer, Config: app.Config, Live: live}

	mode := "dry-run"
	if live {
		mode = "LIVE"
	}
	fmt.Fprintf(app.Stdout, "porthole daemon starting (%s mode, %d allow-list entr(y/ies))\n", mode, len(app.Config.AutoKill.Allow))

	events := d.Run(ctx, interval, ticks)
	for ev := range events {
		if ev.ScanErr != nil {
			fmt.Fprintf(app.Stderr, "scan error: %v\n", ev.ScanErr)
			continue
		}
		for _, a := range ev.Actions {
			switch {
			case a.Skipped:
				fmt.Fprintf(app.Stdout, "skip %s on :%d (%s)\n", a.Service.Process, a.Service.Port, a.SkipReason)
			case !a.Live:
				fmt.Fprintf(app.Stdout, "[dry-run] would kill %s on :%d (pid %d)\n", a.Service.Process, a.Service.Port, a.Service.PID)
			case a.Err != nil:
				fmt.Fprintf(app.Stderr, "kill failed for %s on :%d: %v\n", a.Service.Process, a.Service.Port, a.Err)
			default:
				fmt.Fprintf(app.Stdout, "killed %s on :%d (pid %d): status=%v\n", a.Service.Process, a.Service.Port, a.Service.PID, a.Result.Status)
			}
		}
	}
	return nil
}
