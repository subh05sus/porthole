package scan

import (
	"context"
	"errors"
	"testing"
)

// This test is scaffolding for the window before any OS scanner exists; it
// is expected to stop compiling/matching once scan_windows.go narrows
// default.go's build tag away from this platform, at which point it should
// be deleted rather than fixed.
func TestNewDefaultListerReturnsNotImplementedUntilAnOSLands(t *testing.T) {
	_, err := NewDefaultLister().List(context.Background())
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("got %v, want ErrNotImplemented", err)
	}
}
