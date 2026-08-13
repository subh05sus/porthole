package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/subh05sus/porthole/internal/scan"
)

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

	done := make(chan error, 1)
	go func() { done <- runWatch(ctx, app, time.Second, ticks) }()

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
}

func TestRunWatchStopsOnContextCancellation(t *testing.T) {
	lister := &sequencedLister{results: [][]scan.Service{{}}}
	var stdout, stderr bytes.Buffer
	app := &App{Lister: lister, Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runWatch(ctx, app, time.Second, make(chan time.Time)) }()

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
