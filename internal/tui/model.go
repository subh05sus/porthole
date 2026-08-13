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
	"github.com/subh05sus/porthole/internal/tui/theme"
)

// scanTimeout bounds every scan per PRD §8.3 — a slow scan renders partial
// results plus a warning instead of freezing the UI.
const scanTimeout = 3 * time.Second

type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeConfirmKill
	modeConfirmEscalate
	modeHelp
)

// Model is porthole's Bubble Tea Model. Init/Update/View never call the
// scanner or killer directly on this goroutine — every OS interaction is
// dispatched as a tea.Cmd so the UI thread never blocks (PRD §8.3).
type Model struct {
	lister scan.Lister
	killer kill.Killer
	th     theme.Theme

	services []scan.Service
	filtered []scan.Service
	cursor   int

	filterInput textinput.Model

	mode    mode
	pending *scan.Service // row awaiting kill/escalate confirmation
	force   bool          // whether the pending confirmation is for K (force)

	status   string
	scanErr  error
	scanning bool

	width, height int
	quitting      bool
}

// New builds a Model. lister and killer are injected so this whole package
// can be tested without a real OS (see model_test.go).
func New(lister scan.Lister, killer kill.Killer, th theme.Theme) Model {
	ti := textinput.New()
	ti.Placeholder = "filter by port, process, or project"
	ti.Prompt = "/ "

	return Model{
		lister:      lister,
		killer:      killer,
		th:          th,
		filterInput: ti,
		status:      "scanning sockets",
	}
}

func (m Model) Init() tea.Cmd {
	return m.scanCmd()
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

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case scanCompleteMsg:
		m.scanning = false
		m.scanErr = msg.err
		m.services = msg.services
		m.applyFilter()
		if msg.err != nil {
			m.status = fmt.Sprintf("scan failed: %v", msg.err)
		} else {
			m.status = fmt.Sprintf("%d services listening", len(m.services))
		}
		return m, nil

	case killResultMsg:
		return m.handleKillResult(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
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
	switch {
	case msg.err != nil:
		m.status = fmt.Sprintf("port :%d: %v", msg.target.Port, msg.err)
		m.mode = modeNormal
		m.pending = nil
		return m, nil

	case msg.result.Status == kill.StatusKilled || msg.result.Status == kill.StatusAlreadyDead:
		m.status = fmt.Sprintf("terminated %s on :%d", msg.target.Process, msg.target.Port)
		m.mode = modeNormal
		m.pending = nil
		return m, m.scanCmd()

	case msg.result.Status == kill.StatusNeedsEscalation && !msg.force:
		m.mode = modeConfirmEscalate
		m.pending = &msg.target
		m.status = fmt.Sprintf("process ignored SIGTERM, force kill %s on :%d? (y/n)", msg.target.Process, msg.target.Port)
		return m, nil

	default:
		m.status = fmt.Sprintf("port :%d: process ignored kill signal", msg.target.Port)
		m.mode = modeNormal
		m.pending = nil
		return m, nil
	}
}

func (m Model) selected() *scan.Service {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	return &m.filtered[m.cursor]
}
