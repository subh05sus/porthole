// Package tui implements porthole's interactive Bubble Tea interface.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/scan"
	"github.com/subh05sus/porthole/internal/tui/anim"
	"github.com/subh05sus/porthole/internal/tui/theme"
)

// scanTimeout bounds every scan per PRD §8.3 — a slow scan renders partial
// results plus a warning instead of freezing the UI.
const scanTimeout = 3 * time.Second

// tickInterval drives every animation in the TUI from one recurring
// tea.Tick, per PRD §5.3's "everything drives off one tea.Tick at 60ms. No
// per-row goroutines. No timers stacking."
const tickInterval = 60 * time.Millisecond

// killDissolveDuration is how long a just-killed row stays visible,
// dissolving, before the list reflows (PRD §5.2: "~400ms").
const killDissolveDuration = 400 * time.Millisecond

type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeConfirmKill
	modeConfirmEscalate
	modeKilling
	modeHelp
)

// Model is porthole's Bubble Tea Model. Init/Update/View never call the
// scanner or killer directly on this goroutine — every OS interaction is
// dispatched as a tea.Cmd so the UI thread never blocks (PRD §8.3).
type Model struct {
	lister scan.Lister
	killer kill.Killer
	th     theme.Theme

	// clock is injectable so animation timing is deterministic in tests
	// (see model_test.go) instead of racing real wall-clock time.
	clock func() time.Time

	services []scan.Service
	filtered []scan.Service
	cursor   int

	filterInput textinput.Model

	mode      mode
	pending   *scan.Service // row awaiting kill/escalate/in-flight confirmation
	force     bool          // whether the pending kill is the force (K) path
	killStart time.Time     // when the in-flight kill/escalate began, for the spinner

	dying      *scan.Service // just-killed row still dissolving
	dyingStart time.Time

	status       string
	statusExpiry time.Time // while non-zero and in the future, status overrides computeNormalStatus
	scanErr      error
	scanning     bool
	scanStart    time.Time
	revealStart  time.Time

	width, height int
	quitting      bool
}

// New builds a Model. lister and killer are injected so this whole package
// can be tested without a real OS (see model_test.go).
func New(lister scan.Lister, killer kill.Killer, th theme.Theme) Model {
	ti := textinput.New()
	ti.Placeholder = "filter by port, process, or project"
	ti.Prompt = "/ "

	clock := time.Now
	return Model{
		lister:      lister,
		killer:      killer,
		th:          th,
		clock:       clock,
		filterInput: ti,
		status:      "scanning sockets",
		scanning:    true,
		scanStart:   clock(),
	}
}

// Init cannot mutate the Model — bubbletea's Init() returns only a Cmd, and
// any receiver mutation here would be discarded — so the initial
// scanning/scanStart state is set directly in New() instead.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.scanCmd(), m.tickCmd())
}

type scanCompleteMsg struct {
	services []scan.Service
	err      error
}

func (m Model) scanCmd() tea.Cmd {
	lister := m.lister
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
		defer cancel()
		services, err := lister.List(ctx)
		return scanCompleteMsg{services: services, err: err}
	}
}

type killResultMsg struct {
	target scan.Service
	force  bool
	result kill.Result
	err    error
}

func (m Model) killCmd(target scan.Service, force bool) tea.Cmd {
	killer := m.killer
	t := kill.Target{PID: target.PID, StartTime: target.StartTime, Owned: target.Owned}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := killer.Execute(ctx, t, kill.Options{Force: force})
		return killResultMsg{target: target, force: force, result: res, err: err}
	}
}

func (m Model) escalateCmd(target scan.Service) tea.Cmd {
	killer := m.killer
	t := kill.Target{PID: target.PID, StartTime: target.StartTime, Owned: target.Owned}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := killer.Escalate(ctx, t)
		return killResultMsg{target: target, force: true, result: res, err: err}
	}
}

type tickMsg time.Time

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case scanCompleteMsg:
		m.scanning = false
		m.scanErr = msg.err
		m.services = msg.services
		m.revealStart = m.clock()
		m.applyFilter()
		return m, nil

	case killResultMsg:
		return m.handleKillResult(msg)

	case tickMsg:
		return m.handleTick()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleTick() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.dying != nil {
		elapsed := m.clock().Sub(m.dyingStart)
		if anim.FadeOutComplete(elapsed, killDissolveDuration) {
			m.dying = nil
			cmd = m.scanCmd()
		}
	}

	switch {
	case m.mode == modeNormal && (m.statusExpiry.IsZero() || !m.clock().Before(m.statusExpiry)):
		m.status = m.computeNormalStatus()
	case m.mode == modeKilling:
		verb := "sending kill signal"
		if m.force {
			verb = "sending force kill"
		}
		m.status = fmt.Sprintf("%s… %c", verb, anim.SpinnerFrame(m.clock().Sub(m.killStart)))
	}

	return m, tea.Batch(cmd, m.tickCmd())
}

// ephemeralStatusDuration is how long a one-off message (kill
// cancelled/succeeded/failed) stays on screen in modeNormal before
// computeNormalStatus resumes driving the header text.
const ephemeralStatusDuration = 2 * time.Second

func (m *Model) setEphemeralStatus(text string, dur time.Duration) {
	m.status = text
	m.statusExpiry = m.clock().Add(dur)
}

// computeNormalStatus implements PRD §5.3's "talking status line": the
// header text progresses "scanning sockets" -> "resolving N processes" ->
// "N services listening" as time passes since the last scan, rather than
// tracking literal per-service resolution (Lister.List returns everything
// in one call, so this is a scripted animation beat keyed off elapsed time
// and the final count, not real streaming progress).
func (m Model) computeNormalStatus() string {
	if m.scanErr != nil {
		return fmt.Sprintf("scan timed out or failed: %v", m.scanErr)
	}
	if m.scanning {
		return fmt.Sprintf("scanning sockets %c", anim.SpinnerFrame(m.clock().Sub(m.scanStart)))
	}
	elapsed := m.clock().Sub(m.revealStart)
	if elapsed < anim.RevealCap {
		return fmt.Sprintf("resolving %d processes %c", len(m.services), anim.SpinnerFrame(elapsed))
	}
	return fmt.Sprintf("%d services listening", len(m.services))
}

func (m *Model) applyFilter() {
	query := strings.TrimSpace(m.filterInput.Value())
	if query == "" {
		m.filtered = m.services
	} else {
		m.filtered = filterServices(m.services, query)
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// filterServices does a case-insensitive substring match across port,
// process, and project — the v1 reading of PRD §5.4's "fuzzy filter" (see
// TODO.md for the deferred true-fuzzy-matching upgrade).
func filterServices(services []scan.Service, query string) []scan.Service {
	q := strings.ToLower(query)
	out := make([]scan.Service, 0, len(services))
	for _, s := range services {
		port := fmt.Sprintf("%d", s.Port)
		if strings.Contains(port, q) ||
			strings.Contains(strings.ToLower(s.Process), q) ||
			strings.Contains(strings.ToLower(s.Project), q) {
			out = append(out, s)
		}
	}
	return out
}

func (m Model) handleKillResult(msg killResultMsg) (tea.Model, tea.Cmd) {
	m.pending = nil

	switch {
	case msg.err != nil:
		m.mode = modeNormal
		m.setEphemeralStatus(fmt.Sprintf("port :%d: %v", msg.target.Port, msg.err), ephemeralStatusDuration)
		return m, nil

	case msg.result.Status == kill.StatusKilled || msg.result.Status == kill.StatusAlreadyDead:
		m.mode = modeNormal
		m.dying = &msg.target
		m.dyingStart = m.clock()
		m.setEphemeralStatus(fmt.Sprintf("terminated %s on :%d", msg.target.Process, msg.target.Port), killDissolveDuration)
		return m, nil

	case msg.result.Status == kill.StatusNeedsEscalation && !msg.force:
		m.mode = modeConfirmEscalate
		m.pending = &msg.target
		m.status = fmt.Sprintf("process ignored SIGTERM, force kill %s on :%d? (y/n)", msg.target.Process, msg.target.Port)
		return m, nil

	default:
		m.mode = modeNormal
		m.setEphemeralStatus(fmt.Sprintf("port :%d: process ignored kill signal", msg.target.Port), ephemeralStatusDuration)
		return m, nil
	}
}

func (m Model) selected() *scan.Service {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	return &m.filtered[m.cursor]
}
