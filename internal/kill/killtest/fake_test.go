package killtest

import (
	"context"
	"errors"
	"testing"

	"github.com/subh05sus/porthole/internal/kill"
)

func TestFakeKillerRecordsExecuteCallsAndReturnsScriptedResult(t *testing.T) {
	f := &FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	target := kill.Target{PID: 1, StartTime: 42, Owned: true}
	opts := kill.Options{Force: true}

	res, err := f.Execute(context.Background(), target, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != kill.StatusKilled {
		t.Fatalf("got %v, want StatusKilled", res.Status)
	}
	if len(f.ExecuteCalls) != 1 || f.ExecuteCalls[0].Target != target || f.ExecuteCalls[0].Opts != opts {
		t.Fatalf("ExecuteCalls not recorded correctly: %+v", f.ExecuteCalls)
	}
}

func TestFakeKillerReturnsScriptedExecuteError(t *testing.T) {
	f := &FakeKiller{ExecuteErr: kill.ErrPIDReused}

	_, err := f.Execute(context.Background(), kill.Target{}, kill.Options{})
	if !errors.Is(err, kill.ErrPIDReused) {
		t.Fatalf("got %v, want ErrPIDReused", err)
	}
}

func TestFakeKillerRecordsEscalateCalls(t *testing.T) {
	f := &FakeKiller{EscalateResult: kill.Result{Status: kill.StatusKilled}}
	target := kill.Target{PID: 7}

	res, err := f.Escalate(context.Background(), target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != kill.StatusKilled {
		t.Fatalf("got %v, want StatusKilled", res.Status)
	}
	if len(f.EscalateCalls) != 1 || f.EscalateCalls[0] != target {
		t.Fatalf("EscalateCalls not recorded correctly: %+v", f.EscalateCalls)
	}
}
