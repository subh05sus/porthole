package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/kill/killtest"
	"github.com/subh05sus/porthole/internal/scan"
	"github.com/subh05sus/porthole/internal/scan/scantest"
	"github.com/subh05sus/porthole/internal/tui/theme"
)

// fakeClock gives tests deterministic control over the elapsed time every
// animation calculation in the model reads through m.clock, instead of
// racing real wall-clock time.
type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestModel(services []scan.Service, killer *killtest.FakeKiller) Model {
	lister := &scantest.FakeLister{Services: services}
	if killer == nil {
		killer = &killtest.FakeKiller{}
	}
	return New(lister, killer, theme.New(true))
}

// runCmd executes a tea.Cmd synchronously and feeds its resulting Msg back
// into Update, the same pattern bubbletea's runtime uses — but driven
// directly, with no real terminal, per the plan's testing strategy. A
// tea.Batch (used by Init to run the scan and the tick loop together) is
// unpacked one level deep and each sub-command applied in turn; any *new*
// command a sub-update produces (e.g. tickCmd rescheduling itself) is
// deliberately left unrun, or every test using Init() would recurse into
// the tick loop forever.
func runCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			newModel, _ := m.Update(c())
			m = newModel.(Model)
		}
		return m
	}
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
	fc := &fakeClock{now: time.Now()}
	m.clock = fc.Now

	m = runCmd(t, m, m.Init())

	if len(m.services) != 1 || m.services[0].Port != 3000 {
		t.Fatalf("got services %+v", m.services)
	}
	// Immediately after the scan, still within the "resolving" phase of the
	// progressive status line (PRD §5.3) — the frozen fake clock hasn't
	// crossed anim.RevealCap yet.
	if !strings.Contains(m.status, "resolving") {
		t.Fatalf("got status %q, want a resolving-phase message", m.status)
	}

	// Advance well past the reveal cap and re-tick: status must settle to
	// the final steady-state message.
	fc.Advance(time.Second)
	m2, _ := m.Update(tickMsg(fc.Now()))
	m = m2.(Model)
	if m.status != "1 services listening" {
		t.Fatalf("got status %q, want final steady-state message", m.status)
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

func TestStreamingRevealShowsFewerRowsBeforeStaggerElapses(t *testing.T) {
	m := newTestModel([]scan.Service{{Port: 1}, {Port: 2}, {Port: 3}}, nil)
	fc := &fakeClock{now: time.Now()}
	m.clock = fc.Now

	m = runCmd(t, m, m.Init())

	// Frozen at revealStart: only row 0 should be due (header + 1 row).
	view := m.renderTable()
	if strings.Count(view, "\n") != 1 {
		t.Fatalf("expected exactly one data row rendered immediately after scan, got view:\n%s", view)
	}

	// Advance past the full reveal cascade; all three rows must now render.
	fc.Advance(500 * time.Millisecond)
	view = m.renderTable()
	lines := strings.Split(view, "\n")
	if len(lines) != 4 { // header + 3 rows
		t.Fatalf("expected header + 3 rows once reveal completes, got %d lines:\n%s", len(lines), view)
	}
}

func TestDyingRowPersistsUntilFadeCompletesThenRescans(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	services := []scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}
	m := newTestModel(services, killer)
	fc := &fakeClock{now: time.Now()}
	m.clock = fc.Now

	m = runCmd(t, m, m.Init())
	fc.Advance(time.Second) // past the reveal cascade so all rows show

	m2, _ := m.Update(key("k"))
	m = m2.(Model)
	m2, cmd := m.Update(key("y"))
	m = m2.(Model)
	m = runCmd(t, m, cmd) // applies killResultMsg

	if m.dying == nil {
		t.Fatalf("expected a dying row to be set after a successful kill")
	}

	// Still within the dissolve window: the row must still be there. When
	// handleTick's rescan cmd is nil, tea.Batch(nil, tickCmd) collapses to
	// tickCmd directly (see bubbletea's compactCmds), so the returned Cmd
	// yields a bare tickMsg rather than a BatchMsg — that's how "no rescan
	// happened" is distinguished here.
	fc.Advance(killDissolveDuration / 2)
	m2, cmd = m.Update(tickMsg(fc.Now()))
	m = m2.(Model)
	if m.dying == nil {
		t.Fatalf("row must still be dying mid-dissolve")
	}
	if _, isBatch := cmd().(tea.BatchMsg); isBatch {
		t.Fatalf("must not rescan before the dissolve finishes")
	}

	// Past the dissolve window: it must clear and trigger a rescan, which
	// shows up as a BatchMsg{rescanCmd, tickCmd} instead of a bare tickMsg.
	fc.Advance(killDissolveDuration)
	m2, cmd = m.Update(tickMsg(fc.Now()))
	m = m2.(Model)
	if m.dying != nil {
		t.Fatalf("expected dying to clear once the dissolve completes")
	}
	if _, isBatch := cmd().(tea.BatchMsg); !isBatch {
		t.Fatalf("expected a rescan command batched with the tick once the dissolve completes")
	}
}
