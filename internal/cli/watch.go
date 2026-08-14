package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/subh05sus/porthole/internal/notify"
	"github.com/subh05sus/porthole/internal/output"
	"github.com/subh05sus/porthole/internal/scan"
)

func newWatchCmd(app *App) *cobra.Command {
	var (
		interval   time.Duration
		notifyFlag bool
		all        bool
	)

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Headless live tail of listening services, no TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			var notifier notify.Notifier = notify.NoOp{}
			if notifyFlag {
				notifier = notify.NewDefaultNotifier()
			}
			resolved := resolveInterval(cmd.Flags().Changed("interval"), interval, time.Duration(app.Config.Watch.Interval))
			return runWatch(ctx, app, resolved, nil, notifier, all)
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "how often to rescan (overrides watch.interval in ~/.porthole.yaml)")
	cmd.Flags().BoolVar(&notifyFlag, "notify", false, "send a desktop notification when a new service starts listening")
	cmd.Flags().BoolVar(&all, "all", false, "show every service, ignoring display.hide_* settings in ~/.porthole.yaml")
	return cmd
}

// runWatch drives scan.Watch and prints to app.Stdout/Stderr until ctx is
// cancelled: the first scan prints the full table as a baseline, and every
// scan after that prints only what changed. Takes ticks directly (rather
// than always going through the real timer) so tests can drive multiple
// scans deterministically instead of waiting on wall-clock time. notifier
// fires only for newly-appeared services, never removals — matching the
// plan's "notify when a watched port appears." Unless all is true, every
// printed service, addition, and removal passes through the same
// display-only filter list uses — including a hidden newly-appeared
// service getting neither a "+" line nor a notification.
func runWatch(ctx context.Context, app *App, interval time.Duration, ticks <-chan time.Time, notifier notify.Notifier, all bool) error {
	events := scan.Watch(ctx, app.Lister, interval, ticks)

	filter := func(services []scan.Service) []scan.Service {
		if all {
			return services
		}
		return scan.FilterDisplay(services, app.Config.Display.HideSystemProcesses, app.Config.Display.HidePrivilegedPorts)
	}

	first := true
	for ev := range events {
		if ev.Err != nil {
			fmt.Fprintf(app.Stderr, "scan error: %v\n", ev.Err)
			continue
		}
		if first {
			_ = output.Table(app.Stdout, filter(ev.Services))
			first = false
			continue
		}
		for _, s := range filter(ev.Diff.Added) {
			fmt.Fprintf(app.Stdout, "+ %s on :%d (pid %d)\n", watchDisplayName(s), s.Port, s.PID)
			notifier.Notify("porthole: new service", fmt.Sprintf("%s started listening on :%d", watchDisplayName(s), s.Port))
		}
		for _, s := range filter(ev.Diff.Removed) {
			fmt.Fprintf(app.Stdout, "- %s on :%d (pid %d)\n", watchDisplayName(s), s.Port, s.PID)
		}
	}
	return nil
}

func watchDisplayName(s scan.Service) string {
	if s.Process == "" {
		return "?"
	}
	return s.Process
}
