package anim

import (
	"testing"
	"time"
)

func TestSpinnerFrameAdvancesOverTime(t *testing.T) {
	f0 := SpinnerFrame(0)
	f1 := SpinnerFrame(DefaultSpinnerFrameInterval)
	if f0 == f1 {
		t.Fatalf("expected a different frame after one frame interval, got %q both times", f0)
	}
	if f0 != BrailleFrames[0] {
		t.Fatalf("SpinnerFrame(0) = %q, want first frame %q", f0, BrailleFrames[0])
	}
}

func TestSpinnerFrameWrapsAround(t *testing.T) {
	full := DefaultSpinnerFrameInterval * time.Duration(len(BrailleFrames))
	if got := SpinnerFrame(full); got != BrailleFrames[0] {
		t.Fatalf("SpinnerFrame after a full cycle = %q, want wraparound to %q", got, BrailleFrames[0])
	}
}

func TestSpinnerFrameNegativeElapsedClampsToZero(t *testing.T) {
	if got := SpinnerFrame(-1); got != BrailleFrames[0] {
		t.Fatalf("SpinnerFrame(-1) = %q, want %q", got, BrailleFrames[0])
	}
}
