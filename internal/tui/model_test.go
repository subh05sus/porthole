package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/subh05sus/porthole/internal/config"
	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/kill/killtest"
	"github.com/subh05sus/porthole/internal/proc/proctest"
	"github.com/subh05sus/porthole/internal/restart/restarttest"
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
	return New(lister, killer, &proctest.FakeLookup{}, &restarttest.FakeSpawner{}, config.Config{}, theme.New(true))
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
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
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
	m := New(&scantest.FakeLister{Err: errFake}, &killtest.FakeKiller{}, &proctest.FakeLookup{}, &restarttest.FakeSpawner{}, config.Config{}, theme.New(true))
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

	if len(m.fadingOut) != 1 {
		t.Fatalf("expected one fading row to be set after a successful kill, got %d", len(m.fadingOut))
	}

	// Still within the dissolve window: the row must still be there. When
	// handleTick's rescan cmd is nil, tea.Batch(nil, tickCmd) collapses to
	// tickCmd directly (see bubbletea's compactCmds), so the returned Cmd
	// yields a bare tickMsg rather than a BatchMsg — that's how "no rescan
	// happened" is distinguished here.
	fc.Advance(killDissolveDuration / 2)
	m2, cmd = m.Update(tickMsg(fc.Now()))
	m = m2.(Model)
	if len(m.fadingOut) != 1 {
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
	if len(m.fadingOut) != 0 {
		t.Fatalf("expected fadingOut to clear once the dissolve completes")
	}
	if _, isBatch := cmd().(tea.BatchMsg); !isBatch {
		t.Fatalf("expected a rescan command batched with the tick once the dissolve completes")
	}
}

func TestToggleWatchOnStartsWatchingAndReceivesFirstEvent(t *testing.T) {
	m := newTestModel([]scan.Service{{Port: 3000, PID: 1, Process: "node"}}, nil)
	m = runCmd(t, m, m.Init())

	m2, cmd := m.Update(key("w"))
	m = m2.(Model)
	if !m.watching {
		t.Fatalf("expected watching=true after toggling on")
	}
	if m.watchCancel == nil {
		t.Fatalf("expected watchCancel to be set")
	}
	if cmd == nil {
		t.Fatalf("expected a command to wait for the first watch event")
	}

	m = runCmd(t, m, cmd)
	if !m.watchPulseOn {
		t.Fatalf("expected the pulse to toggle on after the first watch event")
	}
	if len(m.services) != 1 || m.services[0].Port != 3000 {
		t.Fatalf("expected the watch event's services to populate the model, got %+v", m.services)
	}
}

func TestToggleWatchOffClearsState(t *testing.T) {
	m := newTestModel(nil, nil)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("w"))
	m = m2.(Model)
	if !m.watching {
		t.Fatalf("precondition failed: expected watching=true")
	}

	m2, _ = m.Update(key("w"))
	m = m2.(Model)
	if m.watching {
		t.Fatalf("expected watching=false after toggling off")
	}
	if m.watchCancel != nil || m.watchEvents != nil {
		t.Fatalf("expected watch state cleared after toggling off")
	}
}

func TestWatchEventHighlightsAddedAndFadesRemoved(t *testing.T) {
	m := newTestModel([]scan.Service{{Port: 3000, PID: 1}}, nil)
	fc := &fakeClock{now: time.Now()}
	m.clock = fc.Now
	m = runCmd(t, m, m.Init())

	ev := scan.Event{
		Services: []scan.Service{{Port: 3000, PID: 1}, {Port: 8080, PID: 2}},
		Diff: scan.Diff{
			Added:   []scan.Service{{Port: 8080, PID: 2}},
			Removed: []scan.Service{{Port: 9999, PID: 3}},
		},
	}
	m2, _ := m.Update(watchEventMsg{event: ev, ok: true})
	m = m2.(Model)

	if _, ok := m.recentlyAdded[rowKey{port: 8080, pid: 2}]; !ok {
		t.Fatalf("expected the added service to be tracked for highlighting, got %+v", m.recentlyAdded)
	}
	if len(m.fadingOut) != 1 || m.fadingOut[0].service.Port != 9999 {
		t.Fatalf("expected the removed service to be tracked as fading, got %+v", m.fadingOut)
	}
	if m.fadingOut[0].triggersRescan {
		t.Fatalf("a watch-mode removal must not trigger an extra rescan on top of watch's own cadence")
	}
}

func TestWatchEventClosedChannelStopsWatching(t *testing.T) {
	m := newTestModel(nil, nil)
	m = runCmd(t, m, m.Init())
	m2, _ := m.Update(key("w"))
	m = m2.(Model)

	m2, cmd := m.Update(watchEventMsg{ok: false})
	m = m2.(Model)
	if m.watching {
		t.Fatalf("expected watching=false once the event channel closes")
	}
	if cmd != nil {
		t.Fatalf("expected no further wait command once the channel is closed")
	}
}

func TestQuitWhileWatchingDoesNotPanic(t *testing.T) {
	m := newTestModel(nil, nil)
	m = runCmd(t, m, m.Init())
	m2, _ := m.Update(key("w"))
	m = m2.(Model)

	m2, cmd := m.Update(key("q"))
	m = m2.(Model)
	if !m.quitting || cmd == nil {
		t.Fatalf("expected quitting=true and a tea.Quit command")
	}
}

func TestSpaceTogglesMultiSelect(t *testing.T) {
	m := newTestModel([]scan.Service{{Port: 3000, PID: 1, Owned: true}, {Port: 8080, PID: 2, Owned: true}}, nil)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("space"))
	m = m2.(Model)
	if !m.multiSelected[rowKey{port: 3000, pid: 1}] {
		t.Fatalf("expected row 0 selected after space, got %+v", m.multiSelected)
	}

	// Toggling again on the same row deselects it.
	m2, _ = m.Update(key("space"))
	m = m2.(Model)
	if m.multiSelected[rowKey{port: 3000, pid: 1}] {
		t.Fatalf("expected row 0 deselected after a second space")
	}
}

func TestBulkKillConfirmationCoversAllSelectedRows(t *testing.T) {
	killer := &killtest.FakeKiller{}
	services := []scan.Service{
		{Port: 3000, PID: 1, Process: "node", Owned: true},
		{Port: 8080, PID: 2, Process: "go", Owned: true},
		{Port: 9090, PID: 3, Process: "python", Owned: true},
	}
	m := newTestModel(services, killer)
	m = runCmd(t, m, m.Init())

	// Select rows 0 and 1 (cursor starts at 0).
	m2, _ := m.Update(key("space"))
	m = m2.(Model)
	m2, _ = m.Update(key("down"))
	m = m2.(Model)
	m2, _ = m.Update(key("space"))
	m = m2.(Model)

	m2, _ = m.Update(key("k"))
	m = m2.(Model)
	if m.mode != modeConfirmKill {
		t.Fatalf("expected modeConfirmKill, got %v", m.mode)
	}
	if len(m.pendingBulk) != 2 {
		t.Fatalf("expected 2 pending bulk targets, got %d: %+v", len(m.pendingBulk), m.pendingBulk)
	}
}

func TestBulkKillSkipsLockedRowsWithoutBlockingOwnedOnes(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	services := []scan.Service{
		{Port: 3000, PID: 1, Process: "node", Owned: true},
		{Port: 631, PID: 2, Process: "cupsd", Owned: false},
	}
	m := newTestModel(services, killer)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("space"))
	m = m2.(Model)
	m2, _ = m.Update(key("down"))
	m = m2.(Model)
	m2, _ = m.Update(key("space"))
	m = m2.(Model)

	m2, _ = m.Update(key("k"))
	m = m2.(Model)
	if len(m.pendingBulk) != 1 || m.pendingBulk[0].PID != 1 {
		t.Fatalf("expected only the owned row pending, got %+v", m.pendingBulk)
	}
	if !strings.Contains(m.status, "locked rows skipped") {
		t.Fatalf("expected status to mention skipped locked rows, got %q", m.status)
	}
}

func TestBulkKillExecutesAllAndReportsAggregateOutcome(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	services := []scan.Service{
		{Port: 3000, PID: 1, Process: "node", Owned: true},
		{Port: 8080, PID: 2, Process: "go", Owned: true},
	}
	m := newTestModel(services, killer)
	fc := &fakeClock{now: time.Now()}
	m.clock = fc.Now
	m = runCmd(t, m, m.Init())
	fc.Advance(time.Second)

	m2, _ := m.Update(key("space"))
	m = m2.(Model)
	m2, _ = m.Update(key("down"))
	m = m2.(Model)
	m2, _ = m.Update(key("space"))
	m = m2.(Model)
	m2, _ = m.Update(key("k"))
	m = m2.(Model)

	m2, cmd := m.Update(key("y"))
	m = m2.(Model)
	if m.mode != modeKilling || m.killCount != 2 {
		t.Fatalf("expected modeKilling with killCount=2, got mode=%v killCount=%d", m.mode, m.killCount)
	}
	m = runCmd(t, m, cmd)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after bulk kill completes, got %v", m.mode)
	}
	if len(killer.ExecuteCalls) != 2 {
		t.Fatalf("expected 2 Execute calls, got %d", len(killer.ExecuteCalls))
	}
	if len(m.fadingOut) != 2 {
		t.Fatalf("expected both killed rows to be fading, got %d", len(m.fadingOut))
	}
	if len(m.multiSelected) != 0 {
		t.Fatalf("expected selection cleared after bulk kill, got %+v", m.multiSelected)
	}
	if !strings.Contains(m.status, "terminated 2 services") {
		t.Fatalf("expected aggregate success status, got %q", m.status)
	}
}

func TestEnterOpensDetailPaneWithSnapshot(t *testing.T) {
	services := []scan.Service{
		{Port: 3000, PID: 1, Process: "node", Cmdline: "node server.js", CWD: "/app", User: "sub", Owned: true},
		{Port: 3001, PID: 1, Process: "node", Cmdline: "node server.js", CWD: "/app", User: "sub", Owned: true},
	}
	m := newTestModel(services, nil)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("enter"))
	m = m2.(Model)
	if m.mode != modeDetail {
		t.Fatalf("expected modeDetail, got %v", m.mode)
	}
	if m.detailTarget == nil || m.detailTarget.PID != 1 {
		t.Fatalf("expected detailTarget snapshot for pid 1, got %+v", m.detailTarget)
	}

	view := m.View()
	if !strings.Contains(view, "node server.js") || !strings.Contains(view, "/app") {
		t.Fatalf("expected detail view to show cmdline and cwd, got:\n%s", view)
	}
	if !strings.Contains(view, ":3000") || !strings.Contains(view, ":3001") {
		t.Fatalf("expected both sockets sharing pid 1 to be listed, got:\n%s", view)
	}
}

func TestAnyKeyClosesDetailPane(t *testing.T) {
	m := newTestModel([]scan.Service{{Port: 3000, PID: 1, Process: "node", Owned: true}}, nil)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("enter"))
	m = m2.(Model)
	m2, _ = m.Update(key("x"))
	m = m2.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after closing detail pane, got %v", m.mode)
	}
	if m.detailTarget != nil {
		t.Fatalf("expected detailTarget cleared after closing, got %+v", m.detailTarget)
	}
}

func TestEnterQueriesFullSocketListWhenQuerierAvailable(t *testing.T) {
	services := []scan.Service{{Port: 3000, PID: 1, Process: "node", Owned: true}}
	lister := &scantest.FakeQueryingLister{
		FakeLister: scantest.FakeLister{Services: services},
		Sockets: map[int][]scan.Service{
			1: {
				{PID: 1, Proto: scan.ProtoTCP, Port: 3000, Addr: "127.0.0.1"},
				{PID: 1, Proto: scan.ProtoTCP, Port: 54321, Addr: "93.184.216.34"}, // outbound, not in a regular scan
			},
		},
	}
	m := New(lister, &killtest.FakeKiller{}, &proctest.FakeLookup{}, &restarttest.FakeSpawner{}, config.Config{}, theme.New(true))
	m = runCmd(t, m, m.Init())

	m2, cmd := m.Update(key("enter"))
	m = m2.(Model)
	if !m.detailSocketsLoading {
		t.Fatalf("expected detailSocketsLoading while the query is in flight")
	}
	if !strings.Contains(m.View(), "loading") {
		t.Fatalf("expected loading indicator in detail view, got:\n%s", m.View())
	}

	m = runCmd(t, m, cmd)
	if m.detailSocketsLoading {
		t.Fatalf("expected detailSocketsLoading cleared once the query completes")
	}
	view := m.View()
	if !strings.Contains(view, ":54321") {
		t.Fatalf("expected the queried outbound socket (not in a regular scan) to appear, got:\n%s", view)
	}
}

func TestEnterFallsBackToRelatedSocketsOnQueryError(t *testing.T) {
	services := []scan.Service{{Port: 3000, PID: 1, Process: "node", Owned: true}}
	lister := &scantest.FakeQueryingLister{
		FakeLister: scantest.FakeLister{Services: services},
		QueryErr:   errFake,
	}
	m := New(lister, &killtest.FakeKiller{}, &proctest.FakeLookup{}, &restarttest.FakeSpawner{}, config.Config{}, theme.New(true))
	m = runCmd(t, m, m.Init())

	m2, cmd := m.Update(key("enter"))
	m = m2.(Model)
	m = runCmd(t, m, cmd)

	if m.detailSocketsLoading {
		t.Fatalf("expected loading cleared after a failed query")
	}
	if !strings.Contains(m.View(), ":3000") {
		t.Fatalf("expected fallback to the last regular scan's sockets, got:\n%s", m.View())
	}
}

func TestSudoBannerForMajorityUnresolved(t *testing.T) {
	services := []scan.Service{
		{Port: 1, ResolveErr: errFake},
		{Port: 2, ResolveErr: errFake},
		{Port: 3},
	}
	got := sudoBannerFor(services)
	if got == "" {
		t.Fatalf("expected a banner when 2 of 3 services are unresolvable")
	}
	if !strings.Contains(got, "2 of 3") {
		t.Fatalf("got %q, want it to mention 2 of 3", got)
	}
}

func TestSudoBannerForMinorityUnresolvedIsEmpty(t *testing.T) {
	services := []scan.Service{
		{Port: 1, ResolveErr: errFake},
		{Port: 2},
		{Port: 3},
	}
	if got := sudoBannerFor(services); got != "" {
		t.Fatalf("got %q, want empty banner when unresolved is a minority", got)
	}
}

func TestSudoBannerForEmptyServicesIsEmpty(t *testing.T) {
	if got := sudoBannerFor(nil); got != "" {
		t.Fatalf("got %q, want empty banner for zero services (must not divide by zero into 100%%)", got)
	}
}

func TestSudoBannerComputedOnceOnFirstScanOnly(t *testing.T) {
	killer := &killtest.FakeKiller{}
	lister := &scantest.FakeLister{Services: []scan.Service{{Port: 1, ResolveErr: errFake}, {Port: 2, ResolveErr: errFake}}}
	m := New(lister, killer, &proctest.FakeLookup{}, &restarttest.FakeSpawner{}, config.Config{}, theme.New(true))
	m = runCmd(t, m, m.Init())

	if m.sudoBanner == "" {
		t.Fatalf("expected a banner after the first scan found majority-unresolved services")
	}

	// A later scan resolving everything must not retroactively clear the
	// banner — it's a one-time startup check per PRD §8.2, not a live gauge.
	lister.Services = []scan.Service{{Port: 1}, {Port: 2}}
	m2, _ := m.Update(scanCompleteMsg{services: lister.Services})
	m = m2.(Model)
	if m.sudoBanner == "" {
		t.Fatalf("expected the startup banner to persist across later scans")
	}
}

func newRestartTestModel(services []scan.Service, killer *killtest.FakeKiller, lookup *proctest.FakeLookup, spawner *restarttest.FakeSpawner) Model {
	lister := &scantest.FakeLister{Services: services}
	if killer == nil {
		killer = &killtest.FakeKiller{}
	}
	if lookup == nil {
		lookup = &proctest.FakeLookup{}
	}
	if spawner == nil {
		spawner = &restarttest.FakeSpawner{}
	}
	return New(lister, killer, lookup, spawner, config.Config{}, theme.New(true))
}

func TestRestartConfirmFlowSuccess(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	lookup := &proctest.FakeLookup{Info: procInfoWithArgv()}
	spawner := &restarttest.FakeSpawner{}
	m := newRestartTestModel([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer, lookup, spawner)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("R"))
	m = m2.(Model)
	if m.mode != modeConfirmRestart {
		t.Fatalf("expected modeConfirmRestart, got %v", m.mode)
	}

	m2, cmd := m.Update(key("y"))
	m = m2.(Model)
	if m.mode != modeRestarting {
		t.Fatalf("expected modeRestarting, got %v", m.mode)
	}
	if cmd == nil {
		t.Fatalf("expected a restart command to be returned")
	}
	m = runCmd(t, m, cmd)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after successful restart, got %v", m.mode)
	}
	if len(killer.ExecuteCalls) != 1 || killer.ExecuteCalls[0].Target.PID != 111 {
		t.Fatalf("expected Execute called with pid 111, got %+v", killer.ExecuteCalls)
	}
	if len(spawner.Calls) != 1 {
		t.Fatalf("expected Spawn called once, got %d", len(spawner.Calls))
	}
	if !strings.Contains(m.status, "restarted node on :3000") {
		t.Fatalf("got status %q", m.status)
	}
}

func TestRestartConfirmDeclineDoesNotCallExecuteOrSpawn(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	spawner := &restarttest.FakeSpawner{}
	m := newRestartTestModel([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer, nil, spawner)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("R"))
	m = m2.(Model)
	m2, _ = m.Update(key("n"))
	m = m2.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal, got %v", m.mode)
	}
	if len(killer.ExecuteCalls) != 0 || len(spawner.Calls) != 0 {
		t.Fatalf("declined restart must not call Execute or Spawn")
	}
}

func TestRestartOnUnownedRowRefusesWithoutConfirming(t *testing.T) {
	killer := &killtest.FakeKiller{}
	m := newRestartTestModel([]scan.Service{{Port: 631, Process: "cupsd", Owned: false}}, killer, nil, nil)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("R"))
	m = m2.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal, got %v", m.mode)
	}
}

func TestRestartOnContainerBackedRowRefusesWithoutConfirming(t *testing.T) {
	killer := &killtest.FakeKiller{}
	services := []scan.Service{{Port: 5434, Process: "com.docker.backend", Owned: true, Container: "db", ContainerID: "abc123"}}
	m := newRestartTestModel(services, killer, nil, nil)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("R"))
	m = m2.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal (refused without confirming), got %v", m.mode)
	}
	if !strings.Contains(m.status, "docker restart db") {
		t.Fatalf("expected a docker-restart pointer in the status, got %q", m.status)
	}
}

func TestRestartCaptureFailureShowsError(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	lookup := &proctest.FakeLookup{Err: errFake}
	spawner := &restarttest.FakeSpawner{}
	m := newRestartTestModel([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer, lookup, spawner)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("R"))
	m = m2.(Model)
	m2, cmd := m.Update(key("y"))
	m = m2.(Model)
	m = runCmd(t, m, cmd)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after a failed capture, got %v", m.mode)
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("must never attempt to kill if capture failed, got %d Execute calls", len(killer.ExecuteCalls))
	}
	if !strings.Contains(m.status, "restart failed") {
		t.Fatalf("got status %q", m.status)
	}
}

func procInfoWithArgv() proctest.Info {
	return proctest.Info{Cmdline: "node server.js", Argv: []string{"node", "server.js"}, CWD: "/app"}
}

func newProtectedTestModel(services []scan.Service, killer *killtest.FakeKiller, cfg config.Config) Model {
	lister := &scantest.FakeLister{Services: services}
	if killer == nil {
		killer = &killtest.FakeKiller{}
	}
	return New(lister, killer, &proctest.FakeLookup{}, &restarttest.FakeSpawner{}, cfg, theme.New(true))
}

func typeString(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m2, _ := m.Update(key(string(r)))
		m = m2.(Model)
	}
	return m
}

func TestKillProtectedPortRequiresTypedPortConfirmation(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	cfg := config.Config{Protected: []config.ProtectedPort{{Port: 5432, Reason: "prod db"}}}
	m := newProtectedTestModel([]scan.Service{{Port: 5432, PID: 1, Process: "postgres", Owned: true}}, killer, cfg)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("k"))
	m = m2.(Model)
	if m.mode != modeConfirmProtected {
		t.Fatalf("expected modeConfirmProtected, got %v", m.mode)
	}
	if !strings.Contains(m.status, "prod db") {
		t.Fatalf("expected status to mention the protection reason, got %q", m.status)
	}

	m = typeString(t, m, "5432")
	m2, cmd := m.Update(key("enter"))
	m = m2.(Model)
	if m.mode != modeKilling {
		t.Fatalf("expected modeKilling after correct typed confirmation, got %v", m.mode)
	}
	m = runCmd(t, m, cmd)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after the kill completes, got %v", m.mode)
	}
	if len(killer.ExecuteCalls) != 1 {
		t.Fatalf("expected Execute called once, got %d", len(killer.ExecuteCalls))
	}
}

func TestKillProtectedPortWrongInputCancels(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	cfg := config.Config{Protected: []config.ProtectedPort{{Port: 5432}}}
	m := newProtectedTestModel([]scan.Service{{Port: 5432, PID: 1, Process: "postgres", Owned: true}}, killer, cfg)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("k"))
	m = m2.(Model)
	m = typeString(t, m, "9999")
	m2, _ = m.Update(key("enter"))
	m = m2.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after a wrong typed confirmation, got %v", m.mode)
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("wrong confirmation must not call Execute, got %d calls", len(killer.ExecuteCalls))
	}
}

func TestKillProtectedPortEscCancels(t *testing.T) {
	killer := &killtest.FakeKiller{}
	cfg := config.Config{Protected: []config.ProtectedPort{{Port: 5432}}}
	m := newProtectedTestModel([]scan.Service{{Port: 5432, PID: 1, Process: "postgres", Owned: true}}, killer, cfg)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("k"))
	m = m2.(Model)
	m2, _ = m.Update(key("esc"))
	m = m2.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal after esc, got %v", m.mode)
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("esc must not call Execute, got %d calls", len(killer.ExecuteCalls))
	}
}

func TestBulkKillRefusesWhenSelectionIncludesProtectedPort(t *testing.T) {
	killer := &killtest.FakeKiller{}
	cfg := config.Config{Protected: []config.ProtectedPort{{Port: 5432}}}
	services := []scan.Service{
		{Port: 3000, PID: 1, Process: "node", Owned: true},
		{Port: 5432, PID: 2, Process: "postgres", Owned: true},
	}
	m := newProtectedTestModel(services, killer, cfg)
	m = runCmd(t, m, m.Init())

	m2, _ := m.Update(key("space"))
	m = m2.(Model)
	m2, _ = m.Update(key("down"))
	m = m2.(Model)
	m2, _ = m.Update(key("space"))
	m = m2.(Model)

	m2, _ = m.Update(key("k"))
	m = m2.(Model)

	if m.mode != modeNormal {
		t.Fatalf("expected modeNormal (refused, not entering any confirm mode), got %v", m.mode)
	}
	if !strings.Contains(m.status, "individually") {
		t.Fatalf("expected a message explaining protected ports need individual confirmation, got %q", m.status)
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("must never kill anything from a refused bulk selection, got %d calls", len(killer.ExecuteCalls))
	}
}

func TestProtectedRowRendersShieldGlyph(t *testing.T) {
	cfg := config.Config{Protected: []config.ProtectedPort{{Port: 5432}}}
	m := newProtectedTestModel([]scan.Service{{Port: 5432, PID: 1, Process: "postgres", Owned: true}}, nil, cfg)
	m = runCmd(t, m, m.Init())
	fc := &fakeClock{now: time.Now()}
	m.clock = fc.Now
	fc.Advance(time.Second)

	view := m.renderTable()
	if !strings.Contains(view, "🛡") {
		t.Fatalf("expected the protected row to render a shield glyph, got:\n%s", view)
	}
}

func TestContainerBackedRowRendersWhaleGlyph(t *testing.T) {
	services := []scan.Service{{Port: 5434, PID: 1, Process: "com.docker.backend", Owned: true, Container: "db", ContainerID: "abc123"}}
	m := newTestModel(services, nil)
	m = runCmd(t, m, m.Init())
	fc := &fakeClock{now: time.Now()}
	m.clock = fc.Now
	fc.Advance(time.Second)

	view := m.renderTable()
	if !strings.Contains(view, "🐳") {
		t.Fatalf("expected the container-backed row to render a whale glyph, got:\n%s", view)
	}
}
