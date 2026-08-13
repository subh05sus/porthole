package container

import (
	"context"
	"errors"
	"testing"

	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/kill/killtest"
)

type fakeStopper struct {
	stoppedIDs []string
	err        error
}

func (f *fakeStopper) Stop(ctx context.Context, id string) error {
	f.stoppedIDs = append(f.stoppedIDs, id)
	return f.err
}

func newAwareKiller(inner kill.Killer, backend stopper, dialErr error) *AwareKiller {
	return &AwareKiller{
		Inner: inner,
		dial: func(ctx context.Context) (stopper, error) {
			if dialErr != nil {
				return nil, dialErr
			}
			return backend, nil
		},
	}
}

func TestAwareKillerRoutesContainerTargetsToStop(t *testing.T) {
	inner := &killtest.FakeKiller{}
	backend := &fakeStopper{}
	k := newAwareKiller(inner, backend, nil)

	res, err := k.Execute(context.Background(), kill.Target{PID: 1234, ContainerID: "abc123"}, kill.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != kill.StatusKilled {
		t.Fatalf("expected StatusKilled, got %v", res.Status)
	}
	if len(backend.stoppedIDs) != 1 || backend.stoppedIDs[0] != "abc123" {
		t.Fatalf("expected Stop called with the container ID, got %+v", backend.stoppedIDs)
	}
	if len(inner.ExecuteCalls) != 0 {
		t.Fatalf("expected the PID-based ladder to never be called for a container target, got %+v", inner.ExecuteCalls)
	}
}

func TestAwareKillerFallsThroughForPlainTargets(t *testing.T) {
	inner := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	backend := &fakeStopper{}
	k := newAwareKiller(inner, backend, nil)

	res, err := k.Execute(context.Background(), kill.Target{PID: 1234}, kill.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != kill.StatusKilled {
		t.Fatalf("got %v", res.Status)
	}
	if len(backend.stoppedIDs) != 0 {
		t.Fatalf("expected Stop never called for a plain (non-container) target, got %+v", backend.stoppedIDs)
	}
	if len(inner.ExecuteCalls) != 1 {
		t.Fatalf("expected the inner ladder to handle a plain target, got %d calls", len(inner.ExecuteCalls))
	}
}

func TestAwareKillerErrorsWhenDaemonUnreachableForContainerTarget(t *testing.T) {
	inner := &killtest.FakeKiller{}
	k := newAwareKiller(inner, nil, errors.New("no docker socket"))

	_, err := k.Execute(context.Background(), kill.Target{ContainerID: "abc"}, kill.Options{})
	if err == nil {
		t.Fatalf("expected an error when the daemon is unreachable for a container-backed target")
	}
}

func TestAwareKillerEscalateReissuesStopForContainerTargets(t *testing.T) {
	inner := &killtest.FakeKiller{}
	backend := &fakeStopper{}
	k := newAwareKiller(inner, backend, nil)

	res, err := k.Escalate(context.Background(), kill.Target{ContainerID: "abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != kill.StatusKilled {
		t.Fatalf("got %v", res.Status)
	}
	if len(inner.EscalateCalls) != 0 {
		t.Fatalf("expected the PID-based ladder's Escalate to never be called for a container target")
	}
}
