package scan

import (
	"context"
	"testing"
	"time"
)

// sequencedLister returns a different scripted result on each successive
// List call (clamped to the last entry once exhausted), letting watch
// tests control exactly what changes between scans.
type sequencedLister struct {
	results [][]Service
	calls   int
}

func (s *sequencedLister) List(ctx context.Context) ([]Service, error) {
	i := s.calls
	if i >= len(s.results) {
		i = len(s.results) - 1
	}
	s.calls++
	return s.results[i], nil
}

func recvEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()
	select {
	case ev := <-events:
		return ev
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for a watch Event")
		return Event{}
	}
}

func TestWatchSendsInitialScanImmediately(t *testing.T) {
	lister := &sequencedLister{results: [][]Service{{{Port: 3000}}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := Watch(ctx, lister, time.Second, make(chan time.Time))

	ev := recvEvent(t, events)
	if ev.Err != nil {
		t.Fatalf("unexpected error: %v", ev.Err)
	}
	if len(ev.Services) != 1 || ev.Services[0].Port != 3000 {
		t.Fatalf("got %+v", ev.Services)
	}
	if len(ev.Diff.Added) != 1 {
		t.Fatalf("expected the first scan to report its service as added, got %+v", ev.Diff)
	}
}

func TestWatchDiffDetectsAdded(t *testing.T) {
	lister := &sequencedLister{results: [][]Service{
		{{Port: 3000, PID: 1}},
		{{Port: 3000, PID: 1}, {Port: 8080, PID: 2}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)
	events := Watch(ctx, lister, time.Second, ticks)

	recvEvent(t, events) // initial scan

	ticks <- time.Now()
	second := recvEvent(t, events)
	if len(second.Diff.Added) != 1 || second.Diff.Added[0].Port != 8080 {
		t.Fatalf("expected port 8080 added, got %+v", second.Diff)
	}
	if len(second.Diff.Removed) != 0 {
		t.Fatalf("expected nothing removed, got %+v", second.Diff.Removed)
	}
}

func TestWatchDiffDetectsRemoved(t *testing.T) {
	lister := &sequencedLister{results: [][]Service{
		{{Port: 3000, PID: 1}, {Port: 8080, PID: 2}},
		{{Port: 3000, PID: 1}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)
	events := Watch(ctx, lister, time.Second, ticks)

	recvEvent(t, events)
	ticks <- time.Now()
	second := recvEvent(t, events)

	if len(second.Diff.Removed) != 1 || second.Diff.Removed[0].Port != 8080 {
		t.Fatalf("expected port 8080 removed, got %+v", second.Diff)
	}
}

func TestWatchTreatsPIDReuseAsRemoveThenAdd(t *testing.T) {
	lister := &sequencedLister{results: [][]Service{
		{{Port: 3000, PID: 100, StartTime: 1}},
		{{Port: 3000, PID: 100, StartTime: 2}}, // same port+PID, recycled
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)
	events := Watch(ctx, lister, time.Second, ticks)

	recvEvent(t, events)
	ticks <- time.Now()
	second := recvEvent(t, events)

	if len(second.Diff.Added) != 1 || second.Diff.Added[0].StartTime != 2 {
		t.Fatalf("expected the recycled PID's new StartTime to show as added, got %+v", second.Diff.Added)
	}
	if len(second.Diff.Removed) != 1 || second.Diff.Removed[0].StartTime != 1 {
		t.Fatalf("expected the old StartTime to show as removed, got %+v", second.Diff.Removed)
	}
}

func TestWatchNoChangeReportsEmptyDiff(t *testing.T) {
	same := []Service{{Port: 3000, PID: 1}}
	lister := &sequencedLister{results: [][]Service{same, same}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)
	events := Watch(ctx, lister, time.Second, ticks)

	recvEvent(t, events)
	ticks <- time.Now()
	second := recvEvent(t, events)

	if len(second.Diff.Added) != 0 || len(second.Diff.Removed) != 0 {
		t.Fatalf("expected empty diff for an unchanged scan, got %+v", second.Diff)
	}
}

func TestWatchClosesChannelWhenContextCancelled(t *testing.T) {
	lister := &sequencedLister{results: [][]Service{{}}}
	ctx, cancel := context.WithCancel(context.Background())
	events := Watch(ctx, lister, time.Second, make(chan time.Time))

	recvEvent(t, events)
	cancel()

	select {
	case _, ok := <-events:
		if ok {
			t.Fatalf("expected the channel to close after context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for the watch channel to close")
	}
}

// flakyLister errors on its second call, then succeeds again — used to
// confirm a failed scan doesn't corrupt the diff baseline.
type flakyLister struct {
	results [][]Service
	failOn  int
	calls   int
}

func (f *flakyLister) List(ctx context.Context) ([]Service, error) {
	i := f.calls
	f.calls++
	if i == f.failOn {
		return nil, errFake
	}
	if i >= len(f.results) {
		i = len(f.results) - 1
	}
	return f.results[i], nil
}

func TestWatchPropagatesScanErrorWithoutUpdatingBaseline(t *testing.T) {
	lister := &flakyLister{
		results: [][]Service{{{Port: 3000, PID: 1}}, nil, {{Port: 3000, PID: 1}}},
		failOn:  1,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ticks := make(chan time.Time)
	events := Watch(ctx, lister, time.Second, ticks)

	first := recvEvent(t, events)
	if first.Err != nil || len(first.Diff.Added) != 1 {
		t.Fatalf("precondition failed: got %+v", first)
	}

	ticks <- time.Now()
	second := recvEvent(t, events)
	if second.Err == nil {
		t.Fatalf("expected the second scan's error to be reported")
	}

	// Baseline must still be the successful first scan, so the third
	// (successful, unchanged) scan reports an empty diff rather than
	// treating everything as newly added because the failed scan reset it.
	ticks <- time.Now()
	third := recvEvent(t, events)
	if third.Err != nil {
		t.Fatalf("unexpected error on third scan: %v", third.Err)
	}
	if len(third.Diff.Added) != 0 || len(third.Diff.Removed) != 0 {
		t.Fatalf("expected empty diff (baseline preserved across the failed scan), got %+v", third.Diff)
	}
}

var errFake = fakeErr("boom")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }
