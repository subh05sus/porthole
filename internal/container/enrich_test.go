package container

import (
	"testing"

	"github.com/subh05sus/porthole/internal/scan"
)

func TestEnrichServicesMatchesByPublicPortAndProto(t *testing.T) {
	services := []scan.Service{
		{Port: 5434, Proto: scan.ProtoTCP, Process: "com.docker.backend"},
		{Port: 5434, Proto: scan.ProtoTCP6, Process: "com.docker.backend"},
		{Port: 9000, Proto: scan.ProtoUDP, Process: "unrelated"},
	}
	containers := []Container{
		{
			ID:    "abc123def456",
			Names: []string{"/monkpayments-db-1"},
			Image: "pgvector/pgvector:pg16",
			Ports: []Port{{PrivatePort: 5432, PublicPort: 5434, Type: "tcp"}},
		},
	}

	got := EnrichServices(services, containers)

	if got[0].Container != "monkpayments-db-1" || got[0].ContainerImage != "pgvector/pgvector:pg16" || got[0].ContainerID != "abc123def456" {
		t.Fatalf("tcp4 service not enriched correctly: %+v", got[0])
	}
	if got[1].Container != "monkpayments-db-1" {
		t.Fatalf("tcp6 service (same port, base proto tcp) should also match: %+v", got[1])
	}
	if got[2].Container != "" {
		t.Fatalf("unrelated udp service on a different port should not be enriched: %+v", got[2])
	}
}

func TestEnrichServicesProtocolMismatchDoesNotMatch(t *testing.T) {
	services := []scan.Service{{Port: 53, Proto: scan.ProtoTCP}}
	containers := []Container{
		{ID: "x", Names: []string{"/dns"}, Ports: []Port{{PublicPort: 53, Type: "udp"}}},
	}

	got := EnrichServices(services, containers)
	if got[0].Container != "" {
		t.Fatalf("a udp-published port should not enrich a tcp service on the same port number: %+v", got[0])
	}
}

func TestEnrichServicesSkipsUnpublishedPorts(t *testing.T) {
	services := []scan.Service{{Port: 5432, Proto: scan.ProtoTCP}}
	containers := []Container{
		{ID: "x", Names: []string{"/db"}, Ports: []Port{{PrivatePort: 5432, PublicPort: 0, Type: "tcp"}}},
	}

	got := EnrichServices(services, containers)
	if got[0].Container != "" {
		t.Fatalf("a container-internal-only port (PublicPort 0) must not enrich anything: %+v", got[0])
	}
}

func TestContainerDisplayNameFallsBackToShortID(t *testing.T) {
	c := Container{ID: "abcdef0123456789"}
	if got := containerDisplayName(c); got != "abcdef012345" {
		t.Fatalf("got %q, want short ID fallback", got)
	}
}
