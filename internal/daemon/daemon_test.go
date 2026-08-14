package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/subh05sus/porthole/internal/config"
	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/kill/killtest"
	"github.com/subh05sus/porthole/internal/scan"
)

// seqLister returns a different scripted result on each successive
// List() call — needed here because a single Poll() cycle can call
// List() more than once (the initial scan, then one re-verify call per
// match), unlike most other tests in this codebase that only need one
// scripted result per call.
type seqLister struct {
	calls   int
	results [][]scan.Service
	err     error
}

func (s *seqLister) List(ctx context.Context) ([]scan.Service, error) {
	if s.err != nil {
		return nil, s.err
	}
	i := s.calls
	if i >= len(s.results) {
		i = len(s.results) - 1
	}
	s.calls++
	return s.results[i], nil
}

var allowNodeOn3000 = config.Config{AutoKill: config.AutoKill{
	Enabled: true,
	Allow:   []config.AutoKillEntry{{Port: 3000, Process: "node"}},
}}

var matchSvc = scan.Service{Port: 3000, PID: 111, StartTime: 42, Process: "node", Owned: true}

func TestPollDoesNothingWhenDisabled(t *testing.T) {
	lister := &seqLister{results: [][]scan.Service{{matchSvc}}}
	killer := &killtest.FakeKiller{}
	d := &Daemon{Lister: lister, Killer: killer, Config: config.Config{}, Live: true}

	ev := d.Poll(context.Background())
	if len(ev.Actions) != 0 {
		t.Fatalf("expected zero actions when disabled, got %+v", ev.Actions)
	}
	if lister.calls != 0 {
		t.Fatalf("expected not to even scan when disabled, got %d List calls", lister.calls)
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("must never kill anything when disabled")
	}
}

func TestPollDryRunReportsMatchWithoutKilling(t *testing.T) {
	lister := &seqLister{results: [][]scan.Service{{matchSvc}, {matchSvc}}}
	killer := &killtest.FakeKiller{}
	d := &Daemon{Lister: lister, Killer: killer, Config: allowNodeOn3000, Live: false}

	ev := d.Poll(context.Background())
	if len(ev.Actions) != 1 {
		t.Fatalf("got %d actions, want 1: %+v", len(ev.Actions), ev.Actions)
	}
	a := ev.Actions[0]
	if a.Live || a.Skipped {
		t.Fatalf("expected a dry-run (non-live, non-skipped) match, got %+v", a)
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("dry-run must never call Killer.Execute, got %d calls", len(killer.ExecuteCalls))
	}
}

func TestPollLiveKillsMatchingService(t *testing.T) {
	lister := &seqLister{results: [][]scan.Service{{matchSvc}, {matchSvc}}}
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	d := &Daemon{Lister: lister, Killer: killer, Config: allowNodeOn3000, Live: true}

	ev := d.Poll(context.Background())
	if len(ev.Actions) != 1 || !ev.Actions[0].Live {
		t.Fatalf("expected one live action, got %+v", ev.Actions)
	}
	if len(killer.ExecuteCalls) != 1 {
		t.Fatalf("expected exactly one Execute call, got %d", len(killer.ExecuteCalls))
	}
	got := killer.ExecuteCalls[0].Target
	want := kill.Target{PID: matchSvc.PID, StartTime: matchSvc.StartTime, Owned: matchSvc.Owned}
	if got != want {
		t.Fatalf("got target %+v, want %+v", got, want)
	}
	if ev.Actions[0].Result.Status != kill.StatusKilled {
		t.Fatalf("expected the Killer's result to be reported, got %+v", ev.Actions[0])
	}
}

func TestPollSkipsServicesNotOnTheAllowList(t *testing.T) {
	other := scan.Service{Port: 3000, PID: 111, Process: "python", Owned: true} // wrong process name
	lister := &seqLister{results: [][]scan.Service{{other}}}
	killer := &killtest.FakeKiller{}
	d := &Daemon{Lister: lister, Killer: killer, Config: allowNodeOn3000, Live: true}

	ev := d.Poll(context.Background())
	if len(ev.Actions) != 0 {
		t.Fatalf("expected zero actions for a non-allow-listed process, got %+v", ev.Actions)
	}
}

func TestPollSkipsDuringCooldownAfterAMatch(t *testing.T) {
	lister := &seqLister{results: [][]scan.Service{{matchSvc}, {matchSvc}, {matchSvc}}}
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	d := &Daemon{Lister: lister, Killer: killer, Config: allowNodeOn3000, Live: true}

	first := d.Poll(context.Background())
	if len(first.Actions) != 1 || first.Actions[0].Skipped {
		t.Fatalf("expected the first poll to act, got %+v", first.Actions)
	}

	second := d.Poll(context.Background())
	if len(second.Actions) != 1 || !second.Actions[0].Skipped || second.Actions[0].SkipReason != "cooldown" {
		t.Fatalf("expected the second poll to skip on cooldown, got %+v", second.Actions)
	}
	if len(killer.ExecuteCalls) != 1 {
		t.Fatalf("expected exactly one real kill despite two polls (rate-limited), got %d", len(killer.ExecuteCalls))
	}
}

func TestPollSkipsOnReverifyMismatch(t *testing.T) {
	// The initial scan finds the match, but by the time the re-verify
	// scan runs, the process is gone (crashed/exited on its own) —
	// the daemon must not act on stale information.
	lister := &seqLister{results: [][]scan.Service{{matchSvc}, {}}}
	killer := &killtest.FakeKiller{}
	d := &Daemon{Lister: lister, Killer: killer, Config: allowNodeOn3000, Live: true}

	ev := d.Poll(context.Background())
	if len(ev.Actions) != 1 || !ev.Actions[0].Skipped || ev.Actions[0].SkipReason != "reverify mismatch" {
		t.Fatalf("expected a reverify-mismatch skip, got %+v", ev.Actions)
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("must never kill on a reverify mismatch, got %d Execute calls", len(killer.ExecuteCalls))
	}
}

func TestPollSkipsOnReverifyMismatchWhenPIDChanged(t *testing.T) {
	// A different process now owns the same port (PID reuse or a fast
	// crash-restart cycle) — same identity check that protects the kill
	// ladder itself, applied one layer up at the allow-list-match level.
	reused := scan.Service{Port: 3000, PID: 999, StartTime: 1, Process: "node", Owned: true}
	lister := &seqLister{results: [][]scan.Service{{matchSvc}, {reused}}}
	killer := &killtest.FakeKiller{}
	d := &Daemon{Lister: lister, Killer: killer, Config: allowNodeOn3000, Live: true}

	ev := d.Poll(context.Background())
	if len(ev.Actions) != 1 || !ev.Actions[0].Skipped || ev.Actions[0].SkipReason != "reverify mismatch" {
		t.Fatalf("expected a reverify-mismatch skip on PID change, got %+v", ev.Actions)
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("must never kill when the re-verified process identity differs, got %d Execute calls", len(killer.ExecuteCalls))
	}
}

func TestPollReportsReverifyScanFailure(t *testing.T) {
	// Force the second List() call (re-verify) to fail by swapping in an
	// error after the first call succeeds.
	failing := &twoStageLister{first: []scan.Service{matchSvc}, secondErr: errors.New("scan timeout")}
	killer := &killtest.FakeKiller{}
	d := &Daemon{Lister: failing, Killer: killer, Config: allowNodeOn3000, Live: true}

	ev := d.Poll(context.Background())
	if len(ev.Actions) != 1 || !ev.Actions[0].Skipped {
		t.Fatalf("expected a skipped action on reverify scan failure, got %+v", ev.Actions)
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("must never kill when the reverify scan itself fails, got %d Execute calls", len(killer.ExecuteCalls))
	}
}

type twoStageLister struct {
	calls     int
	first     []scan.Service
	secondErr error
}

func (l *twoStageLister) List(ctx context.Context) ([]scan.Service, error) {
	l.calls++
	if l.calls == 1 {
		return l.first, nil
	}
	return nil, l.secondErr
}

func TestPollReportsInitialScanError(t *testing.T) {
	lister := &seqLister{err: errors.New("boom")}
	killer := &killtest.FakeKiller{}
	d := &Daemon{Lister: lister, Killer: killer, Config: allowNodeOn3000, Live: true}

	ev := d.Poll(context.Background())
	if ev.ScanErr == nil {
		t.Fatalf("expected ScanErr to be set")
	}
	if len(ev.Actions) != 0 {
		t.Fatalf("expected no actions when the initial scan fails, got %+v", ev.Actions)
	}
}

func TestRunSendsEventPerTick(t *testing.T) {
	lister := &seqLister{results: [][]scan.Service{{matchSvc}, {matchSvc}, {matchSvc}, {matchSvc}}}
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	d := &Daemon{Lister: lister, Killer: killer, Config: allowNodeOn3000, Live: false}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)

	events := d.Run(ctx, time.Second, ticks)

	first := <-events
	if len(first.Actions) != 1 {
		t.Fatalf("expected the first (immediate) poll to produce an action, got %+v", first)
	}

	ticks <- time.Now()
	second := <-events
	if len(second.Actions) != 1 {
		t.Fatalf("expected a second poll after a tick, got %+v", second)
	}

	cancel()
}
