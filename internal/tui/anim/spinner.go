package anim

import "time"

// BrailleFrames is the spinner sequence PRD §5.3 calls for ("uses a braille
// spinner during work"). Driven manually from the model's single central
// tick rather than a bubbles/spinner.Model, which would schedule its own
// independent tea.Tick loop — violating the "one tea.Tick, no stacking
// timers" animation budget.
var BrailleFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// DefaultSpinnerFrameInterval is how long each frame is shown.
const DefaultSpinnerFrameInterval = 80 * time.Millisecond

// SpinnerFrame returns which braille frame to show at a given elapsed time.
func SpinnerFrame(elapsed time.Duration) rune {
	if elapsed < 0 {
		elapsed = 0
	}
	idx := int(elapsed/DefaultSpinnerFrameInterval) % len(BrailleFrames)
	return BrailleFrames[idx]
}
