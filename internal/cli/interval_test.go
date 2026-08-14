package cli

import (
	"testing"
	"time"
)

func TestResolveIntervalPrefersExplicitFlagOverConfig(t *testing.T) {
	got := resolveInterval(true, 3*time.Second, 10*time.Second)
	if got != 3*time.Second {
		t.Fatalf("got %s, want the explicit flag value (3s)", got)
	}
}

func TestResolveIntervalUsesConfigWhenFlagNotChanged(t *testing.T) {
	got := resolveInterval(false, 2*time.Second, 10*time.Second)
	if got != 10*time.Second {
		t.Fatalf("got %s, want the config value (10s)", got)
	}
}

func TestResolveIntervalFallsBackToFlagDefaultWhenConfigZero(t *testing.T) {
	got := resolveInterval(false, 2*time.Second, 0)
	if got != 2*time.Second {
		t.Fatalf("got %s, want the flag's own default (2s) when config is unset", got)
	}
}
