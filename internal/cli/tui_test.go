package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/subh05sus/porthole/internal/kill/killtest"
	"github.com/subh05sus/porthole/internal/scan/scantest"
)

func TestBareInvocationWithoutTerminalRefusesRatherThanHang(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: &scantest.FakeLister{},
		Killer: &killtest.FakeKiller{},
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	code := Execute(app, nil)
	if code == ExitSuccess {
		t.Fatalf("expected a non-zero exit code when stdout isn't a terminal")
	}
	if !strings.Contains(stderr.String(), "interactive terminal") {
		t.Fatalf("expected ErrNotATerminal message on stderr, got %q", stderr.String())
	}
}

func TestIsTerminalFalseForNonFileWriter(t *testing.T) {
	var buf bytes.Buffer
	if isTerminal(&buf) {
		t.Fatalf("a bytes.Buffer must never report as a terminal")
	}
}
