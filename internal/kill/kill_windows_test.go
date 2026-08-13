//go:build windows

package kill

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// TestKillRealSpawnedProcess is a real integration test: spawns an actual
// child process, kills it through the live ladder (not a fake Signaler),
// and asserts it actually died. This is the one platform this session can
// verify the kill path end-to-end rather than only compile-checking it.
func TestKillRealSpawnedProcess(t *testing.T) {
	cmd := exec.Command("ping", "-n", "30", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start child process: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	time.Sleep(200 * time.Millisecond)

	signaler := windowsSignaler{}
	startTime, alive, err := signaler.StillAlive(pid)
	if err != nil {
		t.Fatalf("StillAlive before kill: %v", err)
	}
	if !alive {
		t.Fatalf("expected spawned process to be alive")
	}

	ladder := &Ladder{Signaler: signaler}
	target := Target{PID: pid, StartTime: startTime, Owned: true}

	res, err := ladder.Execute(context.Background(), target, Options{PollInterval: 50 * time.Millisecond, PollTimeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Status != StatusKilled {
		t.Fatalf("got status %v, want StatusKilled", res.Status)
	}

	if _, alive, err := signaler.StillAlive(pid); err != nil {
		t.Fatalf("StillAlive after kill: %v", err)
	} else if alive {
		t.Fatalf("process is still alive after Execute reported StatusKilled")
	}
}

// TestKillDetectsPIDReuseViaStartTimeMismatch confirms the ladder's PID-
// reuse guard actually fires against real Windows process handles: a
// scanned StartTime that doesn't match the real process's current start
// time must abort rather than signal.
func TestKillDetectsPIDReuseViaStartTimeMismatch(t *testing.T) {
	cmd := exec.Command("ping", "-n", "10", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start child process: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	time.Sleep(200 * time.Millisecond)

	ladder := &Ladder{Signaler: windowsSignaler{}}
	target := Target{PID: pid, StartTime: 1, Owned: true} // deliberately wrong

	_, err := ladder.Execute(context.Background(), target, Options{})
	if err == nil {
		t.Fatalf("expected an error for mismatched start time")
	}
}
