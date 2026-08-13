package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/kill/killtest"
	"github.com/subh05sus/porthole/internal/proc"
	"github.com/subh05sus/porthole/internal/proc/proctest"
	"github.com/subh05sus/porthole/internal/restart/restarttest"
	"github.com/subh05sus/porthole/internal/scan"
	"github.com/subh05sus/porthole/internal/scan/scantest"
)

func newRestartTestApp(services []scan.Service, killer *killtest.FakeKiller, lookup *proctest.FakeLookup, spawner *restarttest.FakeSpawner) (*App, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister:  &scantest.FakeLister{Services: services},
		Killer:  killer,
		Lookup:  lookup,
		Spawner: spawner,
		Stdin:   strings.NewReader(""),
		Stdout:  &stdout,
		Stderr:  &stderr,
	}
	return app, &stdout, &stderr
}

func TestRestartHappyPath(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	lookup := &proctest.FakeLookup{Info: proc.Info{Cmdline: "node server.js", Argv: []string{"node", "server.js"}, CWD: "/app", Env: []string{"X=1"}}}
	spawner := &restarttest.FakeSpawner{}
	services := []scan.Service{{Port: 3000, PID: 111, Process: "node", Owned: true}}
	app, stdout, _ := newRestartTestApp(services, killer, lookup, spawner)

	code := Execute(app, []string{"restart", "3000"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "restarted node on :3000") {
		t.Fatalf("missing success message: %q", stdout.String())
	}
	if len(killer.ExecuteCalls) != 1 {
		t.Fatalf("expected Execute called once, got %d", len(killer.ExecuteCalls))
	}
	if len(spawner.Calls) != 1 || spawner.Calls[0].CWD != "/app" {
		t.Fatalf("expected Spawn called with the captured plan, got %+v", spawner.Calls)
	}
}

func TestRestartNothingFoundExitsOne(t *testing.T) {
	app, _, stderr := newRestartTestApp(nil, &killtest.FakeKiller{}, &proctest.FakeLookup{}, &restarttest.FakeSpawner{})

	code := Execute(app, []string{"restart", "9999"})
	if code != ExitNotFound {
		t.Fatalf("got exit code %d, want %d", code, ExitNotFound)
	}
	if !strings.Contains(stderr.String(), "nothing found on port 9999") {
		t.Fatalf("missing message: %q", stderr.String())
	}
}

func TestRestartRefusesOnUnownedRow(t *testing.T) {
	services := []scan.Service{{Port: 631, PID: 1, Process: "cupsd", Owned: false}}
	killer := &killtest.FakeKiller{}
	app, _, stderr := newRestartTestApp(services, killer, &proctest.FakeLookup{}, &restarttest.FakeSpawner{})

	code := Execute(app, []string{"restart", "631"})
	if code != ExitPermissionDenied {
		t.Fatalf("got exit code %d, want %d", code, ExitPermissionDenied)
	}
	if !strings.Contains(stderr.String(), "needs elevated permissions") {
		t.Fatalf("missing message: %q", stderr.String())
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("must never attempt to kill an unowned row, got %d Execute calls", len(killer.ExecuteCalls))
	}
}

func TestRestartRefusesWhenEnvUnavailable(t *testing.T) {
	services := []scan.Service{{Port: 3000, PID: 1, Process: "node", Owned: true}}
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	lookup := &proctest.FakeLookup{Info: proc.Info{Cmdline: "node x", EnvErr: errFakeEnv}}
	spawner := &restarttest.FakeSpawner{}
	app, _, stderr := newRestartTestApp(services, killer, lookup, spawner)

	code := Execute(app, []string{"restart", "3000"})
	if code != ExitKillFailed {
		t.Fatalf("got exit code %d, want %d", code, ExitKillFailed)
	}
	if !strings.Contains(stderr.String(), "restart:") {
		t.Fatalf("missing restart refusal message: %q", stderr.String())
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("must never kill the process if the respawn plan can't be captured, got %d Execute calls", len(killer.ExecuteCalls))
	}
	if len(spawner.Calls) != 0 {
		t.Fatalf("must never spawn if capture failed, got %d Spawn calls", len(spawner.Calls))
	}
}

func TestRestartDoesNotSpawnIfKillFails(t *testing.T) {
	services := []scan.Service{{Port: 3000, PID: 1, Process: "node", Owned: true}}
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusNeedsEscalation}}
	lookup := &proctest.FakeLookup{Info: proc.Info{Cmdline: "node x", Argv: []string{"node", "x"}}}
	spawner := &restarttest.FakeSpawner{}
	app, _, stderr := newRestartTestApp(services, killer, lookup, spawner)

	code := Execute(app, []string{"restart", "3000"})
	if code != ExitKillFailed {
		t.Fatalf("got exit code %d, want %d", code, ExitKillFailed)
	}
	if !strings.Contains(stderr.String(), "not restarting") {
		t.Fatalf("missing message: %q", stderr.String())
	}
	if len(spawner.Calls) != 0 {
		t.Fatalf("must never spawn if the kill didn't succeed, got %d Spawn calls", len(spawner.Calls))
	}
}

func TestRestartReportsSpawnFailureAfterSuccessfulKill(t *testing.T) {
	services := []scan.Service{{Port: 3000, PID: 1, Process: "node", Owned: true}}
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	lookup := &proctest.FakeLookup{Info: proc.Info{Cmdline: "node x", Argv: []string{"node", "x"}}}
	spawner := &restarttest.FakeSpawner{Err: errFakeSpawn}
	app, _, stderr := newRestartTestApp(services, killer, lookup, spawner)

	code := Execute(app, []string{"restart", "3000"})
	if code != ExitKillFailed {
		t.Fatalf("got exit code %d, want %d", code, ExitKillFailed)
	}
	if !strings.Contains(stderr.String(), "failed to respawn") {
		t.Fatalf("missing message: %q", stderr.String())
	}
}

var errFakeEnv = errors.New("environment unavailable")
var errFakeSpawn = errors.New("spawn boom")
