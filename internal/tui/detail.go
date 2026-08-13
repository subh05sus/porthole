package tui

import (
	"fmt"
	"strings"

	"github.com/subh05sus/porthole/internal/output"
)

// detailView renders the full-cmdline/cwd/user/socket-list pane for
// m.detailTarget (PRD §5.4's "enter" binding).
func (m Model) detailView() string {
	s := *m.detailTarget

	var b strings.Builder
	b.WriteString(m.th.Header.Render(fmt.Sprintf("%s (pid %d)", processOr(s.Process), s.PID)))
	b.WriteString("\n\n")

	rows := [][2]string{
		{"Process", processOr(s.Process)},
		{"PID", fmt.Sprintf("%d", s.PID)},
		{"User", nonEmpty(s.User, "-")},
		{"Project", nonEmpty(s.Project, "-")},
		{"CWD", nonEmpty(s.CWD, "-")},
		{"Cmdline", nonEmpty(s.Cmdline, "-")},
		{"Uptime", output.FormatUptime(s.Uptime)},
		{"Owned", fmt.Sprintf("%v", s.Owned)},
	}
	if s.ResolveErr != nil {
		rows = append(rows, [2]string{"Resolve error", s.ResolveErr.Error()})
	}

	for _, r := range rows {
		b.WriteString(m.th.Muted.Render(padColumns(r[0], 14)))
		b.WriteString(r[1])
		b.WriteString("\n")
	}

	sockets := m.relatedSockets(s)
	b.WriteString("\n")
	b.WriteString(m.th.Muted.Render(fmt.Sprintf("sockets held by this process (%d):", len(sockets))))
	b.WriteString("\n")
	for _, sock := range sockets {
		b.WriteString(fmt.Sprintf("  %s :%d (%s)\n", sock.Proto, sock.Port, nonEmpty(sock.Addr, "-")))
	}

	b.WriteString("\n")
	b.WriteString(m.th.HintBar.Render("any key to close"))
	return b.String()
}

func processOr(s string) string { return nonEmpty(s, "?") }

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
