package restart

import (
	"errors"
	"testing"

	"github.com/subh05sus/porthole/internal/proc"
	"github.com/subh05sus/porthole/internal/proc/proctest"
	"github.com/subh05sus/porthole/internal/scan"
)

func TestCaptureBuildsPlanFromLookup(t *testing.T) {
	lookup := &proctest.FakeLookup{Info: proc.Info{
		ExePath: "/usr/bin/node",
		Cmdline: "node server.js",
		Argv:    []string{"node", "server.js"},
		CWD:     "/app",
		Env:     []string{"PATH=/usr/bin", "NODE_ENV=production"},
	}}
	target := scan.Service{PID: 111, Process: "node"}

	plan, err := Capture(lookup, target)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.ExePath != "/usr/bin/node" || plan.CWD != "/app" {
		t.Fatalf("got %+v", plan)
	}
	if len(plan.Env) != 2 {
		t.Fatalf("expected 2 env entries, got %+v", plan.Env)
	}
	if len(lookup.Calls) != 1 || lookup.Calls[0] != 111 {
		t.Fatalf("expected Lookup called with pid 111, got %+v", lookup.Calls)
	}
}

func TestCaptureRefusesOnEnvError(t *testing.T) {
	envErr := errors.New("reading another process's environment is not supported on macOS without root")
	lookup := &proctest.FakeLookup{Info: proc.Info{
		Cmdline: "node server.js",
		EnvErr:  envErr,
	}}

	_, err := Capture(lookup, scan.Service{PID: 1})
	if err == nil {
		t.Fatalf("expected an error when EnvErr is set")
	}
	if !errors.Is(err, envErr) {
		t.Fatalf("got %v, want it to wrap the original EnvErr", err)
	}
}

func TestCaptureRefusesWithNoCommandLine(t *testing.T) {
	lookup := &proctest.FakeLookup{Info: proc.Info{}}

	_, err := Capture(lookup, scan.Service{PID: 1, Process: "mystery"})
	if err == nil {
		t.Fatalf("expected an error when neither Cmdline nor Argv is available")
	}
}

func TestCapturePropagatesLookupError(t *testing.T) {
	lookupErr := errors.New("boom")
	lookup := &proctest.FakeLookup{Err: lookupErr}

	_, err := Capture(lookup, scan.Service{PID: 1})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("got %v, want it to wrap the lookup error", err)
	}
}

func TestCaptureNilEnvBecomesEmptyNotNil(t *testing.T) {
	// A process with no EnvErr but also no Env slice (shouldn't happen in
	// practice, but guards against a Lookup implementation bug silently
	// making Spawn inherit porthole's own environment via a nil Env).
	lookup := &proctest.FakeLookup{Info: proc.Info{Cmdline: "x", Env: nil}}

	plan, err := Capture(lookup, scan.Service{PID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Env == nil {
		t.Fatalf("expected a non-nil (possibly empty) Env, got nil")
	}
	if len(plan.Env) != 0 {
		t.Fatalf("expected empty Env, got %+v", plan.Env)
	}
}
