package tui

import (
	"fmt"
	"strings"

	"github.com/subh05sus/porthole/internal/output"
	"github.com/subh05sus/porthole/internal/scan"
)

const (
	colPort    = 7
	colPID     = 8
	colProcess = 20
	colProject = 18
	colUptime  = 8
)

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.mode == modeHelp {
		return m.helpView()
	}

	var b strings.Builder

	b.WriteString(m.th.Header.Render(m.status))
	b.WriteString("\n\n")
	b.WriteString(m.renderTable())
	b.WriteString("\n")

	if m.mode == modeFilter {
		b.WriteString(m.filterInput.View())
		b.WriteString("\n")
	}

	b.WriteString(m.th.HintBar.Render(m.hintBar()))
	return b.String()
}

func (m Model) renderTable() string {
	var b strings.Builder

	header := padColumns("PORT", colPort, "PID", colPID, "PROCESS", colProcess, "PROJECT", colProject, "UPTIME", colUptime)
	b.WriteString(m.th.Muted.Render(header))
	b.WriteString("\n")

	if m.scanErr != nil {
		b.WriteString(m.th.Danger.Render("  scan error: " + m.scanErr.Error()))
		return b.String()
	}
	if len(m.filtered) == 0 {
		b.WriteString(m.th.Muted.Render("  no services listening"))
		return b.String()
	}

	for i, s := range m.filtered {
		b.WriteString(m.renderRow(s, i == m.cursor))
		if i < len(m.filtered)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (m Model) renderRow(s scan.Service, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}
	lock := "  "
	if !s.Owned {
		lock = "🔒 "
	}

	process := s.Process
	if process == "" {
		process = "?"
	}
	project := s.Project
	if project == "" {
		project = "-"
	}

	row := cursor + lock + padColumns(
		fmt.Sprintf("%d", s.Port), colPort-4,
		fmt.Sprintf("%d", s.PID), colPID,
		process, colProcess,
		project, colProject,
		output.FormatUptime(s.Uptime), colUptime,
	)

	switch {
	case selected:
		return m.th.Selected.Render(row)
	case !s.Owned:
		return m.th.Locked.Render(row)
	default:
		return row
	}
}

// padColumns takes alternating (value, width) pairs and left-pads each
// value into a fixed-width field. Values longer than their column simply
// overflow rather than being truncated — acceptable for v1.
func padColumns(pairs ...any) string {
	var b strings.Builder
	for i := 0; i+1 < len(pairs); i += 2 {
		val := pairs[i].(string)
		width := pairs[i+1].(int)
		b.WriteString(val)
		if pad := width - len(val); pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		} else {
			b.WriteString(" ")
		}
	}
	return b.String()
}

func (m Model) hintBar() string {
	switch m.mode {
	case modeFilter:
		return "enter apply · esc cancel"
	case modeConfirmKill, modeConfirmEscalate:
		return "y confirm · any other key cancel"
	default:
		return "↑↓ nav · k kill · K force · / filter · r refresh · ? help · q quit"
	}
}
