package output

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/subh05sus/porthole/internal/scan"
)

func TestTableRendersHeaderAndRows(t *testing.T) {
	services := []scan.Service{
		{Port: 3000, PID: 48211, Process: "node", Project: "zapmail-web", Owned: true, Uptime: 2*time.Hour + 14*time.Minute},
	}

	var buf bytes.Buffer
	if err := Table(&buf, services); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "PORT") || !strings.Contains(out, "PROCESS") {
		t.Fatalf("missing header: %q", out)
	}
	if !strings.Contains(out, "3000") || !strings.Contains(out, "node") || !strings.Contains(out, "zapmail-web") || !strings.Contains(out, "2h 14m") {
		t.Fatalf("missing expected row content: %q", out)
	}
}

func TestTableMarksUnownedRowsLocked(t *testing.T) {
	services := []scan.Service{
		{Port: 631, PID: 892, Process: "cupsd", Owned: false, ResolveErr: errors.New("permission denied")},
	}

	var buf bytes.Buffer
	if err := Table(&buf, services); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "cupsd (locked)") {
		t.Fatalf("expected locked marker on unowned row: %q", out)
	}
}

func TestTableNeverDropsUnresolvedRows(t *testing.T) {
	services := []scan.Service{
		{Port: 4000, PID: 0, Process: "", ResolveErr: errors.New("some failure")},
	}

	var buf bytes.Buffer
	if err := Table(&buf, services); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "4000") {
		t.Fatalf("row for unresolved service must still render its port: %q", out)
	}
}

func TestTableEmptyProjectRendersDash(t *testing.T) {
	services := []scan.Service{{Port: 5432, PID: 1211, Process: "postgres", Owned: true}}

	var buf bytes.Buffer
	if err := Table(&buf, services); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected header + 1 row, got %d lines: %q", len(lines), buf.String())
	}
	if !strings.Contains(lines[1], "-") {
		t.Fatalf("expected dash placeholder for empty project: %q", lines[1])
	}
}

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "-"},
		{-time.Minute, "-"},
		{18 * time.Minute, "18m"},
		{2*time.Hour + 14*time.Minute, "2h 14m"},
		{6 * 24 * time.Hour, "6d"},
	}
	for _, c := range cases {
		if got := formatUptime(c.d); got != c.want {
			t.Errorf("formatUptime(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
