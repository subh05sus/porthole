package scan

import (
	"context"
	"time"
)

// watchScanTimeout bounds every individual scan in watch mode, per PRD
// §8.3's "every scan carries a context.WithTimeout of 3 seconds" — the
// same budget the TUI's own scans use (see tui.scanTimeout).
const watchScanTimeout = 3 * time.Second

// Diff describes what changed between two consecutive scans.
type Diff struct {
	Added   []Service
	Removed []Service
}

// identity uniquely identifies a service instance for diffing: Port+PID+
// StartTime, so a PID recycled to a different process (PRD §8.1) is
// correctly treated as one service disappearing and a different one
// appearing, never as "unchanged."
type identity struct {
	port      int
	pid       int
	startTime uint64
}

func serviceIdentity(s Service) identity {
	return identity{port: s.Port, pid: s.PID, startTime: s.StartTime}
}

func diffServices(prev, next []Service) Diff {
	prevSet := make(map[identity]Service, len(prev))
	for _, s := range prev {
		prevSet[serviceIdentity(s)] = s
	}
	nextSet := make(map[identity]Service, len(next))
	for _, s := range next {
		nextSet[serviceIdentity(s)] = s
	}

	var d Diff
	for id, s := range nextSet {
		if _, ok := prevSet[id]; !ok {
			d.Added = append(d.Added, s)
		}
	}
	for id, s := range prevSet {
		if _, ok := nextSet[id]; !ok {
			d.Removed = append(d.Removed, s)
		}
	}
	return d
}

// Event is sent after each completed scan in watch mode.
type Event struct {
	Services []Service
	Diff     Diff
	Err      error
}

// Watch polls lister every interval, sending an Event after each completed
// scan, and closes the returned channel when ctx is done.
//
// Per PRD §8.3, overlapping scans are dropped rather than queued: scans run
// one at a time in a single goroutine, so if a scan takes longer than
// interval, ticks that land while it's still running are simply missed —
// this is time.Ticker's own built-in behavior for a slow receiver, not
// something this function tracks itself. A slow machine degrades to a
// lower effective refresh rate instead of building a backlog.
//
// ticks lets tests drive the polling loop deterministically instead of
// waiting on a real timer; pass nil to use a real time.Ticker at interval.
func Watch(ctx context.Context, lister Lister, interval time.Duration, ticks <-chan time.Time) <-chan Event {
	out := make(chan Event)

	go func() {
		defer close(out)

		if ticks == nil {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			ticks = ticker.C
		}

		var prev []Service
		runScan := func() {
			scanCtx, cancel := context.WithTimeout(ctx, watchScanTimeout)
			services, err := lister.List(scanCtx)
			cancel()

			var d Diff
			if err == nil {
				d = diffServices(prev, services)
				prev = services
			}
			select {
			case out <- Event{Services: services, Diff: d, Err: err}:
			case <-ctx.Done():
			}
		}

		runScan() // first scan immediately, don't wait for the first tick
		for {
			select {
			case <-ctx.Done():
				return
			case _, ok := <-ticks:
				if !ok {
					return
				}
				runScan()
			}
		}
	}()

	return out
}
