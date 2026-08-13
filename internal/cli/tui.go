package cli

import (
	"errors"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/subh05sus/porthole/internal/tui"
	"github.com/subh05sus/porthole/internal/tui/theme"
)

// ErrNotATerminal is returned when bare `porthole` (no subcommand) is run
// without a real terminal attached — matching lazygit's convention of
// launching the TUI by default, but refusing to try when piped rather than
// failing with a confusing bubbletea error.
var ErrNotATerminal = errors.New("porthole: requires an interactive terminal; try `porthole list` instead")

func runTUI(app *App) error {
	if !isTerminal(app.Stdout) {
		return ErrNotATerminal
	}

	forcePlain := !tui.EnableVirtualTerminal()
	th := theme.New(forcePlain)
	model := tui.New(app.Lister, app.Killer, th)

	p := tea.NewProgram(model, tea.WithInput(app.Stdin), tea.WithOutput(app.Stdout))
	_, err := p.Run()
	return err
}

// isTerminal checks whether w is a real console/tty via a type assertion
// against App's generic io.Writer (rather than a dedicated field), so App
// stays trivially fake-able in tests: a bytes.Buffer correctly reports
// false here without any special-casing.
func isTerminal(w io.Writer) bool {
	f, ok := w.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
