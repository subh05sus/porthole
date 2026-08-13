// Package history implements FUTURE_PLANS.md's "history" idea: a
// JSONL log of what porthole killed and when, so "wait, did I kill that"
// has an answer.
package history

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/subh05sus/porthole/internal/kill"
)

// Entry is one line of the history log. Two are written per kill attempt
// (per FUTURE_PLANS.md: "two lines of JSONL appended... useful for wait,
// did I kill that"): one when the signal is sent, one with the outcome.
type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	PID       int       `json:"pid"`
	Event     string    `json:"event"` // "attempt" | "result"
	Force     bool      `json:"force,omitempty"`
	Status    string    `json:"status,omitempty"`
	Err       string    `json:"error,omitempty"`
}

// DefaultPath returns ~/.porthole/history for the current user.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("history: resolving home directory: %w", err)
	}
	return filepath.Join(home, ".porthole", "history"), nil
}

// LoggingKiller wraps a real kill.Killer, appending history entries around
// every call. Wrapping the Killer (rather than adding logging calls at
// every CLI/TUI call site) means every kill gets logged for free regardless
// of which entry point triggered it — CLI kill, TUI kill, bulk kill,
// restart's internal kill — without needing to remember to log at each one.
type LoggingKiller struct {
	Inner kill.Killer
	Path  string // empty disables logging entirely (best-effort feature)
}

var _ kill.Killer = (*LoggingKiller)(nil)

func (l *LoggingKiller) Execute(ctx context.Context, target kill.Target, opts kill.Options) (kill.Result, error) {
	l.append(Entry{Timestamp: time.Now(), PID: target.PID, Event: "attempt", Force: opts.Force})
	res, err := l.Inner.Execute(ctx, target, opts)
	l.append(resultEntry(target.PID, res, err))
	return res, err
}

func (l *LoggingKiller) Escalate(ctx context.Context, target kill.Target) (kill.Result, error) {
	l.append(Entry{Timestamp: time.Now(), PID: target.PID, Event: "attempt", Force: true})
	res, err := l.Inner.Escalate(ctx, target)
	l.append(resultEntry(target.PID, res, err))
	return res, err
}

func resultEntry(pid int, res kill.Result, err error) Entry {
	e := Entry{Timestamp: time.Now(), PID: pid, Event: "result"}
	if err != nil {
		e.Err = err.Error()
	} else {
		e.Status = statusString(res.Status)
	}
	return e
}

func statusString(s kill.Status) string {
	switch s {
	case kill.StatusKilled:
		return "killed"
	case kill.StatusAlreadyDead:
		return "already_dead"
	case kill.StatusNeedsEscalation:
		return "needs_escalation"
	default:
		return "unknown"
	}
}

// append is best-effort: a failure to write the history log must never
// affect the underlying kill, which has already happened by the time this
// runs (or is about to, for the "attempt" entry) — this is a log, not a
// transaction.
func (l *LoggingKiller) append(e Entry) {
	if l.Path == "" {
		return
	}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	data = append(data, '\n')
	_, _ = f.Write(data)
}
