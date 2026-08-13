package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/subh05sus/porthole/internal/scan/scantest"
)

func TestDoctorRunsAndPrintsChecks(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: &scantest.FakeLister{},
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	code := Execute(app, []string{"doctor"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0: stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "NO_COLOR") {
		t.Fatalf("expected NO_COLOR check in output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "config file") {
		t.Fatalf("expected config file check in output, got %q", stdout.String())
	}
}
