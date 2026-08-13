package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/subh05sus/porthole/internal/scan"
)

func TestOneLineNoServices(t *testing.T) {
	var buf bytes.Buffer
	if err := OneLine(&buf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "0 services" {
		t.Fatalf("got %q, want %q", got, "0 services")
	}
}

func TestOneLineSingularService(t *testing.T) {
	var buf bytes.Buffer
	if err := OneLine(&buf, []scan.Service{{Port: 3000, Owned: true}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "1 service" {
		t.Fatalf("got %q, want %q", got, "1 service")
	}
}

func TestOneLineReportsLockedCount(t *testing.T) {
	var buf bytes.Buffer
	services := []scan.Service{{Port: 1, Owned: true}, {Port: 2, Owned: false}, {Port: 3, Owned: false}}
	if err := OneLine(&buf, services); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "3 services · 2 locked" {
		t.Fatalf("got %q, want %q", got, "3 services · 2 locked")
	}
}
