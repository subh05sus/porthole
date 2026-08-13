// Package scan discovers listening network sockets and the processes behind them.
package scan

import "context"

// Proto identifies the socket protocol/family a Service was discovered on.
type Proto string

const (
	ProtoTCP  Proto = "tcp"
	ProtoTCP6 Proto = "tcp6"
	ProtoUDP  Proto = "udp"
)

// Service describes one listening socket and everything known about the
// process behind it. A Service is a point-in-time snapshot: fields like PID
// and StartTime must be re-verified before acting on them (see StartTime).
type Service struct {
	Port    int
	Proto   Proto
	Addr    string
	PID     int
	Process string
	Cmdline string
	User    string
	CWD     string

	// Project is the resolved project name owning this service, or "" if
	// unresolved. Populated by enrich.go, not by the raw OS scanners.
	Project string

	// StartTime is an opaque, OS-specific process start-time value (jiffies
	// since boot on Linux, a FILETIME on Windows, a lstart timestamp on
	// macOS). It has no meaning across platforms and no meaning as a wall
	// clock time — its only purpose is equality comparison against a fresh
	// read of the same PID immediately before signaling it, to detect PID
	// reuse between scan time and kill time.
	StartTime uint64

	// Owned reports whether the current user has permission to signal this
	// process. When false, ResolveErr explains why, and the kill path must
	// refuse rather than attempt a syscall that will fail anyway.
	Owned bool

	// ResolveErr explains why any of the above fields could not be fully
	// resolved (e.g. permission denied walking another user's process). A
	// Service is never dropped because of a resolution failure — it is
	// rendered with whatever was learned, plus this explanation.
	ResolveErr error
}

// Lister discovers the currently listening services on this host. TUI, CLI,
// and JSON renderers all consume []Service and nothing else — Lister is the
// one seam between platform-specific discovery and everything above it.
type Lister interface {
	List(ctx context.Context) ([]Service, error)
}
