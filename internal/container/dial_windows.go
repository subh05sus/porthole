//go:build windows

package container

import (
	"context"
	"fmt"
	"net"

	"github.com/Microsoft/go-winio"
)

// candidatePipes are tried in order. Modern Docker Desktop's default
// context (WSL2 backend, "desktop-linux") actually serves the Engine API
// on \\.\pipe\dockerDesktopLinuxEngine, not the classically-documented
// \\.\pipe\docker_engine — confirmed live against a real Docker Desktop
// 4.60 install, where both happened to be reachable, but only the first
// is guaranteed present on every install (docker_engine is the legacy/
// Windows-containers-mode pipe). Podman Desktop's Windows pipe name isn't
// covered — not installed in this environment to verify against.
var candidatePipes = []string{
	`\\.\pipe\dockerDesktopLinuxEngine`,
	`\\.\pipe\docker_engine`,
}

// NewDefaultClient dials each candidate named pipe in turn via go-winio
// (stdlib net.Dial doesn't support Windows named pipes), returning a
// client bound to the first one that connects.
func NewDefaultClient(ctx context.Context) (*Client, error) {
	var lastErr error
	for _, pipe := range candidatePipes {
		conn, err := winio.DialPipeContext(ctx, pipe)
		if err != nil {
			lastErr = err
			continue
		}
		conn.Close()

		p := pipe
		return newClientWithDialer(func(ctx context.Context) (net.Conn, error) {
			return winio.DialPipeContext(ctx, p)
		}), nil
	}
	return nil, fmt.Errorf("container: no docker-compatible named pipe reachable: %w", lastErr)
}
