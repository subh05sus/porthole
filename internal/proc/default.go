//go:build !linux && !darwin

package proc

import "errors"

// ErrNotImplemented is returned by the default Lookup until a real
// platform-specific implementation exists for the current build.
var ErrNotImplemented = errors.New("proc: no metadata resolver implemented for this platform yet")

type notImplementedLookup struct{}

func (notImplementedLookup) Lookup(pid int) (Info, error) {
	return Info{}, ErrNotImplemented
}

// NewDefaultLookup returns the Lookup for the current platform, mirroring
// scan.NewDefaultLister's build-tag-narrowing factory pattern.
func NewDefaultLookup() Lookup {
	return notImplementedLookup{}
}
