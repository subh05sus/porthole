package container

import (
	"context"
	"fmt"

	"github.com/subh05sus/porthole/internal/kill"
)

// stopper is the subset of *Client's behavior AwareKiller needs.
type stopper interface {
	Stop(ctx context.Context, id string) error
}

// AwareKiller decorates a kill.Killer: any Target with a non-empty
// ContainerID is routed to `docker stop <container>` via the Engine API
// instead of signaling the PID directly. The PID a container-backed
// Service reports belongs to the host-side forwarding process
// (com.docker.backend/docker-proxy/wslrelay depending on platform and
// which scan table found it first, see EnrichServices) — killing that
// PID wouldn't stop the container, and on Docker Desktop it's a shared
// backend process serving every container's forwarding, so killing it
// would take down every other container's ports along with it.
//
// Falls back to the inner Killer whenever ContainerID is empty (the
// common case — most Targets aren't container-backed) or the container
// runtime can't be reached, same "enrichment, not a requirement" degrade
// AwareLister uses; a Target with a ContainerID but an unreachable daemon
// returns an error rather than silently signaling the wrong PID, since
// unlike enrichment, a kill needs an explicit outcome one way or the
// other, not a silent fallback to the wrong action.
type AwareKiller struct {
	Inner kill.Killer
	dial  func(ctx context.Context) (stopper, error)
}

// NewAwareKiller wraps inner using the real platform-default Docker
// Engine API client.
func NewAwareKiller(inner kill.Killer) *AwareKiller {
	return &AwareKiller{
		Inner: inner,
		dial: func(ctx context.Context) (stopper, error) {
			return NewDefaultClient(ctx)
		},
	}
}

func (k *AwareKiller) Execute(ctx context.Context, target kill.Target, opts kill.Options) (kill.Result, error) {
	if target.ContainerID == "" {
		return k.Inner.Execute(ctx, target, opts)
	}
	return k.stopContainer(ctx, target.ContainerID)
}

// Escalate re-issues Stop for a container target: `docker stop` already
// sends SIGTERM then SIGKILL after its own grace period, so there's no
// separate "force" verb here the way the PID ladder has Kill vs Terminate.
func (k *AwareKiller) Escalate(ctx context.Context, target kill.Target) (kill.Result, error) {
	if target.ContainerID == "" {
		return k.Inner.Escalate(ctx, target)
	}
	return k.stopContainer(ctx, target.ContainerID)
}

func (k *AwareKiller) stopContainer(ctx context.Context, id string) (kill.Result, error) {
	client, err := k.dial(ctx)
	if err != nil {
		return kill.Result{}, fmt.Errorf("container: docker engine unreachable: %w", err)
	}
	if err := client.Stop(ctx, id); err != nil {
		return kill.Result{}, err
	}
	return kill.Result{Status: kill.StatusKilled}, nil
}
