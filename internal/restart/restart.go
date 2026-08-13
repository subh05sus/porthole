// Package restart implements PRD's v1.2 feature (FUTURE_PLANS.md): kill a
// process and immediately respawn it with the same command line, working
// directory, and environment.
package restart

import (
	"fmt"

	"github.com/subh05sus/porthole/internal/proc"
	"github.com/subh05sus/porthole/internal/scan"
)

// Plan captures what's needed to respawn a process, taken from a fresh
// proc.Lookup immediately before the target is killed — not from the
// scan.Service that located it, since Service.Cmdline is already a
// display-joined string, not a real argv[].
type Plan struct {
	Process string   // for status messages only
	ExePath string   // Windows: full path, used as the real application path
	Cmdline string   // Windows: exact raw command line, used verbatim
	Argv    []string // Unix: argv[0..]
	CWD     string
	Env     []string // always non-nil when Capture succeeds — see below
}

// Capture builds a Plan for target's PID, or returns an error if the
// process's environment can't be reliably captured (e.g. macOS without
// root) — per this project's own stated principle (FUTURE_PLANS.md v1.2):
// restart ships only when it can do it correctly, or refuse honestly,
// never by respawning with a silently incomplete environment.
func Capture(lookup proc.Lookup, target scan.Service) (Plan, error) {
	info, err := lookup.Lookup(target.PID)
	if err != nil {
		return Plan{}, fmt.Errorf("restart: resolving process info: %w", err)
	}
	if info.EnvErr != nil {
		return Plan{}, fmt.Errorf("restart: %w", info.EnvErr)
	}
	if info.Cmdline == "" && len(info.Argv) == 0 {
		return Plan{}, fmt.Errorf("restart: no command line available to respawn %s (pid %d) with", target.Process, target.PID)
	}

	env := info.Env
	if env == nil {
		// Distinguish "genuinely empty environment" (valid, respawn with
		// no env vars) from "nil because uninitialized" (would make
		// exec.Cmd silently inherit porthole's own environment instead).
		env = []string{}
	}

	return Plan{
		Process: target.Process,
		ExePath: info.ExePath,
		Cmdline: info.Cmdline,
		Argv:    info.Argv,
		CWD:     info.CWD,
		Env:     env,
	}, nil
}

// Spawner respawns a captured Plan as a new, detached process (so it
// survives porthole exiting).
type Spawner interface {
	Spawn(plan Plan) error
}

type defaultSpawner struct{}

// NewDefaultSpawner returns the platform's real Spawner.
func NewDefaultSpawner() Spawner { return defaultSpawner{} }

func (defaultSpawner) Spawn(plan Plan) error { return spawn(plan) }
