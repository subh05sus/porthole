// Package anim holds pure animation math for the TUI: no bubbletea
// dependency, no wall-clock reads of its own — every function takes
// elapsed time as a parameter, so it's fully deterministic and testable.
package anim

import "time"

// StaggerInterval is the gap between each row's reveal, and RevealCap is
// the longest the whole cascade is ever allowed to run, both per PRD §5.3:
// "staggered ~25ms apart... capped, never longer than 400ms total."
const (
	StaggerInterval = 25 * time.Millisecond
	RevealCap       = 400 * time.Millisecond
)

// Revealed reports whether the row at index should be visible yet, given
// elapsed time since the scan completed. The scanner itself is not
// streaming (Lister.List returns a fully resolved slice in one call), so
// this simulates the cascade by staggering *display* of an already-known
// list rather than tracking real per-row resolution times.
func Revealed(index int, elapsed time.Duration) bool {
	return elapsed >= RevealDelay(index)
}

// RevealDelay returns how long index's row waits before revealing, capped
// at RevealCap regardless of how large index is.
func RevealDelay(index int) time.Duration {
	delay := time.Duration(index) * StaggerInterval
	if delay > RevealCap {
		return RevealCap
	}
	return delay
}
