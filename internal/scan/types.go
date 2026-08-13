// Package scan discovers listening network sockets and the processes behind them.
package scan

import (
	"context"
	"time"
)

// Proto identifies the socket protocol/family a Service was discovered on.
type Proto string

const (
	ProtoTCP  Proto = "tcp"
	ProtoTCP6 Proto = "tcp6"
	ProtoUDP  Proto = "udp"
	ProtoUDP6 Proto = "udp6"
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

	// Uptime is how long the process has been running as of scan time,
	// resolved from the OS's own wall-clock notion of process start (not
	// derived from StartTime, which is deliberately opaque). Display only.
	Uptime time.Duration

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

// SocketQuerier is an optional capability a Lister may additionally
// implement: an on-demand query for every socket a specific PID holds, not
// just the listening/bound ones a regular List() reports. It exists for the
// TUI detail pane's expanded socket list — a broader per-process query isn't
// worth paying for on every routine scan, so it's a separate call made only
// when the detail pane opens.
type SocketQuerier interface {
	SocketsForPID(ctx context.Context, pid int) ([]Service, error)
}
