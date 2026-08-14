package cli

import "time"

// resolveInterval implements "an explicit CLI flag always wins, otherwise
// config sets the baseline default": flagValue already equals the flag's
// own hardcoded default whenever flagChanged is false (Cobra initializes
// the bound variable to the flag's default before RunE runs), so there's
// nothing else to fall back to once configValue is non-positive (unset,
// or a config that was never loaded — e.g. most tests construct App{}
// directly without going through config.Load).
func resolveInterval(flagChanged bool, flagValue, configValue time.Duration) time.Duration {
	if flagChanged || configValue <= 0 {
		return flagValue
	}
	return configValue
}
