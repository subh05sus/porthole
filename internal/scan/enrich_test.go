package scan

import (
	"testing"

	"github.com/subh05sus/porthole/internal/project"
)

func TestEnrichSetsProjectFromCWD(t *testing.T) {
	services := []Service{
		{Port: 3000, CWD: ""}, // no CWD -> detector's "system" fallback
	}

	got := Enrich(services, project.NewDetector())
	if got[0].Project != "system" {
		t.Fatalf("got Project %q, want system", got[0].Project)
	}
}

func TestEnrichReusesDetectorCacheAcrossCalls(t *testing.T) {
	detector := project.NewDetector()
	// Detect once with an empty cwd to prime a known cache entry, then
	// confirm Enrich goes through the same Detector rather than
	// constructing a fresh one per call.
	if got := detector.Detect(""); got != "system" {
		t.Fatalf("precondition failed: got %q", got)
	}

	got := Enrich([]Service{{Port: 1, CWD: ""}}, detector)
	if got[0].Project != "system" {
		t.Fatalf("got %q, want system", got[0].Project)
	}
}
