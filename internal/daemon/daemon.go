// Package daemon implements `porthole daemon` — an auto-kill daemon that
// watches for services matching a user-authored allow-list and kills
// them. This is the single highest-risk feature in porthole: an
// unsupervised process that kills things is exactly the "killed Postgres
// at 2am" story PRD.md opens with, just automated. Every safety rail
// below exists specifically to keep the blast radius of a bug small:
//
//   - Disabled by default (config.AutoKill.Enabled), even with allow
//     entries present.
//   - Dry-run by default even when enabled — real kills require the
//     caller to explicitly set Live, which cli/daemon.go only ever does
//     via an explicit --live flag, never a config-file toggle or
//     anything reachable from the TUI.
//   - Allow-list entries are exact (port, process-name) pairs only —
//     never a wildcard, range, or pattern.
//   - Every match is re-verified against a fresh scan immediately before
//     acting, not just trusted from the poll that found it — closing the
//     gap where a port briefly matches an allow-listed name but the
//     underlying process changed in between (the same PID-reuse class of
//     risk PRD §8.1 already worries about, now with no human in the loop
//     to catch it).
//   - Every kill still funnels through the exact same kill.Killer every
//     other entry point uses, PID-reuse guard included — no separate,
//     less-audited kill path.
//   - Rate-limited: a cooldown per port after any auto-kill attempt (live
//     or dry-run), so a crash-looping process that keeps rebinding the
//     same port doesn't get killed in a tight loop.
package daemon

import (
	"context"
	"time"

	"github.com/subh05sus/porthole/internal/config"
	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/scan"
)

// cooldown is how long a port is skipped after any auto-kill attempt.
const cooldown = 30 * time.Second

// Action describes what the daemon did, or would do, for one matched
// service during a single poll cycle.
type Action struct {
	Service scan.Service
	// Live is true if this was a real kill attempt, false if this was a
	// dry-run/notify-only match (Result/Err are zero in that case).
	Live       bool
	Skipped    bool
	SkipReason string // "cooldown", "reverify mismatch", "reverify scan failed: ...", or ""
	Result     kill.Result
	Err        error
}

// Event is sent after each completed poll cycle.
type Event struct {
	Actions []Action
	// ScanErr is set if the poll's own scan failed — the whole cycle is
	// abandoned in that case (no partial action list), same "don't act on
	// a scan we don't trust" caution scan.Watch uses.
	ScanErr error
}

// Daemon polls Lister for services matching Config's auto-kill allow-list.
// Construct directly (all fields required except Live, which defaults to
// dry-run/false).
type Daemon struct {
	Lister scan.Lister
	Killer kill.Killer
	Config config.Config
	Live   bool

	cooldownUntil map[int]time.Time
}

// Poll runs exactly one cycle. See the package doc for the full safety
// rail list this implements.
func (d *Daemon) Poll(ctx context.Context) Event {
	if d.cooldownUntil == nil {
		d.cooldownUntil = make(map[int]time.Time)
	}
	if !d.Config.AutoKill.Enabled {
		return Event{}
	}

	services, err := d.Lister.List(ctx)
	if err != nil {
		return Event{ScanErr: err}
	}

	now := time.Now()
	var actions []Action
	for _, s := range services {
		if !d.Config.IsAutoKillAllowed(s.Port, s.Process) {
			continue
		}

		if until, ok := d.cooldownUntil[s.Port]; ok && now.Before(until) {
			actions = append(actions, Action{Service: s, Skipped: true, SkipReason: "cooldown"})
			continue
		}

		fresh, ferr := d.Lister.List(ctx)
		if ferr != nil {
			actions = append(actions, Action{Service: s, Skipped: true, SkipReason: "reverify scan failed: " + ferr.Error()})
			continue
		}
		if !stillMatches(fresh, s) {
			actions = append(actions, Action{Service: s, Skipped: true, SkipReason: "reverify mismatch"})
			continue
		}

		d.cooldownUntil[s.Port] = now.Add(cooldown)

		if !d.Live {
			actions = append(actions, Action{Service: s, Live: false})
			continue
		}

		target := kill.Target{PID: s.PID, StartTime: s.StartTime, Owned: s.Owned, ContainerID: s.ContainerID}
		res, kerr := d.Killer.Execute(ctx, target, kill.Options{AutoEscalate: true})
		actions = append(actions, Action{Service: s, Live: true, Result: res, Err: kerr})
	}

	return Event{Actions: actions}
}

// stillMatches reports whether want's exact (port, pid, starttime)
// identity is still present in fresh — not just "something is still
// listening on that port," but specifically the same process instance
// the original poll saw.
func stillMatches(fresh []scan.Service, want scan.Service) bool {
	for _, s := range fresh {
		if s.Port == want.Port && s.PID == want.PID && s.StartTime == want.StartTime {
			return true
		}
	}
	return false
}

// Run polls at interval, sending an Event after each cycle, until ctx is
// done. Mirrors scan.Watch's exact shape, including the same "ticks lets
// tests drive this deterministically instead of a real timer" seam.
func (d *Daemon) Run(ctx context.Context, interval time.Duration, ticks <-chan time.Time) <-chan Event {
	out := make(chan Event)

	go func() {
		defer close(out)

		if ticks == nil {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			ticks = ticker.C
		}

		runOnce := func() {
			ev := d.Poll(ctx)
			select {
			case out <- ev:
			case <-ctx.Done():
			}
		}

		runOnce()
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-ticks:
				if !ok {
					return
				}
				runOnce()
			}
		}
	}()

	return out
}
