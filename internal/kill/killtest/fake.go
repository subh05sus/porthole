// Package killtest provides a scriptable kill.Killer test double so CLI and
// TUI code can be tested without a real operating system.
package killtest

import (
	"context"

	"github.com/subh05sus/porthole/internal/kill"
)

var _ kill.Killer = (*FakeKiller)(nil)

// FakeKiller returns scripted results/errors and records every call it
// receives so tests can assert on what was requested.
type FakeKiller struct {
	ExecuteResult  kill.Result
	ExecuteErr     error
	EscalateResult kill.Result
	EscalateErr    error

	ExecuteCalls  []ExecuteCall
	EscalateCalls []kill.Target
}

type ExecuteCall struct {
	Target kill.Target
	Opts   kill.Options
}

func (f *FakeKiller) Execute(ctx context.Context, target kill.Target, opts kill.Options) (kill.Result, error) {
	f.ExecuteCalls = append(f.ExecuteCalls, ExecuteCall{Target: target, Opts: opts})
	if f.ExecuteErr != nil {
		return kill.Result{}, f.ExecuteErr
	}
	return f.ExecuteResult, nil
}

func (f *FakeKiller) Escalate(ctx context.Context, target kill.Target) (kill.Result, error) {
	f.EscalateCalls = append(f.EscalateCalls, target)
	if f.EscalateErr != nil {
		return kill.Result{}, f.EscalateErr
	}
	return f.EscalateResult, nil
}
