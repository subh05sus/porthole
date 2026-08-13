package history

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/kill/killtest"
)

func TestLoggingKillerWritesTwoLinesPerExecute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	inner := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	lk := &LoggingKiller{Inner: inner, Path: path}

	_, err := lk.Execute(context.Background(), kill.Target{PID: 111}, kill.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 (attempt + result), got %+v", len(entries), entries)
	}
	if entries[0].Event != "attempt" || entries[0].PID != 111 {
		t.Errorf("got %+v, want an attempt entry for pid 111", entries[0])
	}
	if entries[1].Event != "result" || entries[1].Status != "killed" {
		t.Errorf("got %+v, want a result entry with status killed", entries[1])
	}
}

func TestLoggingKillerLogsEscalate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	inner := &killtest.FakeKiller{EscalateResult: kill.Result{Status: kill.StatusKilled}}
	lk := &LoggingKiller{Inner: inner, Path: path}

	_, err := lk.Escalate(context.Background(), kill.Target{PID: 222})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 2 || !entries[0].Force {
		t.Fatalf("expected an attempt entry with Force=true for an escalate, got %+v", entries)
	}
}

func TestLoggingKillerLogsErrorResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	inner := &killtest.FakeKiller{ExecuteErr: errors.New("boom")}
	lk := &LoggingKiller{Inner: inner, Path: path}

	_, _ = lk.Execute(context.Background(), kill.Target{PID: 333}, kill.Options{})

	entries, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(entries) != 2 || entries[1].Err != "boom" {
		t.Fatalf("expected a result entry with error boom, got %+v", entries)
	}
}

func TestLoggingKillerEmptyPathDisablesLogging(t *testing.T) {
	inner := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	lk := &LoggingKiller{Inner: inner, Path: ""}

	// Must not panic or error just because logging is disabled.
	if _, err := lk.Execute(context.Background(), kill.Target{PID: 1}, kill.Options{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoggingKillerPropagatesInnerResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	wantErr := errors.New("kill failed")
	inner := &killtest.FakeKiller{ExecuteErr: wantErr}
	lk := &LoggingKiller{Inner: inner, Path: path}

	_, err := lk.Execute(context.Background(), kill.Target{PID: 1}, kill.Options{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("got %v, want the inner Killer's error propagated unchanged", err)
	}
}

func TestReadAllMissingFileReturnsEmptyNotError(t *testing.T) {
	entries, err := ReadAll(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("got %d entries, want 0", len(entries))
	}
}

func TestReadAllSkipsMalformedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	inner := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	lk := &LoggingKiller{Inner: inner, Path: path}
	_, _ = lk.Execute(context.Background(), kill.Target{PID: 1}, kill.Options{})

	// Append a corrupt line directly.
	appendRaw(t, path, "not valid json{{{")

	entries, err := ReadAll(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected the malformed line to be skipped, got %d entries", len(entries))
	}
}

func appendRaw(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to open history file: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("failed to append: %v", err)
	}
}
