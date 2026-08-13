package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/subh05sus/porthole/internal/output"
	"github.com/subh05sus/porthole/internal/scan"
)

func newWatchCmd(app *App) *cobra.Command {
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Headless live tail of listening services, no TUI",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			return runWatch(ctx, app, interval, nil)
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "how often to rescan")
	return cmd
}

// runWatch drives scan.Watch and prints to app.Stdout/Stderr until ctx is
// cancelled: the first scan prints the full table as a baseline, and every
// scan after that prints only what changed. Takes ticks directly (rather
// than always going through the real timer) so tests can drive multiple
// scans deterministically instead of waiting on wall-clock time.
func runWatch(ctx context.Context, app *App, interval time.Duration, ticks <-chan time.Time) error {
	events := scan.Watch(ctx, app.Lister, interval, ticks)

	first := true
	for ev := range events {
		if ev.Err != nil {
			fmt.Fprintf(app.Stderr, "scan error: %v\n", ev.Err)
			continue
		}
		if first {
			_ = output.Table(app.Stdout, ev.Services)
			first = false
			continue
		}
		for _, s := range ev.Diff.Added {
			fmt.Fprintf(app.Stdout, "+ %s on :%d (pid %d)\n", watchDisplayName(s), s.Port, s.PID)
		}
		for _, s := range ev.Diff.Removed {
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
