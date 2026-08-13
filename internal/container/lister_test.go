package container

import (
	"context"
	"errors"
	"testing"

	"github.com/subh05sus/porthole/internal/scan"
	"github.com/subh05sus/porthole/internal/scan/scantest"
)

type fakeBackend struct {
	containers []Container
	err        error
	calls      int
}

func (f *fakeBackend) List(ctx context.Context) ([]Container, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.containers, nil
}

func newAwareLister(inner scan.Lister, backend listerBackend, dialErr error) *AwareLister {
	calls := 0
	return &AwareLister{
		inner: inner,
		dial: func(ctx context.Context) (listerBackend, error) {
			calls++
			if dialErr != nil {
				return nil, dialErr
			}
			return backend, nil
		},
	}
}

func TestAwareListerEnrichesWhenBackendReachable(t *testing.T) {
	inner := &scantest.FakeLister{Services: []scan.Service{{Port: 5434, Proto: scan.ProtoTCP}}}
	backend := &fakeBackend{containers: []Container{
		{ID: "abc", Names: []string{"/db"}, Ports: []Port{{PublicPort: 5434, Type: "tcp"}}},
	}}
	l := newAwareLister(inner, backend, nil)

	got, err := l.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].Container != "db" {
		t.Fatalf("expected enrichment, got %+v", got[0])
	}
}

func TestAwareListerDegradesSilentlyWhenDaemonUnreachable(t *testing.T) {
	inner := &scantest.FakeLister{Services: []scan.Service{{Port: 3000, Proto: scan.ProtoTCP, Process: "node"}}}
	l := newAwareLister(inner, nil, errors.New("no docker socket"))

	got, err := l.List(context.Background())
	if err != nil {
		t.Fatalf("a missing container runtime must not fail the scan: %v", err)
	}
	if len(got) != 1 || got[0].Process != "node" || got[0].Container != "" {
		t.Fatalf("expected the plain scan result unchanged, got %+v", got)
	}
}

func TestAwareListerPropagatesInnerScanError(t *testing.T) {
	inner := &scantest.FakeLister{Err: errors.New("scan boom")}
	l := newAwareLister(inner, &fakeBackend{}, nil)

	_, err := l.List(context.Background())
	if err == nil {
		t.Fatalf("expected the inner scan's error to propagate")
	}
}

func TestAwareListerCachesBackendAcrossCalls(t *testing.T) {
	inner := &scantest.FakeLister{Services: []scan.Service{{Port: 1}}}
	backend := &fakeBackend{}
	dials := 0
	l := &AwareLister{
		inner: inner,
		dial: func(ctx context.Context) (listerBackend, error) {
			dials++
			return backend, nil
		},
	}

	if _, err := l.List(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := l.List(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dials != 1 {
		t.Fatalf("expected exactly one dial across two List calls, got %d", dials)
	}
	if backend.calls != 2 {
		t.Fatalf("expected the cached backend's List to be called once per scan, got %d", backend.calls)
	}
}

func TestAwareListerRedialsAfterBackendFailure(t *testing.T) {
	inner := &scantest.FakeLister{Services: []scan.Service{{Port: 1}}}
	failing := &fakeBackend{err: errors.New("daemon restarted")}
	working := &fakeBackend{}
	dials := 0
	backends := []listerBackend{failing, working}
	l := &AwareLister{
		inner: inner,
		dial: func(ctx context.Context) (listerBackend, error) {
			b := backends[dials]
			dials++
			return b, nil
		},
	}

	if _, err := l.List(context.Background()); err != nil {
		t.Fatalf("a failed container query must not fail the scan: %v", err)
	}
	if _, err := l.List(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dials != 2 {
		t.Fatalf("expected a redial after the first backend's List failed, got %d dials", dials)
	}
}
