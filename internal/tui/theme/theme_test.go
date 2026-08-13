package theme

import (
	"os"
	"testing"
)

func TestNewForcePlainProducesPlainTheme(t *testing.T) {
	th := New(true)
	if !th.Plain {
		t.Fatalf("expected Plain theme when forcePlain=true")
	}
}

func TestNewRespectsNOCOLOREnvVarEvenWhenEmpty(t *testing.T) {
	// The NO_COLOR convention (no-color.org) treats presence, not value, as
	// the signal — an empty string still means "disable color."
	t.Setenv("NO_COLOR", "")
	th := New(false)
	if !th.Plain {
		t.Fatalf("expected Plain theme when NO_COLOR is set, even to an empty value")
	}
}

func TestNewWithoutNOCOLORProducesColorTheme(t *testing.T) {
	unsetForTest(t, "NO_COLOR")
	th := New(false)
	if th.Plain {
		t.Fatalf("expected non-plain theme when NO_COLOR is unset and forcePlain=false")
	}
}

// unsetForTest removes an env var for the duration of the test and restores
// its prior value (or absence) afterward. t.Setenv only supports setting a
// value, not removing one, which is what a "NO_COLOR is absent" test needs.
func unsetForTest(t *testing.T, key string) {
	t.Helper()
	prev, wasSet := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("failed to unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if wasSet {
			os.Setenv(key, prev)
		}
	})
}
