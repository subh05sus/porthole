//go:build !linux && !darwin && !windows

package firewall

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by the default Manager on any platform
// without a real implementation (see firewall_linux.go/_darwin.go/_windows.go).
var ErrNotImplemented = errors.New("porthole: no firewall manager implemented for this platform")

type notImplementedManager struct{}

func (notImplementedManager) List(ctx context.Context) ([]Rule, error)    { return nil, ErrNotImplemented }
func (notImplementedManager) Apply(ctx context.Context, rule Rule) error  { return ErrNotImplemented }
func (notImplementedManager) Remove(ctx context.Context, rule Rule) error { return ErrNotImplemented }
func (notImplementedManager) RemoveAll(ctx context.Context) error         { return ErrNotImplemented }

func NewDefaultManager() Manager { return notImplementedManager{} }
