package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
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
	case modeDetail:
		return m.handleDetailKey(msg)
	case modeConfirmRestart:
		return m.handleConfirmRestartKey(msg)
	case modeConfirmProtected:
		return m.handleConfirmProtectedKey(msg)
	case modeKilling, modeRestarting:
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

	case "R":
		return m.beginRestartConfirm()

	case "enter":
		if t := m.selected(); t != nil {
			target := *t
			m.detailTarget = &target
			m.mode = modeDetail

			var cmds []tea.Cmd
			if m.querier != nil {
				m.detailSockets = nil
				m.detailSocketsLoading = true
				cmds = append(cmds, m.detailSocketsCmd(target.PID))
			} else {
				m.detailSockets = m.relatedSockets(target)
				m.detailSocketsLoading = false
			}
			if m.resQuerier != nil {
				m.detailResources = nil
				m.detailResourcesErr = nil
				m.detailResourcesLoading = true
				cmds = append(cmds, m.detailResourcesCmd(target.PID))
			}
			return m, tea.Batch(cmds...)
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "q" || msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}
	m.mode = modeNormal
	m.detailTarget = nil
	m.detailSockets = nil
	m.detailSocketsLoading = false
	m.detailResources = nil
	m.detailResourcesErr = nil
	m.detailResourcesLoading = false
	return m, nil
}

// relatedSockets returns every service in m.services sharing target's PID
// — PRD §5.4's detail pane calls for "the full socket list," and Service is
// one row per socket rather than one row per process, so this groups them
// by PID at render time rather than needing a new data model.
func (m Model) relatedSockets(target scan.Service) []scan.Service {
	var out []scan.Service
	for _, s := range m.services {
		if s.PID == target.PID {
			out = append(out, s)
		}
	}
	return out
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

	// Protected ports need typed confirmation, which doesn't make sense to
	// combine with a multi-target bulk kill — refuse and point the user at
	// killing them individually rather than guessing what "type the port"
	// would even mean across several different protected ports at once.
	var protectedPorts []int
	for _, t := range owned {
		if protected, _ := m.cfg.IsProtected(t.Port); protected {
			protectedPorts = append(protectedPorts, t.Port)
		}
	}
	if len(protectedPorts) > 0 {
		if len(owned) == 1 {
			return m.beginProtectedConfirm(owned[0], force)
		}
		m.status = fmt.Sprintf("port(s) %v are protected — kill them individually, not as part of a bulk selection", protectedPorts)
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

// beginProtectedConfirm starts the typed-port confirmation flow (v1.3) for
// a single protected-port kill.
func (m Model) beginProtectedConfirm(target scan.Service, force bool) (tea.Model, tea.Cmd) {
	_, reason := m.cfg.IsProtected(target.Port)

	ti := textinput.New()
	ti.Placeholder = fmt.Sprintf("type %d to confirm", target.Port)
	ti.Focus()
	m.confirmInput = ti

	m.mode = modeConfirmProtected
	m.pendingBulk = []scan.Service{target}
	m.force = force

	prompt := fmt.Sprintf("port :%d is protected", target.Port)
	if reason != "" {
		prompt += " (" + reason + ")"
	}
	m.status = prompt
	return m, nil
}

func (m Model) handleConfirmProtectedKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.pendingBulk = nil
		m.setEphemeralStatus("kill cancelled", ephemeralStatusDuration)
		return m, nil

	case "enter":
		target := m.pendingBulk[0]
		m.pendingBulk = nil
		if strings.TrimSpace(m.confirmInput.Value()) != portString(target.Port) {
			m.mode = modeNormal
			m.setEphemeralStatus("confirmation did not match, cancelled", ephemeralStatusDuration)
			return m, nil
		}
		m.mode = modeKilling
		m.killStart = m.clock()
		m.killCount = 1
		m.status = "sending kill signal…"
		return m, m.killCmd(target, m.force)
	}

	var cmd tea.Cmd
	m.confirmInput, cmd = m.confirmInput.Update(msg)
	return m, cmd
}

// beginRestartConfirm starts the restart confirmation flow for the row
// under the cursor. Single-target only — restart is inherently per-process
// (FUTURE_PLANS.md v1.2 never mentions a bulk form), so it doesn't consult
// multiSelected the way beginKillConfirm does.
func (m Model) beginRestartConfirm() (tea.Model, tea.Cmd) {
	target := m.selected()
	if target == nil {
		return m, nil
	}
	if !target.Owned {
		m.status = "needs elevated permissions, try running as Administrator/root"
		return m, nil
	}
	if target.ContainerID != "" {
		// See internal/cli/restart.go's identical refusal: the captured
		// cmdline belongs to the host-side forwarding process, not the
		// containerized one, so respawning it would be meaningless.
		m.status = "container-backed port — restart not supported, use `docker restart " + target.Container + "`"
		return m, nil
	}

	t := *target
	m.mode = modeConfirmRestart
	m.pendingRestart = &t
	m.status = "restart " + t.Process + " on :" + portString(t.Port) + "? (y/n)"
	return m, nil
}

func (m Model) handleConfirmRestartKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		target := *m.pendingRestart
		m.mode = modeRestarting
		m.restartStart = m.clock()
		m.status = "restarting…"
		return m, m.restartCmd(target)
	default:
		m.mode = modeNormal
		m.pendingRestart = nil
		m.setEphemeralStatus("restart cancelled", ephemeralStatusDuration)
		return m, nil
	}
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
