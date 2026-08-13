package anim

import (
	"testing"
	"time"
)

func TestFadeOutStageProgression(t *testing.T) {
	total := 400 * time.Millisecond

	if got := FadeOutStage(0, total); got != 0 {
		t.Errorf("FadeOutStage(0, total) = %d, want 0", got)
	}
	if got := FadeOutStage(total, total); got != FadeStages {
		t.Errorf("FadeOutStage(total, total) = %d, want %d", got, FadeStages)
	}
	if got := FadeOutStage(total*2, total); got != FadeStages {
		t.Errorf("FadeOutStage(2*total, total) = %d, want %d (must not overshoot)", got, FadeStages)
	}

	mid := FadeOutStage(total/2, total)
	if mid <= 0 || mid >= FadeStages {
		t.Errorf("FadeOutStage(total/2, total) = %d, want a middle stage strictly between 0 and %d", mid, FadeStages)
	}
}

func TestFadeOutStageMonotonic(t *testing.T) {
	total := 400 * time.Millisecond
	prev := 0
	for ms := 0; ms <= 400; ms += 10 {
		stage := FadeOutStage(time.Duration(ms)*time.Millisecond, total)
		if stage < prev {
			t.Fatalf("FadeOutStage regressed at %dms: %d < %d", ms, stage, prev)
		}
		prev = stage
	}
}

func TestFadeOutCompleteAtOrPastTotal(t *testing.T) {
	total := 400 * time.Millisecond
	if FadeOutComplete(total-time.Millisecond, total) {
		t.Fatalf("must not be complete just before total elapses")
	}
	if !FadeOutComplete(total, total) {
		t.Fatalf("must be complete exactly at total")
	}
}

func TestFadeOutStageZeroTotalIsImmediatelyComplete(t *testing.T) {
	if got := FadeOutStage(0, 0); got != FadeStages {
		t.Fatalf("FadeOutStage(0, 0) = %d, want %d (degenerate zero-duration fade)", got, FadeStages)
	}
}
