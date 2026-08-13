package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/subh05sus/porthole/internal/scan"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeFilter:
		return m.handleFilterKey(msg)
	case modeConfirmKill:
		return m.handleConfirmKillKey(msg)
	case modeConfirmEscalate:
		return m.handleConfirmEscalateKey(msg)
	case modeHelp:
		return m.handleHelpKey(msg)
	case modeKilling:
		// An operation is in flight; ignore input except quit rather than
		// let it fall through to nav/kill handling and start a second one.
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	default:
		return m.handleNormalKey(msg)
	}
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// PRD §5.4's keybinding table lists "j k" for Navigate in one row and
	// "k" for Kill in another — a literal conflict. Resolved in favor of
	// the PRD's own hint bar text ("↑↓ nav · k kill · K force"), which is
	// unambiguous: arrows (plus "j" for down, harmless since it doesn't
	// collide) navigate, and "k"/"K" are kill/force-kill only.
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		if m.watchCancel != nil {
			m.watchCancel()
		}
		m.quitting = true
		return m, tea.Quit

	case "w":
		return m.toggleWatch()

	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "down", "j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
		return m, nil

	case "r":
		m.scanning = true
		m.scanStart = m.clock()
		m.status = "scanning sockets"
		return m, m.scanCmd()

	case "/":
		m.mode = modeFilter
		m.filterInput.Focus()
		return m, nil

	case "?":
		m.mode = modeHelp
		return m, nil

	case " ":
		return m.toggleSelection()

	case "k":
		return m.beginKillConfirm(false)

	case "K":
		return m.beginKillConfirm(true)
	}

	return m, nil
}

// toggleSelection marks/unmarks the row under the cursor for multi-select
// bulk kill (PRD §5.4's "space" binding).
func (m Model) toggleSelection() (tea.Model, tea.Cmd) {
	target := m.selected()
	if target == nil {
		return m, nil
	}
	if m.multiSelected == nil {
		m.multiSelected = make(map[rowKey]bool)
	}
	key := keyOf(*target)
	if m.multiSelected[key] {
		delete(m.multiSelected, key)
	} else {
		m.multiSelected[key] = true
	}
	return m, nil
}

// bulkTargets resolves the current multi-selection against m.filtered. A
// selected key with nothing matching it anymore (e.g. the process already
// exited) is silently dropped rather than erroring.
func (m Model) bulkTargets() []scan.Service {
	if len(m.multiSelected) == 0 {
		return nil
	}
	var out []scan.Service
	for _, s := range m.filtered {
		if m.multiSelected[keyOf(s)] {
			out = append(out, s)
		}
	}
	return out
}

func (m Model) beginKillConfirm(force bool) (tea.Model, tea.Cmd) {
	targets := m.bulkTargets()
	if len(targets) == 0 {
		if t := m.selected(); t != nil {
			targets = []scan.Service{*t}
		}
	}
	if len(targets) == 0 {
		return m, nil
	}

	var owned, locked []scan.Service
	for _, t := range targets {
		if t.Owned {
			owned = append(owned, t)
		} else {
			locked = append(locked, t)
		}
	}
	if len(owned) == 0 {
		m.status = "needs elevated permissions, try running as Administrator/root"
		return m, nil
	}

	m.mode = modeConfirmKill
	m.pendingBulk = owned
	m.force = force

	verb := "kill"
	if force {
		verb = "force kill"
	}
	switch {
	case len(owned) == 1 && len(locked) == 0:
		m.status = verb + " " + owned[0].Process + " on :" + portString(owned[0].Port) + "? (y/n)"
	case len(owned) == 1:
		m.status = fmt.Sprintf("%s %s on :%d? (%d locked rows skipped) (y/n)", verb, owned[0].Process, owned[0].Port, len(locked))
	default:
		m.status = fmt.Sprintf("%s %d selected services?", verb, len(owned))
		if len(locked) > 0 {
			m.status += fmt.Sprintf(" (%d locked rows skipped)", len(locked))
		}
		m.status += " (y/n)"
	}
	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filterInput.SetValue("")
		m.filterInput.Blur()
		m.mode = modeNormal
		m.applyFilter()
		return m, nil
	case "enter":
		m.filterInput.Blur()
		m.mode = modeNormal
		m.applyFilter()
		return m, nil
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.applyFilter()
	return m, cmd
}

func (m Model) handleConfirmKillKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		targets := m.pendingBulk
		m.pendingBulk = nil
		m.mode = modeKilling
		m.killStart = m.clock()
		m.killCount = len(targets)

		if len(targets) == 1 {
			m.status = "sending kill signal…"
			return m, m.killCmd(targets[0], m.force)
		}
		m.status = fmt.Sprintf("sending kill signal to %d services…", len(targets))
		return m, m.bulkKillCmd(targets, m.force)
	default:
		m.mode = modeNormal
		m.pendingBulk = nil
		m.setEphemeralStatus("kill cancelled", ephemeralStatusDuration)
		return m, nil
	}
}

func (m Model) handleConfirmEscalateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		target := *m.pending
		m.mode = modeKilling
		m.killStart = m.clock()
		m.status = "sending force kill…"
		return m, m.escalateCmd(target)
	default:
		m.mode = modeNormal
		m.pending = nil
		m.setEphemeralStatus("kill cancelled", ephemeralStatusDuration)
		return m, nil
	}
}

func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	return m, nil
}

func portString(port int) string {
	// Tiny local helper to avoid pulling in strconv just for this.
	if port == 0 {
		return "0"
	}
	neg := port < 0
	if neg {
		port = -port
	}
	var digits []byte
	for port > 0 {
		digits = append([]byte{byte('0' + port%10)}, digits...)
		port /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

// toggleWatch turns watch mode on or off. Turning it on starts
// scan.Watch's own polling goroutine (independent of the TUI's 60ms
// animation tick) and begins consuming its events via the standard
// bubbletea "listen on channel, requeue Cmd" pattern; turning it off just
// cancels that context, letting scan.Watch's own goroutine exit on its own.
func (m Model) toggleWatch() (tea.Model, tea.Cmd) {
	if m.watching {
		if m.watchCancel != nil {
			m.watchCancel()
		}
		m.watching = false
		m.watchCancel = nil
		m.watchEvents = nil
		return m, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.watching = true
	m.watchCancel = cancel
	m.watchEvents = scan.Watch(ctx, m.lister, watchInterval, nil)
	return m, m.waitForWatchEvent()
}
