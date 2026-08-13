// Package container talks to a local Docker-Engine-API-compatible daemon
// (Docker, Podman, OrbStack, Colima) to enrich scan results with container
// names/images and route kills through `docker stop` instead of signaling
// the docker-proxy PID directly. The Engine API is plain HTTP over a local
// Unix domain socket (or a Windows named pipe) — this implements only the
// two endpoints porthole needs rather than pulling in the full docker/docker
// SDK.
package container

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
)

// Client is a minimal Docker Engine API client bound to one already-located
// local socket/pipe. Construct via NewDefaultClient.
type Client struct {
	httpClient *http.Client
}

func newClientWithDialer(dial func(ctx context.Context) (net.Conn, error)) *Client {
	return &Client{
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return dial(ctx)
				},
			},
		},
	}
}

// Container is the subset of `GET /containers/json` fields porthole needs.
type Container struct {
	ID    string
	Names []string
	Image string
	Ports []Port
}

// Port is one published port mapping reported by the Engine API.
type Port struct {
	PrivatePort int
	PublicPort  int
	Type        string // "tcp" or "udp"
}

type containerJSON struct {
	ID    string `json:"Id"`
	Names []string
	Image string
	Ports []struct {
		PrivatePort int
		PublicPort  int
		Type        string
	}
}

// List returns every running container's published ports, via
// `GET /containers/json` (the default running-only filter — a stopped
// container can't be holding an open port, so there's nothing to enrich).
func (c *Client) List(ctx context.Context) ([]Container, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("container: docker engine unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("container: docker engine returned %s", resp.Status)
	}

	var raw []containerJSON
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("container: decoding response: %w", err)
	}

	out := make([]Container, 0, len(raw))
	for _, r := range raw {
		ports := make([]Port, 0, len(r.Ports))
		for _, p := range r.Ports {
			ports = append(ports, Port{PrivatePort: p.PrivatePort, PublicPort: p.PublicPort, Type: p.Type})
		}
		out = append(out, Container{ID: r.ID, Names: r.Names, Image: r.Image, Ports: ports})
	}
	return out, nil
}

// Stop stops a container by ID or name via `POST /containers/{id}/stop` —
// the kill path's routing target for a Service backed by a container,
// instead of signaling the docker-proxy PID directly.
func (c *Client) Stop(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://docker/containers/"+id+"/stop", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("container: docker engine unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotModified {
		return fmt.Errorf("container: stop returned %s", resp.Status)
	}
	return nil
}
