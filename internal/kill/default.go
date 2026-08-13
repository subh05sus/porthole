//go:build !linux && !darwin && !windows

package kill

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by the default Killer until a real
// platform-specific killer exists for the current build.
var ErrNotImplemented = errors.New("porthole: no killer implemented for this platform yet")

type notImplementedKiller struct{}

func (notImplementedKiller) Execute(ctx context.Context, target Target, opts Options) (Result, error) {
	return Result{}, ErrNotImplemented
}

func (notImplementedKiller) Escalate(ctx context.Context, target Target) (Result, error) {
	return Result{}, ErrNotImplemented
}

// NewDefaultKiller returns the Killer for the current platform. This file
// has no build tag today because no OS implementation exists yet; as
// kill_unix.go and kill_windows.go land, this file is narrowed with a
// matching negative build tag, mirroring scan.NewDefaultLister.
func NewDefaultKiller() Killer {
	return notImplementedKiller{}
}
