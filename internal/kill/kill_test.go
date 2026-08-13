package kill

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeSignaler is a scriptable Signaler for testing the ladder in isolation
// from any real OS. terminateEffect/killEffect let a test simulate a
// process that dies on signal, or one that ignores it entirely.
type fakeSignaler struct {
	startTime uint64
	alive     bool

	terminateEffect func(f *fakeSignaler)
	killEffect      func(f *fakeSignaler)

	terminateCalls  int
	killCalls       int
	stillAliveCalls int
}

func (f *fakeSignaler) StillAlive(pid int) (uint64, bool, error) {
	f.stillAliveCalls++
	return f.startTime, f.alive, nil
}

func (f *fakeSignaler) Terminate(pid int) error {
	f.terminateCalls++
	if f.terminateEffect != nil {
		f.terminateEffect(f)
	}
	return nil
}

func (f *fakeSignaler) Kill(pid int) error {
	f.killCalls++
	if f.killEffect != nil {
		f.killEffect(f)
	}
	return nil
}

func diesOnSignal(f *fakeSignaler) { f.alive = false }

func fastPollOptions() Options {
	return Options{PollInterval: time.Millisecond, PollTimeout: 20 * time.Millisecond}
}

func TestExecuteSuccessGracefulKill(t *testing.T) {
	sig := &fakeSignaler{startTime: 42, alive: true, terminateEffect: diesOnSignal}
	l := &Ladder{Signaler: sig}

	res, err := l.Execute(context.Background(), Target{PID: 1, StartTime: 42, Owned: true}, fastPollOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusKilled {
		t.Fatalf("got status %v, want StatusKilled", res.Status)
	}
	if sig.terminateCalls != 1 || sig.killCalls != 0 {
		t.Fatalf("terminateCalls=%d killCalls=%d, want 1/0", sig.terminateCalls, sig.killCalls)
	}
}

func TestExecuteAlreadyDead(t *testing.T) {
	sig := &fakeSignaler{startTime: 42, alive: false}
	l := &Ladder{Signaler: sig}

	res, err := l.Execute(context.Background(), Target{PID: 1, StartTime: 42, Owned: true}, fastPollOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusAlreadyDead {
		t.Fatalf("got status %v, want StatusAlreadyDead", res.Status)
	}
	if sig.terminateCalls != 0 {
		t.Fatalf("terminateCalls=%d, want 0 (dead process must never be signaled)", sig.terminateCalls)
	}
}

func TestExecutePIDReusedAborts(t *testing.T) {
	sig := &fakeSignaler{startTime: 999, alive: true}
	l := &Ladder{Signaler: sig}

	_, err := l.Execute(context.Background(), Target{PID: 1, StartTime: 42, Owned: true}, fastPollOptions())
	if !errors.Is(err, ErrPIDReused) {
		t.Fatalf("got %v, want ErrPIDReused", err)
	}
	if sig.terminateCalls != 0 {
		t.Fatalf("terminateCalls=%d, want 0 (recycled PID must never be signaled)", sig.terminateCalls)
	}
}

func TestExecuteNotOwnedAborts(t *testing.T) {
	sig := &fakeSignaler{startTime: 42, alive: true}
	l := &Ladder{Signaler: sig}

	_, err := l.Execute(context.Background(), Target{PID: 1, StartTime: 42, Owned: false}, fastPollOptions())
	if !errors.Is(err, ErrNotOwned) {
		t.Fatalf("got %v, want ErrNotOwned", err)
	}
	if sig.stillAliveCalls != 0 && sig.terminateCalls != 0 {
		t.Fatalf("ownership must be checked before any syscall")
	}
}

func TestExecuteIgnoresSIGTERMNeedsEscalation(t *testing.T) {
	sig := &fakeSignaler{startTime: 42, alive: true} // terminateEffect nil: ignores SIGTERM
	l := &Ladder{Signaler: sig}

	res, err := l.Execute(context.Background(), Target{PID: 1, StartTime: 42, Owned: true}, fastPollOptions())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusNeedsEscalation {
		t.Fatalf("got status %v, want StatusNeedsEscalation", res.Status)
	}
	if sig.killCalls != 0 {
		t.Fatalf("killCalls=%d, want 0 (no auto-escalation requested)", sig.killCalls)
	}
}

func TestExecuteAutoEscalateForcesKillAfterIgnoredSIGTERM(t *testing.T) {
	sig := &fakeSignaler{startTime: 42, alive: true, killEffect: diesOnSignal}
	l := &Ladder{EscalatePollInterval: time.Millisecond, EscalatePollTimeout: 20 * time.Millisecond, Signaler: sig}

	opts := fastPollOptions()
	opts.AutoEscalate = true
	res, err := l.Execute(context.Background(), Target{PID: 1, StartTime: 42, Owned: true}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusKilled {
		t.Fatalf("got status %v, want StatusKilled", res.Status)
	}
	if sig.terminateCalls != 1 || sig.killCalls != 1 {
		t.Fatalf("terminateCalls=%d killCalls=%d, want 1/1", sig.terminateCalls, sig.killCalls)
	}
}

func TestEscalateReVerifiesStartTimeAndAbortsOnReuse(t *testing.T) {
	sig := &fakeSignaler{startTime: 999, alive: true}
	l := &Ladder{Signaler: sig}

	_, err := l.Escalate(context.Background(), Target{PID: 1, StartTime: 42, Owned: true})
	if !errors.Is(err, ErrPIDReused) {
		t.Fatalf("got %v, want ErrPIDReused", err)
	}
	if sig.killCalls != 0 {
		t.Fatalf("killCalls=%d, want 0 (recycled PID must never be force-killed)", sig.killCalls)
	}
}

func TestExecuteForceSkipsGracefulSignal(t *testing.T) {
	sig := &fakeSignaler{startTime: 42, alive: true, killEffect: diesOnSignal}
	l := &Ladder{Signaler: sig}

	opts := fastPollOptions()
	opts.Force = true
	res, err := l.Execute(context.Background(), Target{PID: 1, StartTime: 42, Owned: true}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != StatusKilled {
		t.Fatalf("got status %v, want StatusKilled", res.Status)
	}
	if sig.terminateCalls != 0 || sig.killCalls != 1 {
		t.Fatalf("terminateCalls=%d killCalls=%d, want 0/1", sig.terminateCalls, sig.killCalls)
	}
}

func TestExecuteRespectsContextCancellation(t *testing.T) {
	sig := &fakeSignaler{startTime: 42, alive: true}
	l := &Ladder{Signaler: sig}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := Options{PollInterval: time.Second, PollTimeout: time.Minute}
	_, err := l.Execute(ctx, Target{PID: 1, StartTime: 42, Owned: true}, opts)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}
