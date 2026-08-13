package tui

import "strings"

var helpEntries = []struct {
	key, desc string
}{
	{"↑ / ↓", "navigate"},
	{"j", "navigate down (vim-style alias)"},
	{"k", "kill selected (SIGTERM, then prompt to escalate)"},
	{"K", "force kill (SIGKILL, still confirms)"},
	{"space", "toggle multi-select for bulk kill"},
	{"/", "fuzzy filter by port, process, or project"},
	{"w", "toggle watch mode (live-updating list)"},
	{"r", "manual refresh"},
	{"?", "toggle this help"},
	{"q / esc", "quit"},
}

func (m Model) helpView() string {
	var b strings.Builder
	b.WriteString(m.th.Header.Render("porthole — help"))
	b.WriteString("\n\n")
	for _, e := range helpEntries {
		b.WriteString(m.th.Muted.Render(padColumns(e.key, 10)))
		b.WriteString(e.desc)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.th.HintBar.Render("any key to close"))
	return b.String()
}
