//go:build !linux && !darwin

package scan

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by the default Lister until a real
// platform-specific scanner exists for the current build.
var ErrNotImplemented = errors.New("porthole: no scanner implemented for this platform yet")

type notImplementedLister struct{}

func (notImplementedLister) List(ctx context.Context) ([]Service, error) {
	return nil, ErrNotImplemented
}

// NewDefaultLister returns the Lister for the current platform. This file
// has no build tag today because no OS implementation exists yet; as each
// one lands (scan_linux.go, scan_darwin.go, scan_windows.go), this file is
// narrowed with a matching negative build tag so exactly one implementation
// of NewDefaultLister is visible to the compiler per GOOS at all times.
func NewDefaultLister() Lister {
	return notImplementedLister{}
}
