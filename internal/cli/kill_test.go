package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/kill/killtest"
	"github.com/subh05sus/porthole/internal/scan"
	"github.com/subh05sus/porthole/internal/scan/scantest"
)

func newKillTestApp(services []scan.Service, killer *killtest.FakeKiller, stdin string) (*App, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: &scantest.FakeLister{Services: services},
		Killer: killer,
		Stdin:  strings.NewReader(stdin),
		Stdout: &stdout,
		Stderr: &stderr,
	}
	return app, &stdout, &stderr
}

func TestKillSuccessWithConfirmation(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	app, stdout, _ := newKillTestApp([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer, "y\n")

	code := Execute(app, []string{"kill", "3000"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "terminated node on :3000") {
		t.Fatalf("missing success message: %q", stdout.String())
	}
	if len(killer.ExecuteCalls) != 1 {
		t.Fatalf("got %d Execute calls, want 1", len(killer.ExecuteCalls))
	}
}

func TestKillDeclinedConfirmationSkips(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	app, stdout, _ := newKillTestApp([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer, "n\n")

	code := Execute(app, []string{"kill", "3000"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "skipped :3000") {
		t.Fatalf("missing skip message: %q", stdout.String())
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("got %d Execute calls, want 0 (declined confirmation must not kill)", len(killer.ExecuteCalls))
	}
}

func TestKillYesFlagSkipsPrompt(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	app, _, _ := newKillTestApp([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer, "")

	code := Execute(app, []string{"kill", "3000", "--yes"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if len(killer.ExecuteCalls) != 1 {
		t.Fatalf("got %d Execute calls, want 1", len(killer.ExecuteCalls))
	}
}

func TestKillDryRunDoesNothing(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	app, stdout, _ := newKillTestApp([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer, "")

	code := Execute(app, []string{"kill", "3000", "--dry-run"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "would kill node") {
		t.Fatalf("missing dry-run message: %q", stdout.String())
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("dry-run must never call Execute, got %d calls", len(killer.ExecuteCalls))
	}
}

func TestKillForceFlagPassedThrough(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	app, _, _ := newKillTestApp([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer, "")

	code := Execute(app, []string{"kill", "3000", "--force", "--yes"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if !killer.ExecuteCalls[0].Opts.Force {
		t.Fatalf("expected Force option to be true")
	}
}

func TestKillNothingFoundExitsOne(t *testing.T) {
	killer := &killtest.FakeKiller{}
	app, _, stderr := newKillTestApp(nil, killer, "")

	code := Execute(app, []string{"kill", "9999", "--yes"})
	if code != ExitNotFound {
		t.Fatalf("got exit code %d, want %d", code, ExitNotFound)
	}
	if !strings.Contains(stderr.String(), "nothing found on port 9999") {
		t.Fatalf("missing not-found message: %q", stderr.String())
	}
}

func TestKillPortRangeExpandsAndDeduplicates(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	services := []scan.Service{
		{Port: 3000, PID: 1, Process: "a", Owned: true},
		{Port: 3001, PID: 2, Process: "b", Owned: true},
		{Port: 3002, PID: 3, Process: "c", Owned: true},
	}
	app, _, _ := newKillTestApp(services, killer, "")

	code := Execute(app, []string{"kill", "3000-3002", "3001", "--yes"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if len(killer.ExecuteCalls) != 3 {
		t.Fatalf("got %d Execute calls, want 3 (deduplicated range+explicit port)", len(killer.ExecuteCalls))
	}
}

func TestKillProjectFlag(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	services := []scan.Service{
		{Port: 3000, PID: 1, Process: "node", Project: "zapmail-web", Owned: true},
		{Port: 3001, PID: 2, Process: "node", Project: "zapmail-api", Owned: true},
		{Port: 5173, PID: 3, Process: "node", Project: "slotli", Owned: true},
	}
	app, _, _ := newKillTestApp(services, killer, "")

	code := Execute(app, []string{"kill", "--project", "zapmail-web", "--yes"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if len(killer.ExecuteCalls) != 1 || killer.ExecuteCalls[0].Target.PID != 1 {
		t.Fatalf("expected exactly the zapmail-web service to be killed, got %+v", killer.ExecuteCalls)
	}
}

func TestKillPortsAndProjectAreMutuallyExclusive(t *testing.T) {
	app, _, _ := newKillTestApp(nil, &killtest.FakeKiller{}, "")

	code := Execute(app, []string{"kill", "3000", "--project", "slotli"})
	if code != ExitNotFound {
		t.Fatalf("got exit code %d, want %d", code, ExitNotFound)
	}
}

func TestKillPIDReusedExitsFour(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteErr: kill.ErrPIDReused}
	app, _, stderr := newKillTestApp([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer, "")

	code := Execute(app, []string{"kill", "3000", "--yes"})
	if code != ExitPIDChanged {
		t.Fatalf("got exit code %d, want %d", code, ExitPIDChanged)
	}
	if !strings.Contains(stderr.String(), "process changed since scan, refreshed") {
		t.Fatalf("missing PID-reuse message: %q", stderr.String())
	}
}

func TestKillNotOwnedExitsTwo(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteErr: kill.ErrNotOwned}
	app, _, stderr := newKillTestApp([]scan.Service{{Port: 631, PID: 892, Process: "cupsd", Owned: false}}, killer, "")

	code := Execute(app, []string{"kill", "631", "--yes"})
	if code != ExitPermissionDenied {
		t.Fatalf("got exit code %d, want %d", code, ExitPermissionDenied)
	}
	if !strings.Contains(stderr.String(), "needs elevated permissions") {
		t.Fatalf("missing permission message: %q", stderr.String())
	}
}

func TestKillIgnoredSignalExitsThree(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusNeedsEscalation}}
	app, _, stderr := newKillTestApp([]scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}, killer, "")

	code := Execute(app, []string{"kill", "3000", "--yes"})
	if code != ExitKillFailed {
		t.Fatalf("got exit code %d, want %d", code, ExitKillFailed)
	}
	if !strings.Contains(stderr.String(), "ignored kill signal") {
		t.Fatalf("missing failure message: %q", stderr.String())
	}
}

func TestKillNoArgsAndNoProjectErrors(t *testing.T) {
	app, _, _ := newKillTestApp(nil, &killtest.FakeKiller{}, "")

	code := Execute(app, []string{"kill"})
	if code != ExitNotFound {
		t.Fatalf("got exit code %d, want %d", code, ExitNotFound)
	}
}

func TestParsePortsInvalidRangeErrors(t *testing.T) {
	_, err := parsePorts([]string{"3010-3000"})
	if err == nil {
		t.Fatalf("expected error for descending range")
	}
}

func TestParsePortsInvalidPortErrors(t *testing.T) {
	_, err := parsePorts([]string{"not-a-port"})
	if err == nil {
		t.Fatalf("expected error for non-numeric port")
	}
}
