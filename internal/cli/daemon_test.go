package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/subh05sus/porthole/internal/config"
	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/kill/killtest"
	"github.com/subh05sus/porthole/internal/scan"
)

func TestDaemonRefusesToStartWhenNotEnabledInConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: &sequencedLister{results: [][]scan.Service{{}}},
		Killer: &killtest.FakeKiller{},
		Config: config.Config{}, // AutoKill.Enabled left false
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	code := Execute(app, []string{"daemon"})
	if code == ExitSuccess {
		t.Fatalf("expected a non-zero exit when auto_kill.enabled is false")
	}
	if !strings.Contains(stderr.String(), "auto_kill.enabled") {
		t.Fatalf("expected a clear refusal message, got %q", stderr.String())
	}
}

func TestRunDaemonDryRunReportsMatchesWithoutKilling(t *testing.T) {
	match := scan.Service{Port: 3000, PID: 111, StartTime: 1, Process: "node", Owned: true}
	lister := &sequencedLister{results: [][]scan.Service{{match}, {match}}}
	killer := &killtest.FakeKiller{}
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: lister,
		Killer: killer,
		Config: config.Config{AutoKill: config.AutoKill{Enabled: true, Allow: []config.AutoKillEntry{{Port: 3000, Process: "node"}}}},
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runDaemon(ctx, app, false, time.Second, make(chan time.Time)) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("runDaemon did not return after context cancellation")
	}

	if !strings.Contains(stdout.String(), "[dry-run] would kill node on :3000") {
		t.Fatalf("expected a dry-run report, got %q", stdout.String())
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("dry-run must never call Killer.Execute, got %d calls", len(killer.ExecuteCalls))
	}
}

func TestRunDaemonLiveModeKillsAndReports(t *testing.T) {
	match := scan.Service{Port: 3000, PID: 111, StartTime: 1, Process: "node", Owned: true}
	lister := &sequencedLister{results: [][]scan.Service{{match}, {match}}}
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: lister,
		Killer: killer,
		Config: config.Config{AutoKill: config.AutoKill{Enabled: true, Allow: []config.AutoKillEntry{{Port: 3000, Process: "node"}}}},
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runDaemon(ctx, app, true, time.Second, make(chan time.Time)) }()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if !strings.Contains(stdout.String(), "killed node on :3000") {
		t.Fatalf("expected a kill report, got %q", stdout.String())
	}
	if len(killer.ExecuteCalls) != 1 {
		t.Fatalf("expected exactly one real kill, got %d", len(killer.ExecuteCalls))
	}
	if !strings.Contains(stdout.String(), "LIVE mode") {
		t.Fatalf("expected the startup banner to say LIVE mode, got %q", stdout.String())
	}
}
