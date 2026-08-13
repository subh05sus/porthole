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

	// StartTime is an opaque, OS-specific value with the same contract as
	// scan.Service.StartTime — comparable, not portable, not a wall clock.
	StartTime uint64
	Uptime    time.Duration
}

// Lookup resolves per-process metadata for a PID.
type Lookup interface {
	Lookup(pid int) (Info, error)
}
