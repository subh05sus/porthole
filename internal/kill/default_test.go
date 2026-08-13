package kill

import (
	"context"
	"errors"
	"testing"
)

// Scaffolding for the window before any OS killer exists; delete once
// kill_windows.go narrows default.go's build tag away from this platform.
func TestNewDefaultKillerReturnsNotImplementedUntilAnOSLands(t *testing.T) {
	k := NewDefaultKiller()

	if _, err := k.Execute(context.Background(), Target{}, Options{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Execute: got %v, want ErrNotImplemented", err)
	}
	if _, err := k.Escalate(context.Background(), Target{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Escalate: got %v, want ErrNotImplemented", err)
	}
}
