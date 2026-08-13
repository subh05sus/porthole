// Package kill implements the OS-agnostic kill ladder: verify the target is
// still the process we scanned, signal it, wait, and escalate. The ladder
// itself has no OS-specific code — it drives a small Signaler primitive that
// kill_unix.go and kill_windows.go implement, which is what makes the ladder
// (including the PID-reuse guard) fully unit-testable on any platform.
package kill

import (
	"context"
	"errors"
	"time"
)

// ErrPIDReused means the PID scanned earlier now belongs to a different
// process (start time no longer matches). The caller must abort and refresh
// rather than signal the wrong process.
var ErrPIDReused = errors.New("process changed since scan, refreshed")

// ErrNotOwned means the current user lacks permission to signal this
// process. The caller must not attempt a syscall that will only fail.
var ErrNotOwned = errors.New("needs elevated permissions")

// Target identifies a process to kill, captured at scan time. StartTime is
// the opaque scan.Service.StartTime value re-checked immediately before
// every signal to detect PID reuse.
type Target struct {
	PID       int
	StartTime uint64
	Owned     bool
}

// Status describes the outcome of a ladder step.
type Status int

const (
	// StatusKilled means the process was confirmed dead.
	StatusKilled Status = iota
	// StatusAlreadyDead means the process no longer existed when checked.
	StatusAlreadyDead
	// StatusNeedsEscalation means a graceful signal was sent but the
	// process is still alive after the poll timeout. The caller decides
	// whether to call Escalate (e.g. after a user confirms force-kill).
	StatusNeedsEscalation
)

// Result reports what happened after a ladder step.
type Result struct {
	Status Status
}

// Options tunes the ladder's behavior.
type Options struct {
	// Force skips the graceful signal and force-kills immediately.
	Force bool
	// AutoEscalate force-kills automatically if the graceful signal is
	// ignored past PollTimeout, instead of returning StatusNeedsEscalation
	// for the caller to decide. Used by the CLI's "escalate after 2s"
	// non-interactive behavior; the TUI leaves this false and prompts.
	AutoEscalate bool
	// PollInterval is how often liveness is rechecked. Defaults to 100ms.
	PollInterval time.Duration
	// PollTimeout is how long to wait for a graceful exit before
	// escalating. Defaults to 2s.
	PollTimeout time.Duration
}

func (o Options) withDefaults() Options {
	if o.PollInterval <= 0 {
		o.PollInterval = 100 * time.Millisecond
	}
	if o.PollTimeout <= 0 {
		o.PollTimeout = 2 * time.Second
	}
	return o
}

// Signaler is the OS-specific primitive the ladder drives.
type Signaler interface {
	// StillAlive reports whether pid is currently running and, if so, its
	// current start-time value for reuse comparison.
	StillAlive(pid int) (startTime uint64, alive bool, err error)
	// Terminate sends a graceful termination request (SIGTERM on Unix;
	// on Windows there is no distinct graceful signal, so this and Kill
	// both call TerminateProcess — see kill_windows.go).
	Terminate(pid int) error
	// Kill sends a forceful termination request (SIGKILL on Unix,
	// TerminateProcess on Windows).
	Kill(pid int) error
}

// Killer runs the kill ladder against a Target.
type Killer interface {
	// Execute verifies the target hasn't been recycled, signals it
	// (gracefully unless Force), and waits up to PollTimeout for it to
	// die. See Status for what the returned Result means.
	Execute(ctx context.Context, target Target, opts Options) (Result, error)
	// Escalate force-kills a target that Execute returned
	// StatusNeedsEscalation for, re-verifying start time first since more
	// time has passed since the scan.
	Escalate(ctx context.Context, target Target) (Result, error)
}

var _ Killer = (*Ladder)(nil)

// Ladder implements Killer by driving a Signaler through the algorithm
// described in PRD §7.4. It has no OS-specific logic of its own.
type Ladder struct {
	Signaler Signaler

	// EscalatePollInterval/EscalatePollTimeout tune Escalate's wait for a
	// force-kill to take effect. Zero values default to 100ms/2s.
	EscalatePollInterval time.Duration
	EscalatePollTimeout  time.Duration
}

func (l *Ladder) Execute(ctx context.Context, target Target, opts Options) (Result, error) {
	opts = opts.withDefaults()

	res, err := l.verifyAndSignal(target, opts.Force)
	if err != nil {
		return Result{}, err
	}
	if res.Status == StatusAlreadyDead {
		return res, nil
	}

	dead, err := l.pollUntilDead(ctx, target.PID, opts.PollInterval, opts.PollTimeout)
	if err != nil {
		return Result{}, err
	}
	if dead {
		return Result{Status: StatusKilled}, nil
	}
	if opts.Force {
		// Already sent a force kill and it still didn't die — nothing
		// further to escalate to.
		return Result{Status: StatusNeedsEscalation}, nil
	}
	if opts.AutoEscalate {
		return l.Escalate(ctx, target)
	}
	return Result{Status: StatusNeedsEscalation}, nil
}

func (l *Ladder) Escalate(ctx context.Context, target Target) (Result, error) {
	res, err := l.verifyAndSignal(target, true)
	if err != nil {
		return Result{}, err
	}
	if res.Status == StatusAlreadyDead {
		return res, nil
	}

	interval, timeout := l.EscalatePollInterval, l.EscalatePollTimeout
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	dead, err := l.pollUntilDead(ctx, target.PID, interval, timeout)
	if err != nil {
		return Result{}, err
	}
	if dead {
		return Result{Status: StatusKilled}, nil
	}
	return Result{Status: StatusNeedsEscalation}, nil
}

// verifyAndSignal re-checks the target is still the process we scanned,
// then sends the requested signal. This is the single choke point every
// signal passes through, so the PID-reuse guard can never be bypassed.
func (l *Ladder) verifyAndSignal(target Target, force bool) (Result, error) {
	if !target.Owned {
		return Result{}, ErrNotOwned
	}

	startTime, alive, err := l.Signaler.StillAlive(target.PID)
	if err != nil {
		return Result{}, err
	}
	if !alive {
		return Result{Status: StatusAlreadyDead}, nil
	}
	if startTime != target.StartTime {
		return Result{}, ErrPIDReused
	}

	if force {
		err = l.Signaler.Kill(target.PID)
	} else {
		err = l.Signaler.Terminate(target.PID)
	}
	if err != nil {
		return Result{}, err
	}
	return Result{}, nil
}

func (l *Ladder) pollUntilDead(ctx context.Context, pid int, interval, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		_, alive, err := l.Signaler.StillAlive(pid)
		if err != nil {
			return false, err
		}
		if !alive {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ticker.C:
		}
	}
}
