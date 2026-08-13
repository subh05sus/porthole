//go:build windows

package proc

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestLookupAgainstRealSpawnedProcess is a real integration test: it
// spawns an actual child process with a known working directory and
// command line, then verifies proc.Lookup's PEB read (the riskiest,
// undocumented-internals part of this package) reports the same thing.
// This is the one place this session can empirically validate the PEB
// field offsets rather than trusting them from memory.
func TestLookupAgainstRealSpawnedProcess(t *testing.T) {
	dir := t.TempDir()

	cmd := exec.Command("ping", "-n", "20", "127.0.0.1")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PORTHOLE_TEST_MARKER=verify12345")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start child process: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// Give the child a moment to finish initializing its PEB before we
	// read it.
	time.Sleep(200 * time.Millisecond)

	lookup := NewDefaultLookup()
	info, err := lookup.Lookup(pid)
	if err != nil {
		t.Fatalf("Lookup(%d) failed: %v", pid, err)
	}

	if !strings.EqualFold(info.Process, "ping") {
		t.Errorf("Process = %q, want %q", info.Process, "ping")
	}

	wantDir, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("failed to stat expected dir: %v", err)
	}
	gotDir, err := os.Stat(strings.TrimRight(info.CWD, `\`))
	if err != nil {
		t.Fatalf("proc.Lookup CWD %q does not exist: %v", info.CWD, err)
	}
	if !os.SameFile(wantDir, gotDir) {
		t.Errorf("CWD = %q, want a path resolving to %q", info.CWD, dir)
	}

	if !strings.Contains(info.Cmdline, "127.0.0.1") {
		t.Errorf("Cmdline = %q, want it to contain %q", info.Cmdline, "127.0.0.1")
	}

	if info.Uptime < 0 || info.Uptime > time.Minute {
		t.Errorf("Uptime = %v, want a small positive duration", info.Uptime)
	}
	if info.StartTime == 0 {
		t.Errorf("StartTime = 0, want a non-zero FILETIME-derived value")
	}

	if info.EnvErr != nil {
		t.Errorf("EnvErr = %v, want nil", info.EnvErr)
	}
	found := false
	for _, kv := range info.Env {
		if kv == "PORTHOLE_TEST_MARKER=verify12345" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Env does not contain the marker variable we set; got %d entries", len(info.Env))
	}

	if info.ExePath == "" || !strings.Contains(strings.ToLower(info.ExePath), "ping.exe") {
		t.Errorf("ExePath = %q, want it to contain ping.exe", info.ExePath)
	}
}
