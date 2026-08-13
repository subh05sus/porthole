package tui

import (
	"fmt"
	"strings"

	"github.com/subh05sus/porthole/internal/output"
	"github.com/subh05sus/porthole/internal/scan"
	"github.com/subh05sus/porthole/internal/tui/anim"
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
	// PRD §5.2's mockup embeds the status text in the border's top edge;
	// lipgloss has no built-in "titled border" slot, so the status line is
	// rendered above the box instead — a cosmetic, not functional,
	// deviation from the literal ASCII mockup.
	b.WriteString(m.th.Border.Padding(0, 1).Render(m.renderTable()))
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

	revealElapsed := m.clock().Sub(m.revealStart)
	rendered := 0
	for i, s := range m.filtered {
		// Streaming reveal (PRD §5.3): rows appear staggered rather than
		// all at once. Since Lister.List isn't actually streaming, this
		// simulates the cascade against the already-resolved list — a row
		// not yet due simply isn't drawn this frame, so the list visibly
		// grows over ~400ms after each scan.
		if !anim.Revealed(i, revealElapsed) {
			break
		}
		if rendered > 0 {
			b.WriteString("\n")
		}
		b.WriteString(m.renderRow(s, i == m.cursor))
		rendered++
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

	if m.isDying(s) {
		stage := anim.FadeOutStage(m.clock().Sub(m.dyingStart), killDissolveDuration)
		if stage < anim.FadeStages/2 {
			return m.th.Success.Render(row)
		}
		return m.th.Muted.Render(row)
	}

	switch {
	case selected:
		return m.th.Selected.Render(row)
	case !s.Owned:
		return m.th.Locked.Render(row)
	default:
		return row
	}
}

func (m Model) isDying(s scan.Service) bool {
	return m.dying != nil && m.dying.PID == s.PID && m.dying.Port == s.Port
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
	case modeKilling:
		return "working…"
	default:
		return "↑↓ nav · k kill · K force · / filter · r refresh · ? help · q quit"
	}
}
