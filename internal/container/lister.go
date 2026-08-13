package container

import (
	"context"
	"sync"

	"github.com/subh05sus/porthole/internal/scan"
)

// listerBackend is the subset of *Client's behavior AwareLister needs —
// letting tests substitute a fake without a real daemon or an httptest
// server standing in for one.
type listerBackend interface {
	List(ctx context.Context) ([]Container, error)
}

// AwareLister decorates a scan.Lister, best-effort enriching every scan
// with container info (see EnrichServices). Docker/Podman/etc. being
// unreachable is never an error here — container awareness sits on top of
// the core scan/kill loop, not a requirement for it, so a failed dial or
// query just leaves services un-enriched, same as any other Lister call.
//
// The dialed client is cached across calls rather than redialing on every
// scan (watch mode polls every couple of seconds) — see getBackend.
type AwareLister struct {
	inner scan.Lister
	dial  func(ctx context.Context) (listerBackend, error)

	mu      sync.Mutex
	backend listerBackend
}

// NewAwareLister wraps inner with container enrichment using the real
// platform-default Docker Engine API client.
func NewAwareLister(inner scan.Lister) *AwareLister {
	return &AwareLister{
		inner: inner,
		dial: func(ctx context.Context) (listerBackend, error) {
			return NewDefaultClient(ctx)
		},
	}
}

func (l *AwareLister) List(ctx context.Context) ([]scan.Service, error) {
	services, err := l.inner.List(ctx)
	if err != nil {
		return services, err
	}

	backend := l.getBackend(ctx)
	if backend == nil {
		return services, nil
	}

	containers, err := backend.List(ctx)
	if err != nil {
		// The cached client may have gone stale (daemon restarted) —
		// drop it so the next call redials instead of failing the same
		// way forever.
		l.mu.Lock()
		l.backend = nil
		l.mu.Unlock()
		return services, nil
	}

	return EnrichServices(services, containers), nil
}

func (l *AwareLister) getBackend(ctx context.Context) listerBackend {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.backend != nil {
		return l.backend
	}
	backend, err := l.dial(ctx)
	if err != nil {
		return nil
	}
	l.backend = backend
	return l.backend
}
