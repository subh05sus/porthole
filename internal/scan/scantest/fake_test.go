package scantest

import (
	"context"
	"errors"
	"testing"

	"github.com/subh05sus/porthole/internal/scan"
)

func TestFakeListerReturnsScriptedServices(t *testing.T) {
	f := &FakeLister{Services: []scan.Service{{Port: 3000, PID: 1}}}

	got, err := f.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Port != 3000 {
		t.Fatalf("got %+v, want one service on port 3000", got)
	}
}

func TestFakeListerReturnsScriptedError(t *testing.T) {
	wantErr := errors.New("boom")
	f := &FakeLister{Err: wantErr}

	_, err := f.List(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want %v", err, wantErr)
	}
}

func TestFakeListerRespectsContextCancellation(t *testing.T) {
	ready := make(chan struct{}) // never closed
	f := &FakeLister{Ready: ready}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := f.List(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestFakeListerMutationDoesNotAffectSource(t *testing.T) {
	f := &FakeLister{Services: []scan.Service{{Port: 3000}}}

	got, err := f.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got[0].Port = 9999

	if f.Services[0].Port != 3000 {
		t.Fatalf("FakeLister.Services was mutated by caller: %+v", f.Services)
	}
}
