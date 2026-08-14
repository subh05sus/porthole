package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/subh05sus/porthole/internal/config"
	"github.com/subh05sus/porthole/internal/notify"
	"github.com/subh05sus/porthole/internal/scan"
)

// spyNotifier records every Notify call. Not synchronized: every test using
// it drives runWatch via the same sleep-then-cancel-then-inspect pattern
// already used for stdout/stderr assertions in this file, so the read
// always happens after the writes it cares about have landed.
type spyNotifier struct {
	calls []string
}

func (s *spyNotifier) Notify(title, body string) {
	s.calls = append(s.calls, title+": "+body)
}

// sequencedLister mirrors internal/scan's own test fake (unexported there),
// returning a different scripted result on each successive call.
type sequencedLister struct {
	results [][]scan.Service
	calls   int
}

func (s *sequencedLister) List(ctx context.Context) ([]scan.Service, error) {
	i := s.calls
	if i >= len(s.results) {
		i = len(s.results) - 1
	}
	s.calls++
	return s.results[i], nil
}

func TestRunWatchPrintsInitialTableThenDiffsOnly(t *testing.T) {
	lister := &sequencedLister{results: [][]scan.Service{
		{{Port: 3000, PID: 1, Process: "node"}},
		{{Port: 3000, PID: 1, Process: "node"}, {Port: 8080, PID: 2, Process: "go"}},
	}}
	var stdout, stderr bytes.Buffer
	app := &App{Lister: lister, Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}

	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)

	spy := &spyNotifier{}
	done := make(chan error, 1)
	go func() { done <- runWatch(ctx, app, time.Second, ticks, spy, false) }()

	// Give the initial scan a moment to print, then trigger a second scan
	// and cancel once its output should have landed.
	time.Sleep(50 * time.Millisecond)
	ticks <- time.Now()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("runWatch did not return after context cancellation")
	}

	out := stdout.String()
	if !strings.Contains(out, "node") || !strings.Contains(out, "3000") {
		t.Fatalf("expected initial table in output, got %q", out)
	}
	if !strings.Contains(out, "+ go on :8080 (pid 2)") {
		t.Fatalf("expected an added-service diff line, got %q", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "- ") {
			t.Fatalf("nothing was removed, unexpected removal line %q in output %q", line, out)
		}
	}

	if len(spy.calls) != 1 || !strings.Contains(spy.calls[0], ":8080") {
		t.Fatalf("expected exactly one notification for the added service, got %+v", spy.calls)
	}
}

func TestRunWatchHidesUnownedWhenConfigured(t *testing.T) {
	lister := &sequencedLister{results: [][]scan.Service{
		{{Port: 3000, PID: 1, Process: "node", Owned: true}},
		{{Port: 3000, PID: 1, Process: "node", Owned: true}, {Port: 22, PID: 2, Process: "sshd", Owned: false}},
	}}
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: lister,
		Config: config.Config{Display: config.Display{HideSystemProcesses: true}},
		Stdin:  strings.NewReader(""), Stdout: &stdout, Stderr: &stderr,
	}

	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	spy := &spyNotifier{}
	done := make(chan error, 1)
	go func() { done <- runWatch(ctx, app, time.Second, ticks, spy, false) }()

	time.Sleep(50 * time.Millisecond)
	ticks <- time.Now()
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	out := stdout.String()
	if strings.Contains(out, "sshd") {
		t.Fatalf("expected the unowned added service hidden from output, got %q", out)
	}
	if len(spy.calls) != 0 {
		t.Fatalf("expected no notification for a hidden added service, got %+v", spy.calls)
	}
}

func TestRunWatchAllFlagShowsHiddenRows(t *testing.T) {
	lister := &sequencedLister{results: [][]scan.Service{
		{{Port: 22, PID: 1, Process: "sshd", Owned: false}},
	}}
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: lister,
		Config: config.Config{Display: config.Display{HideSystemProcesses: true}},
		Stdin:  strings.NewReader(""), Stdout: &stdout, Stderr: &stderr,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runWatch(ctx, app, time.Second, make(chan time.Time), notify.NoOp{}, true) }()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if !strings.Contains(stdout.String(), "sshd") {
		t.Fatalf("expected --all (true) to show the row hide_system_processes would hide, got %q", stdout.String())
	}
}

func TestRunWatchStopsOnContextCancellation(t *testing.T) {
	lister := &sequencedLister{results: [][]scan.Service{{}}}
	var stdout, stderr bytes.Buffer
	app := &App{Lister: lister, Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runWatch(ctx, app, time.Second, make(chan time.Time), notify.NoOp{}, false) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("runWatch did not return after cancellation")
	}
}
