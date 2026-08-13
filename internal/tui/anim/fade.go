package anim

import "time"

// FadeStages is how many discrete steps a fade transition passes through.
// Terminals have no true alpha blending, so a "fade" here is an
// approximation via a small number of visually distinct stages rather than
// a continuous gradient.
const FadeStages = 4

// FadeOutStage returns which discrete stage a fade-out (e.g. a just-killed
// row dissolving, per PRD §5.2's "~400ms" spec) is in, given elapsed time
// since the fade began. 0 = just started (fully visible), FadeStages =
// finished (should be removed from the list).
func FadeOutStage(elapsed, total time.Duration) int {
	if total <= 0 || elapsed >= total {
		return FadeStages
	}
	if elapsed <= 0 {
		return 0
	}
	stage := int(elapsed * FadeStages / total)
	if stage > FadeStages {
		stage = FadeStages
	}
	return stage
}

// FadeOutComplete reports whether a fade-out has finished — the row should
// be dropped and the list allowed to reflow.
func FadeOutComplete(elapsed, total time.Duration) bool {
	return FadeOutStage(elapsed, total) >= FadeStages
}

// SlideInStage mirrors FadeOutStage for watch mode's "new services slide in
// from the bottom" (PRD §5.3), reusing the same discrete-stage model.
func SlideInStage(elapsed, total time.Duration) int {
	return FadeOutStage(elapsed, total)
}
