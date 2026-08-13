// Package proctest provides a scriptable proc.Lookup test double, mirroring
// scan/scantest and kill/killtest.
package proctest

import "github.com/subh05sus/porthole/internal/proc"

var _ proc.Lookup = (*FakeLookup)(nil)

// FakeLookup returns a single scripted Info/error regardless of the PID
// requested — sufficient for testing call sites that look up exactly one
// process at a time (restart, resource-stat queries), matching the same
// "one scripted result" simplicity as scantest.FakeLister.
type FakeLookup struct {
	Info Info
	Err  error

	Calls []int
}

// Info is a type alias so callers can write proctest.Info{...} without an
// extra import of internal/proc alongside this package.
type Info = proc.Info

func (f *FakeLookup) Lookup(pid int) (proc.Info, error) {
	f.Calls = append(f.Calls, pid)
	if f.Err != nil {
		return proc.Info{}, f.Err
	}
	return f.Info, nil
}
