package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/subh05sus/porthole/internal/config"
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

func TestKillProtectedPortRequiresTypedConfirmation(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	services := []scan.Service{{Port: 5432, PID: 1, Process: "postgres", Owned: true}}
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: &scantest.FakeLister{Services: services},
		Killer: killer,
		Config: config.Config{Protected: []config.ProtectedPort{{Port: 5432, Reason: "prod db"}}},
		Stdin:  strings.NewReader("5432\n"),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	code := Execute(app, []string{"kill", "5432"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0: stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "protected") || !strings.Contains(stdout.String(), "prod db") {
		t.Fatalf("expected the protected-port prompt to mention the reason, got %q", stdout.String())
	}
	if len(killer.ExecuteCalls) != 1 {
		t.Fatalf("expected Execute called once after correct typed confirmation, got %d", len(killer.ExecuteCalls))
	}
}

func TestKillProtectedPortWrongTypedInputSkips(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	services := []scan.Service{{Port: 5432, PID: 1, Process: "postgres", Owned: true}}
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: &scantest.FakeLister{Services: services},
		Killer: killer,
		Config: config.Config{Protected: []config.ProtectedPort{{Port: 5432}}},
		Stdin:  strings.NewReader("y\n"), // a plain "y" must NOT satisfy a typed-port confirmation
		Stdout: &stdout,
		Stderr: &stderr,
	}

	code := Execute(app, []string{"kill", "5432"})
	if code != ExitSuccess {
		// Matches this codebase's existing precedent for a plain declined
		// y/n confirmation (TestKillDeclinedConfirmationSkips): skipping is
		// not itself an error, exit 0 either way — what matters here is
		// that Execute was never called.
		t.Fatalf("got exit code %d, want %d", code, ExitSuccess)
	}
	if !strings.Contains(stdout.String(), "did not match") {
		t.Fatalf("missing skip message: %q", stdout.String())
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("expected Execute never called on a wrong/incomplete typed confirmation, got %d calls", len(killer.ExecuteCalls))
	}
}

func TestKillProtectedPortYesFlagDoesNotBypassConfirmation(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	services := []scan.Service{{Port: 5432, PID: 1, Process: "postgres", Owned: true}}
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: &scantest.FakeLister{Services: services},
		Killer: killer,
		Config: config.Config{Protected: []config.ProtectedPort{{Port: 5432}}},
		Stdin:  strings.NewReader(""), // empty input: never types the port
		Stdout: &stdout,
		Stderr: &stderr,
	}

	code := Execute(app, []string{"kill", "5432", "--yes"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want %d", code, ExitSuccess)
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("--yes must not bypass protected-port confirmation, got %d Execute calls", len(killer.ExecuteCalls))
	}
}

func TestKillDevSweepsOwnedPortsInRangeOnly(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	services := []scan.Service{
		{Port: 3000, PID: 1, Process: "node", Owned: true},
		{Port: 5432, PID: 2, Process: "postgres", Owned: false}, // locked, must be silently dropped
		{Port: 8080, PID: 3, Process: "go", Owned: true},
		{Port: 80, PID: 4, Process: "nginx", Owned: true}, // outside 3000-9999, must be ignored
	}
	app, stdout, stderr := newKillTestApp(services, killer, "")

	code := Execute(app, []string{"kill", "--dev", "--yes"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0: stderr=%q", code, stderr.String())
	}
	if len(killer.ExecuteCalls) != 2 {
		t.Fatalf("expected exactly the 2 owned in-range ports killed, got %d calls: %+v", len(killer.ExecuteCalls), killer.ExecuteCalls)
	}
	if strings.Contains(stderr.String(), "nothing found") {
		t.Fatalf("--dev must not spam 'nothing found' for the thousands of empty ports in range, got %q", stderr.String())
	}
	_ = stdout
}

func TestKillDevWithNoOwnedPortsReportsCleanly(t *testing.T) {
	killer := &killtest.FakeKiller{}
	services := []scan.Service{{Port: 5432, PID: 1, Process: "postgres", Owned: false}}
	app, stdout, _ := newKillTestApp(services, killer, "")

	code := Execute(app, []string{"kill", "--dev", "--yes"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "nothing owned") {
		t.Fatalf("got %q", stdout.String())
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("expected no Execute calls, got %d", len(killer.ExecuteCalls))
	}
}

func TestKillDevCannotCombineWithExplicitPorts(t *testing.T) {
	app, _, _ := newKillTestApp(nil, &killtest.FakeKiller{}, "")

	code := Execute(app, []string{"kill", "--dev", "3000"})
	if code != ExitNotFound {
		t.Fatalf("got exit code %d, want %d", code, ExitNotFound)
	}
}
