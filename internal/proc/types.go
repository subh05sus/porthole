// Package proc resolves per-process metadata (name, cmdline, cwd, user,
// start time) for a PID discovered by internal/scan.
package proc

import "time"

// Info is what we know about a process beyond its listening sockets.
type Info struct {
	Process string
	Cmdline string
	User    string
	CWD     string
	// ExePath is the full path to the executable, when resolvable —
	// distinct from Process (which is just the base name, for display).
	// Used by internal/restart to respawn on Windows, which needs a real
	// application path rather than relying on PATH lookup.
	ExePath string

	// StartTime is an opaque, OS-specific value with the same contract as
	// scan.Service.StartTime — comparable, not portable, not a wall clock.
	StartTime uint64
	Uptime    time.Duration

	// Argv is the exact argv[0..] for Unix-style respawn (exec.Command
	// on Linux/Darwin uses this directly). Nil on Windows, which has no
	// native argv[] — Windows respawn instead uses the raw Cmdline string
	// above directly, to avoid Go's own re-quoting differing from the
	// original invocation.
	Argv []string

	// Env is the process's environment as "KEY=VALUE" strings, used only
	// by internal/restart. Nil with EnvErr set when it can't be reliably
	// read (e.g. macOS without root) — per this project's "do it correctly
	// or refuse honestly" principle, restart must never respawn with a
	// silently incomplete environment, so EnvErr being non-nil is a hard
	// stop for that one feature, not a soft degrade like the other fields.
	Env    []string
	EnvErr error
}

// Lookup resolves per-process metadata for a PID.
type Lookup interface {
	Lookup(pid int) (Info, error)
}

// ResourceStats is a point-in-time CPU/memory reading for one process.
type ResourceStats struct {
	// CPUPercent follows ps/htop's convention, not Task Manager's: percent
	// of one CPU core, unnormalized by core count, so a saturated
	// multi-threaded process can read above 100. Consistent across all
	// three platforms rather than picking Windows' different default.
	CPUPercent float64
	RSSBytes   uint64
}

// ResourceQuerier is an optional capability a Lookup may additionally
// implement: an on-demand CPU%/RSS reading for one PID, for the TUI detail
// pane (FUTURE_PLANS.md's resource-stats idea). Deliberately not part of
// the regular scan/Lookup path — CPU% needs a short delta sample (two
// reads a brief interval apart), which isn't something every routine scan
// should pay for.
type ResourceQuerier interface {
	Resources(pid int) (ResourceStats, error)
}
