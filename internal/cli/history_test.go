package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/subh05sus/porthole/internal/history"
	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/kill/killtest"
)

func TestHistoryEmptyReportsNoHistory(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := &App{HistoryPath: filepath.Join(t.TempDir(), "history"), Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}

	code := Execute(app, []string{"history"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "no kill history yet") {
		t.Fatalf("got %q", stdout.String())
	}
}

func TestHistoryShowsLoggedEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	lk := &history.LoggingKiller{Inner: &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}, Path: path}
	if _, err := lk.Execute(context.Background(), kill.Target{PID: 999}, kill.Options{}); err != nil {
		t.Fatalf("unexpected error priming history: %v", err)
	}

	var stdout, stderr bytes.Buffer
	app := &App{HistoryPath: path, Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}

	code := Execute(app, []string{"history"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0: %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "pid=999") {
		t.Fatalf("expected pid=999 in output, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "status=killed") {
		t.Fatalf("expected status=killed in output, got %q", stdout.String())
	}
}
