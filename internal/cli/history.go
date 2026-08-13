package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/subh05sus/porthole/internal/history"
)

func newHistoryCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show recent kill history",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHistory(app)
		},
	}
	return cmd
}

func runHistory(app *App) error {
	if app.HistoryPath == "" {
		fmt.Fprintln(app.Stdout, "no kill history yet")
		return nil
	}

	entries, err := history.ReadAll(app.HistoryPath)
	if err != nil {
		return exitErr(ExitKillFailed, err)
	}
	if len(entries) == 0 {
		fmt.Fprintln(app.Stdout, "no kill history yet")
		return nil
	}

	for _, e := range entries {
		line := fmt.Sprintf("%s pid=%-8d %s", e.Timestamp.Format(time.RFC3339), e.PID, e.Event)
		if e.Force {
			line += " force=true"
		}
		if e.Status != "" {
			line += " status=" + e.Status
		}
		if e.Err != "" {
			line += " error=" + e.Err
		}
		fmt.Fprintln(app.Stdout, line)
	}
	return nil
}
