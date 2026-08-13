// Package output renders []scan.Service for the CLI: a plain table for
// humans and JSON for scripts.
package output

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/subh05sus/porthole/internal/scan"
)

// Table writes a human-readable, aligned table to w. Rows the current user
// cannot own are marked "(locked)" rather than silently omitted — a service
// is never dropped just because part of it couldn't be resolved.
func Table(w io.Writer, services []scan.Service) error {
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "PORT\tPID\tPROCESS\tPROJECT\tUPTIME")
	for _, s := range services {
		fmt.Fprintf(tw, "%d\t%d\t%s\t%s\t%s\n", s.Port, s.PID, processCell(s), projectCell(s), FormatUptime(s.Uptime))
	}
	return tw.Flush()
}

func processCell(s scan.Service) string {
	name := s.Process
	if name == "" {
		name = "?"
	}
	if !s.Owned {
		name += " (locked)"
	}
	return name
}

func projectCell(s scan.Service) string {
	if s.Project == "" {
		return "-"
	}
	return s.Project
}

// FormatUptime renders a duration the way the PRD's example table does:
// "2h 14m", "18m", "6d". Durations too small to resolve render as "-".
func FormatUptime(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	d = d.Round(time.Minute)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute

	switch {
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}
