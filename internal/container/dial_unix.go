//go:build linux || darwin

package container

import (
	"context"
	"fmt"
	"net"
	"os"
)

// candidateSockets are tried in order — Docker Desktop/Engine first, then
// Podman, then OrbStack, then Colima's default profile. Only Docker
// Desktop is actually reachable in this dev environment; the Podman/
// OrbStack/Colima paths are best-effort and untestable here — flagged
// honestly in TODO.md rather than claimed as verified.
func candidateSockets() []string {
	home, _ := os.UserHomeDir()
	return []string{
		"/var/run/docker.sock",
		"/run/podman/podman.sock",
		home + "/.orbstack/run/docker.sock",
		home + "/.colima/default/docker.sock",
	}
}

// NewDefaultClient probes each candidate socket in turn, returning a
// client bound to the first one that accepts a connection. Callers should
// treat a returned error as "no container runtime available" and degrade
// gracefully — container awareness is enrichment, never a hard requirement
// for porthole's core scan/kill loop.
func NewDefaultClient(ctx context.Context) (*Client, error) {
	var lastErr error
	for _, path := range candidateSockets() {
		conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
		if err != nil {
			lastErr = err
			continue
		}
		conn.Close()

		sockPath := path
		return newClientWithDialer(func(ctx context.Context) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
		}), nil
	}
	return nil, fmt.Errorf("container: no docker-compatible socket reachable: %w", lastErr)
}
