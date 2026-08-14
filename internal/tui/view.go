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
	if m.mode == modeDetail && m.detailTarget != nil {
		return m.detailView()
	}

	var b strings.Builder

	header := m.status
	if m.watching {
		// PRD §5.3: "a subtle dot in the header pulses each refresh cycle
		// so you know it is live." Toggled once per watch Event received
		// (handleWatchEvent), not on every 60ms tick — a pulse per scan
		// cycle, not a fast blink.
		dot := "○"
		if m.watchPulseOn {
			dot = "●"
		}
		header = dot + " " + header
	}
	b.WriteString(m.th.Header.Render(header))
	b.WriteString("\n")
	if m.sudoBanner != "" {
		b.WriteString(m.th.Danger.Render(m.sudoBanner))
		b.WriteString("\n")
	}
	b.WriteString("\n")
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
	if m.mode == modeConfirmProtected {
		b.WriteString(m.confirmInput.View())
		b.WriteString("\n")
	}

	b.WriteString(m.th.HintBar.Render(m.hintBar()))
	return b.String()
}

// visibleRows returns how many data rows the table box can draw without
// pushing the rest of the view past the terminal height — every other line
// the View draws (header, banner, blanks, the box's own border edges and
// column header, the hint bar, and the filter/confirm input line when
// active) is fixed at one line each, so whatever's left over is the row
// budget. Returns -1 when the terminal size isn't known yet (no
// WindowSizeMsg has landed — notably true throughout the test suite),
// meaning "unbounded": render every row, the old behavior.
func (m Model) visibleRows() int {
	if m.height <= 0 {
		return -1
	}
	// header + blank-after-header + border-top + column-header +
	// border-bottom + blank-after-box + hint-bar.
	fixed := 7
	if m.sudoBanner != "" {
		fixed++
	}
	if m.mode == modeFilter || m.mode == modeConfirmProtected {
		fixed++
	}
	visible := m.height - fixed
	if visible < 1 {
		visible = 1
	}
	return visible
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
	fadingExtra := m.orphanedFadingRows()
	if len(m.filtered) == 0 && len(fadingExtra) == 0 {
		b.WriteString(m.th.Muted.Render("  no services listening"))
		return b.String()
	}

	// visibleRows()==-1 means the terminal size isn't known yet: fall back
	// to the old unbounded behavior (also what the test suite exercises,
	// since it never sends a WindowSizeMsg). Otherwise window the list to
	// the offset ensureCursorVisible has already computed, so the box stays
	// a fixed height and never overflows the terminal — PORT-xx: previously
	// every filtered row rendered unconditionally, so a list longer than
	// the terminal spilled past it and up/down navigation looked broken
	// once the cursor scrolled off screen.
	visible := m.visibleRows()
	start, end := 0, len(m.filtered)
	if visible >= 0 {
		start = m.listOffset
		if start > len(m.filtered) {
			start = len(m.filtered)
		}
		end = start + visible
		if end > len(m.filtered) {
			end = len(m.filtered)
		}
	}

	revealElapsed := m.clock().Sub(m.revealStart)
	rendered := 0
	for i := start; i < end; i++ {
		s := m.filtered[i]
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
	// Watch-mode removals disappear from m.filtered the instant a fresh
	// scan lands (unlike a user-initiated kill, which deliberately delays
	// its rescan — see handleKillResult), so a still-dissolving removed
	// row has to be appended here rather than found in the normal loop.
	// Only room left in the current window gets one, since they're
	// transient anyway and must not push the box past its fixed height.
	for _, s := range fadingExtra {
		if visible >= 0 && rendered >= visible {
			break
		}
		if rendered > 0 {
			b.WriteString("\n")
		}
		b.WriteString(m.renderRow(s, false))
		rendered++
	}
	return b.String()
}

// orphanedFadingRows returns fadingOut rows no longer present in
// m.filtered — i.e. ones that must be rendered as extra rows rather than
// found during the normal m.filtered loop.
func (m Model) orphanedFadingRows() []scan.Service {
	present := make(map[rowKey]bool, len(m.filtered))
	for _, s := range m.filtered {
		present[keyOf(s)] = true
	}
	var out []scan.Service
	for _, f := range m.fadingOut {
		if !present[keyOf(f.service)] {
			out = append(out, f.service)
		}
	}
	return out
}

func (m Model) renderRow(s scan.Service, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}
	protected, _ := m.cfg.IsProtected(s.Port)
	mark := "  "
	switch {
	case m.multiSelected[keyOf(s)]:
		mark = "✓ "
	case protected:
		mark = "🛡 "
	case !s.Owned:
		mark = "🔒 "
	case s.ContainerID != "":
		mark = "🐳 "
	}

	process := s.Process
	if process == "" {
		process = "?"
	}
	project := s.Project
	if project == "" {
		project = "-"
	}

	row := cursor + mark + padColumns(
		fmt.Sprintf("%d", s.Port), colPort-4,
		fmt.Sprintf("%d", s.PID), colPID,
		process, colProcess,
		project, colProject,
		output.FormatUptime(s.Uptime), colUptime,
	)

	if f, ok := m.findFading(s); ok {
		// PRD §5.2: a user-initiated kill shows "terminated" in green
		// before dissolving. A watch-mode removal (the process just exited
		// on its own) has nothing to celebrate, so it just dims throughout.
		if f.triggersRescan {
			stage := anim.FadeOutStage(m.clock().Sub(f.startedAt), killDissolveDuration)
			if stage < anim.FadeStages/2 {
				return m.th.Success.Render(row)
			}
		}
		return m.th.Muted.Render(row)
	}
	if _, ok := m.recentlyAdded[keyOf(s)]; ok {
		return m.th.Success.Render(row)
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

func (m Model) findFading(s scan.Service) (fadeEntry, bool) {
	key := keyOf(s)
	for _, f := range m.fadingOut {
		if keyOf(f.service) == key {
			return f, true
		}
	}
	return fadeEntry{}, false
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
	case modeConfirmKill, modeConfirmEscalate, modeConfirmRestart:
		return "y confirm · any other key cancel"
	case modeConfirmProtected:
		return "enter confirm · esc cancel"
	case modeKilling, modeRestarting:
		return "working…"
	default:
		hints := "↑↓ nav · space select · k kill · K force · / filter · w watch · r refresh · ? help · q quit"
		if pos := m.scrollPosition(); pos != "" {
			hints = pos + " · " + hints
		}
		return hints
	}
}

// scrollPosition returns a "12-19/40" indicator when the table is windowed
// to fewer rows than m.filtered holds, so a scrolled list still tells the
// user how much more there is and where the visible slice sits — otherwise
// a fixed-height box with no indicator would look truncated rather than
// scrollable. Empty string when everything fits (nothing to indicate) or
// the terminal size isn't known yet (visibleRows()==-1).
func (m Model) scrollPosition() string {
	visible := m.visibleRows()
	if visible < 0 || len(m.filtered) <= visible {
		return ""
	}
	start := m.listOffset + 1
	end := m.listOffset + visible
	if end > len(m.filtered) {
		end = len(m.filtered)
	}
	return fmt.Sprintf("%d-%d/%d", start, end, len(m.filtered))
}
