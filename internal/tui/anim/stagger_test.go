package anim

import (
	"testing"
	"time"
)

func TestRevealedFirstRowIsImmediate(t *testing.T) {
	if !Revealed(0, 0) {
		t.Fatalf("row 0 should be revealed at elapsed=0")
	}
}

func TestRevealedRespectsStaggerInterval(t *testing.T) {
	if Revealed(1, StaggerInterval-time.Millisecond) {
		t.Fatalf("row 1 should not be revealed just before its stagger delay")
	}
	if !Revealed(1, StaggerInterval) {
		t.Fatalf("row 1 should be revealed exactly at its stagger delay")
	}
}

func TestRevealDelayCapsAt400ms(t *testing.T) {
	// A very large index would naively compute a multi-second delay; it
	// must be clamped to RevealCap.
	if got := RevealDelay(1000); got != RevealCap {
		t.Fatalf("RevealDelay(1000) = %v, want %v", got, RevealCap)
	}
}

func TestRevealedAtCapRevealsEverything(t *testing.T) {
	if !Revealed(1000, RevealCap) {
		t.Fatalf("even a far-out row must be revealed once elapsed reaches RevealCap")
	}
}

func TestRevealDelayIsMonotonicWithinCap(t *testing.T) {
	prev := RevealDelay(0)
	for i := 1; i < 20; i++ {
		d := RevealDelay(i)
		if d < prev {
			t.Fatalf("RevealDelay(%d)=%v is less than RevealDelay(%d)=%v", i, d, i-1, prev)
		}
		prev = d
	}
}
