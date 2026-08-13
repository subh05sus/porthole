package container

import (
	"strings"

	"github.com/subh05sus/porthole/internal/scan"
)

// EnrichServices cross-references discovered services against a list of
// running containers' published ports, filling in Service.Container/
// ContainerImage/ContainerID wherever a service's port matches a
// container's public port mapping.
//
// Matching is done purely on (published port, protocol) rather than by
// checking whether the owning process is "docker-proxy" — the actual
// forwarding process differs across platforms and backends (a per-port
// docker-proxy process on Linux's classic bridge network vs. a single
// shared VM/WSL2 backend process on macOS/Windows Docker Desktop), so
// matching on the port the container actually asked to publish is both
// simpler and correct across all of them.
func EnrichServices(services []scan.Service, containers []Container) []scan.Service {
	type key struct {
		port  int
		proto string // "tcp" or "udp", matching Container.Port.Type
	}
	byPort := make(map[key]Container, len(containers))
	for _, c := range containers {
		for _, p := range c.Ports {
			if p.PublicPort == 0 {
				continue // container-internal port, never published to the host
			}
			byPort[key{port: p.PublicPort, proto: p.Type}] = c
		}
	}

	for i := range services {
		c, ok := byPort[key{port: services[i].Port, proto: baseProto(services[i].Proto)}]
		if !ok {
			continue
		}
		services[i].Container = containerDisplayName(c)
		services[i].ContainerImage = c.Image
		services[i].ContainerID = c.ID
	}
	return services
}

// baseProto strips the v6 suffix scan.Proto uses (tcp6/udp6) down to the
// plain "tcp"/"udp" the Engine API reports port types as — a container's
// published port isn't reported separately per address family.
func baseProto(p scan.Proto) string {
	return strings.TrimSuffix(string(p), "6")
}

// containerDisplayName returns the container's first name with Docker's
// leading "/" stripped, or its short ID if it somehow has no name.
func containerDisplayName(c Container) string {
	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	if len(c.ID) >= 12 {
		return c.ID[:12]
	}
	return c.ID
}
