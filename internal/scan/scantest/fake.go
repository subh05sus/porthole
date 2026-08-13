// Package scantest provides a scriptable scan.Lister test double so that
// every layer above the OS boundary (CLI, TUI, watch engine) can be tested
// without a real operating system.
package scantest

import (
	"context"

	"github.com/subh05sus/porthole/internal/scan"
)

var _ scan.Lister = (*FakeLister)(nil)

// FakeLister returns a scripted result (or error) from List, optionally
// after waiting for a caller-controlled signal — useful for exercising
// timeout and cancellation behavior deterministically.
type FakeLister struct {
	Services []scan.Service
	Err      error

	// Ready, if non-nil, is read from before List returns, letting tests
	// control exactly when a scan "completes" without a real sleep.
	Ready <-chan struct{}
}

func (f *FakeLister) List(ctx context.Context) ([]scan.Service, error) {
	if f.Ready != nil {
		select {
		case <-f.Ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.Err != nil {
		return nil, f.Err
	}
	out := make([]scan.Service, len(f.Services))
	copy(out, f.Services)
	return out, nil
}

var _ scan.SocketQuerier = (*FakeQueryingLister)(nil)

// FakeQueryingLister wraps FakeLister and additionally implements
// scan.SocketQuerier, for tests exercising the on-demand full-socket-list
// path (e.g. the TUI detail pane) that a plain FakeLister — deliberately —
// doesn't trigger, since most callers don't need it.
type FakeQueryingLister struct {
	FakeLister
	Sockets  map[int][]scan.Service // keyed by PID
	QueryErr error
}

func (f *FakeQueryingLister) SocketsForPID(ctx context.Context, pid int) ([]scan.Service, error) {
	if f.QueryErr != nil {
		return nil, f.QueryErr
	}
	return f.Sockets[pid], nil
}
