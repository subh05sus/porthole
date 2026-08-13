package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/subh05sus/porthole/internal/scan"
	"github.com/subh05sus/porthole/internal/scan/scantest"
)

func newTestApp(services []scan.Service) (*App, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: &scantest.FakeLister{Services: services},
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	}
	return app, &stdout, &stderr
}

func TestListPrintsTable(t *testing.T) {
	app, stdout, _ := newTestApp([]scan.Service{{Port: 3000, Process: "node", Owned: true}})

	code := Execute(app, []string{"list"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "3000") || !strings.Contains(stdout.String(), "node") {
		t.Fatalf("missing expected content: %q", stdout.String())
	}
}

func TestListJSONFlag(t *testing.T) {
	app, stdout, _ := newTestApp([]scan.Service{{Port: 3000, Process: "node", Owned: true}})

	code := Execute(app, []string{"list", "--json"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}

	var got []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout.String())
	}
	if len(got) != 1 || got[0]["port"].(float64) != 3000 {
		t.Fatalf("got %+v", got)
	}
}

func TestListPortFilter(t *testing.T) {
	app, stdout, _ := newTestApp([]scan.Service{
		{Port: 3000, Process: "node", Owned: true},
		{Port: 5432, Process: "postgres", Owned: true},
	})

	code := Execute(app, []string{"list", "--port", "5432"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	out := stdout.String()
	if strings.Contains(out, "node") || !strings.Contains(out, "postgres") {
		t.Fatalf("port filter did not apply: %q", out)
	}
}

func TestListProjectFilter(t *testing.T) {
	app, stdout, _ := newTestApp([]scan.Service{
		{Port: 3000, Process: "node", Project: "zapmail-web", Owned: true},
		{Port: 5173, Process: "node", Project: "slotli", Owned: true},
	})

	code := Execute(app, []string{"list", "--project", "slotli"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	out := stdout.String()
	if strings.Contains(out, "3000") || !strings.Contains(out, "5173") {
		t.Fatalf("project filter did not apply: %q", out)
	}
}

func TestListPropagatesScanError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: &scantest.FakeLister{Err: errBoom},
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	code := Execute(app, []string{"list"})
	if code == ExitSuccess {
		t.Fatalf("expected non-zero exit code on scan failure")
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Fatalf("expected error surfaced on stderr: %q", stderr.String())
	}
}

func TestListSinceFilter(t *testing.T) {
	app, stdout, _ := newTestApp([]scan.Service{
		{Port: 3000, Process: "fresh", Owned: true, Uptime: time.Minute},
		{Port: 5432, Process: "old", Owned: true, Uptime: time.Hour},
	})

	code := Execute(app, []string{"list", "--since", "5m"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "fresh") || strings.Contains(out, "old") {
		t.Fatalf("--since filter did not apply correctly: %q", out)
	}
}

func TestListOnelineFlag(t *testing.T) {
	app, stdout, _ := newTestApp([]scan.Service{
		{Port: 3000, Process: "node", Owned: true},
		{Port: 5432, Process: "postgres", Owned: false},
	})

	code := Execute(app, []string{"list", "--oneline"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if got := strings.TrimSpace(stdout.String()); got != "2 services · 1 locked" {
		t.Fatalf("got %q, want %q", got, "2 services · 1 locked")
	}
}
