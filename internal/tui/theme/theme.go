// Package theme centralizes every lipgloss style the TUI uses, resolved
// once at startup so the rest of the TUI never has to think about color
// degradation itself.
package theme

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// Theme holds every style used by the TUI. Plain is true when animation and
// color must be skipped entirely (NO_COLOR set, or Windows virtual
// terminal processing couldn't be enabled — see platform_windows.go).
type Theme struct {
	Border   lipgloss.Style
	Header   lipgloss.Style
	HintBar  lipgloss.Style
	Selected lipgloss.Style
	Locked   lipgloss.Style
	Danger   lipgloss.Style
	Success  lipgloss.Style
	Muted    lipgloss.Style
	Plain    bool
}

// New resolves a Theme for the current environment and the user's
// config.Theme preference ("auto" | "color" | "plain"). Hard environment
// constraints always win over that preference: NO_COLOR (any value, per
// the convention at no-color.org) and forcePlain (e.g. a failed Windows
// VT100 enable) both force the plain theme regardless of mode — a config
// preference must never force color onto a terminal that genuinely can't
// render it. mode == "plain" can force plain even on a capable terminal,
// respecting an explicit preference. "auto" and "color" are functionally
// identical today — this package has only one color theme, no light/dark
// variants to choose between — "auto" exists for schema clarity ("let
// porthole decide") rather than a real second option.
func New(mode string, forcePlain bool) Theme {
	if mode == "plain" || forcePlain || noColorSet() {
		return plainTheme()
	}
	return colorTheme()
}

func noColorSet() bool {
	_, set := os.LookupEnv("NO_COLOR")
	return set
}

func plainTheme() Theme {
	plain := lipgloss.NewStyle()
	return Theme{
		Border:   plain,
		Header:   plain,
		HintBar:  plain,
		Selected: plain,
		Locked:   plain,
		Danger:   plain,
		Success:  plain,
		Muted:    plain,
		Plain:    true,
	}
}

// colorTheme uses lipgloss's own termenv-backed color profile detection,
// which degrades truecolor -> 256 -> 16 automatically based on the
// terminal's actual capability — no manual profile juggling needed here.
func colorTheme() Theme {
	return Theme{
		Border:   lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")),
		Header:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")),
		HintBar:  lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Selected: lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230")),
		Locked:   lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Danger:   lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		Success:  lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		Muted:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	}
}
