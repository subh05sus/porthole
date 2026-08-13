package output

import (
	"fmt"
	"io"

	"github.com/subh05sus/porthole/internal/scan"
)

// OneLine writes a compact single-line summary — FUTURE_PLANS.md's
// tmux/zellij status-line integration idea ("porthole --oneline"). Never
// drops a row silently elsewhere in this package, but a status line has no
// room for per-row detail, so this intentionally only ever reports counts.
func OneLine(w io.Writer, services []scan.Service) error {
	locked := 0
	for _, s := range services {
		if !s.Owned {
			locked++
		}
	}

	summary := fmt.Sprintf("%d service", len(services))
	if len(services) != 1 {
		summary += "s"
	}
	if locked > 0 {
		summary += fmt.Sprintf(" · %d locked", locked)
	}

	_, err := fmt.Fprintln(w, summary)
	return err
}
