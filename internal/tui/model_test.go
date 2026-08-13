package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/kill/killtest"
	"github.com/subh05sus/porthole/internal/scan"
	"github.com/subh05sus/porthole/internal/scan/scantest"
	"github.com/subh05sus/porthole/internal/tui/theme"
)

func newTestModel(services []scan.Service, killer *killtest.FakeKiller) Model {
	lister := &scantest.FakeLister{Services: services}
	if killer == nil {
		killer = &killtest.FakeKiller{}
	}
	return New(lister, killer, theme.New(true))
}

// runCmd executes a tea.Cmd synchronously and feeds its resulting Msg back
// into Update, the same pattern bubbletea's runtime uses — but driven
// directly, with no real terminal, per the plan's testing strategy.
func runCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	newModel, _ := m.Update(msg)
	return newModel.(Model)
}

func key(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestInitTriggersScanAndPopulatesServices(t *testing.T) {
	m := newTestModel([]scan.Service{{Port: 3000, Process: "node", Owned: true}}, nil)

	m = runCmd(t, m, m.Init())

	if len(m.services) != 1 || m.services[0].Port != 3000 {
		t.Fatalf("got services %+v", m.services)
	}
	if m.status != "1 services listening" {
		t.Fatalf("got status %q", m.status)
	}
}

func TestScanErrorSetsStatus(t *testing.T) {
	m := New(&scantest.FakeLister{Err: errFake}, &killtest.FakeKiller{}, theme.New(true))
	m = runCmd(t, m, m.Init())

	if m.scanErr == nil {
		t.Fatalf("expected scanErr to be set")
	}
}

func TestNavigationMovesCursorWithinBounds(t *testing.T) {
	m := newTestModel([]scan.Service{{Port: 1}, {Port: 2}, {Port: 3}}, nil)
	m = runCmd(t, m, m.Init())

	if m.cursor != 0 {
		t.Fatalf("expected cursor 0, got %d", m.cursor)
	}

	m2, _ := m.Update(key("down"))
	m = m2.(Model)
	if m.cursor != 1 {
		t.Fatalf("expected cursor 1 after down, got %d", m.cursor)
	}

	m2, _ = m.Update(key("j"))
	m = m2.(Model)
	if m.cursor != 2 {
		t.Fatalf("expected cursor 2 after j, got %d", m.cursor)
	}

	// Already at the last row; down must not overflow.
	m2, _ = m.Update(key("down"))
	m = m2.(Model)
	if m.cursor != 2 {
		t.Fatalf("expected cursor to stay at 2, got %d", m.cursor)
	}

	m2, _ = m.Update(key("up"))
	m = m2.(Model)
	if m.cursor != 1 {
		t.Fatalf("expected cursor 1 after up, got %d", m.cursor)
	}
}

func TestFilterNarrowsResultsAndClampsCursor(t *testing.T) {
	m := newTestModel([]scan.Service{
		{Port: 3000, Process: "node", Project: "zapmail-web"},
		{Port: 5432, Process: "postgres", Project: "system"},
	}, nil)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("/"))
	m = m2.(Model)
	if m.mode != modeFilter {
		t.Fatalf("expected modeFilter, got %v", m.mode)
	}

	for _, r := range "postgres" {
		m2, _ = m.Update(key(string(r)))
		m = m2.(Model)
	}
	if len(m.filtered) != 1 || m.filtered[0].Process != "postgres" {
		t.Fatalf("expected filter to narrow to postgres, got %+v", m.filtered)
	}

	m2, _ = m.Update(key("enter"))
	m = m2.(Model)
	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after enter, got %v", m.mode)
	}
}

func TestFilterEscClearsQuery(t *testing.T) {
	m := newTestModel([]scan.Service{{Port: 3000, Process: "node"}, {Port: 5432, Process: "postgres"}}, nil)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("/"))
	m = m2.(Model)
	m2, _ = m.Update(key("p"))
	m = m2.(Model)
	m2, _ = m.Update(key("esc"))
	m = m2.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after esc, got %v", m.mode)
	}
	if len(m.filtered) != 2 {
		t.Fatalf("expected filter cleared (2 results), got %d", len(m.filtered))
	}
}

func TestKillOnUnownedRowRefusesWithoutConfirming(t *testing.T) {
	killer := &killtest.FakeKiller{}
	m := newTestModel([]scan.Service{{Port: 631, Process: "cupsd", Owned: false}}, killer)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("k"))
	m = m2.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal, got %v", m.mode)
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("unowned row must never reach Execute, got %d calls", len(killer.ExecuteCalls))
	}
}

func TestKillConfirmFlowSuccessTriggersRescan(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	m := newTestModel([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("k"))
	m = m2.(Model)
	if m.mode != modeConfirmKill {
		t.Fatalf("expected modeConfirmKill, got %v", m.mode)
	}

	m2, cmd := m.Update(key("y"))
	m = m2.(Model)
	if cmd == nil {
		t.Fatalf("expected a kill command to be returned")
	}
	m = runCmd(t, m, cmd)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after successful kill, got %v", m.mode)
	}
	if len(killer.ExecuteCalls) != 1 || killer.ExecuteCalls[0].Target.PID != 111 {
		t.Fatalf("expected Execute called with PID 111, got %+v", killer.ExecuteCalls)
	}
}

func TestKillConfirmDeclineDoesNotCallExecute(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	m := newTestModel([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("k"))
	m = m2.(Model)
	m2, _ = m.Update(key("n"))
	m = m2.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal, got %v", m.mode)
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("declined confirmation must not call Execute, got %d calls", len(killer.ExecuteCalls))
	}
}

func TestKillNeedsEscalationPromptsForce(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusNeedsEscalation}}
	m := newTestModel([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("k"))
	m = m2.(Model)
	m2, cmd := m.Update(key("y"))
	m = m2.(Model)
	m = runCmd(t, m, cmd)

	if m.mode != modeConfirmEscalate {
		t.Fatalf("expected modeConfirmEscalate, got %v", m.mode)
	}

	killer.EscalateResult = kill.Result{Status: kill.StatusKilled}
	m2, cmd = m.Update(key("y"))
	m = m2.(Model)
	m = runCmd(t, m, cmd)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after escalated kill succeeds, got %v", m.mode)
	}
	if len(killer.EscalateCalls) != 1 {
		t.Fatalf("expected Escalate called once, got %d", len(killer.EscalateCalls))
	}
}

func TestForceKillSkipsEscalationPromptOnFailure(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusNeedsEscalation}}
	m := newTestModel([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("K"))
	m = m2.(Model)
	m2, cmd := m.Update(key("y"))
	m = m2.(Model)
	m = runCmd(t, m, cmd)

	if m.mode != modeNormal {
		t.Fatalf("a failed force-kill has nothing further to escalate to, expected modeNormal, got %v", m.mode)
	}
}

func TestHelpTogglesAndAnyKeyCloses(t *testing.T) {
	m := newTestModel(nil, nil)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("?"))
	m = m2.(Model)
	if m.mode != modeHelp {
		t.Fatalf("expected modeHelp, got %v", m.mode)
	}

	m2, _ = m.Update(key("x"))
	m = m2.(Model)
	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after closing help, got %v", m.mode)
	}
}

func TestQuitSetsQuittingAndReturnsQuitCmd(t *testing.T) {
	m := newTestModel(nil, nil)
	m = runCmd(t, m, m.Init())

	m2, cmd := m.Update(key("q"))
	m = m2.(Model)
	if !m.quitting {
		t.Fatalf("expected quitting=true")
	}
	if cmd == nil {
		t.Fatalf("expected tea.Quit command")
	}
}

var errFake = fakeErr("boom")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }
