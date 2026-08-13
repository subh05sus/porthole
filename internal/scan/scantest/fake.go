// Package scantest provides a scriptable scan.Lister test double so that
// every layer above the OS boundary (CLI, TUI, watch engine) can be tested
// without a real operating system.
package scantest

import (
	"context"

	"github.com/subh05sus/porthole/internal/scan"
)

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
