//go:build windows

package restart

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/subh05sus/porthole/internal/proc"
)

// TestSpawnRealProcess is a real integration test: spawns an actual
// process via the production spawn() function (not a fake), then uses the
// already-empirically-verified proc.Lookup PEB reader to confirm the
// spawned process's CWD and environment actually match the Plan — closing
// the loop on the whole capture-kill-respawn round trip, since Spawn's
// interface deliberately doesn't return a PID to track (fire-and-forget,
// detached from porthole).
func TestSpawnRealProcess(t *testing.T) {
	dir := t.TempDir()
	const pingPath = `C:\Windows\System32\PING.EXE`
	const marker = "PORTHOLE_RESTART_SPAWN_TEST=marker98765"

	plan := Plan{
		Process: "ping",
		ExePath: pingPath,
		Cmdline: pingPath + ` -n 15 127.0.0.1`,
		CWD:     dir,
		Env:     []string{marker},
	}

	if err := spawn(plan); err != nil {
		t.Fatalf("spawn failed: %v", err)
	}

	pid := findSpawnedPing(t, dir)
	t.Cleanup(func() {
		_ = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run()
	})

	lookup := proc.NewDefaultLookup()
	info, err := lookup.Lookup(pid)
	if err != nil {
		t.Fatalf("Lookup(%d): %v", pid, err)
	}

	gotCWD := strings.TrimRight(info.CWD, `\`)
	wantCWD := strings.TrimRight(dir, `\`)
	if !strings.EqualFold(gotCWD, wantCWD) {
		t.Errorf("CWD = %q, want %q", info.CWD, dir)
	}

	found := false
	for _, kv := range info.Env {
		if kv == marker {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("spawned process env does not contain %q; got %d entries", marker, len(info.Env))
	}
}

// findSpawnedPing polls for a PING.EXE process whose cwd matches dir,
// distinguishing it from any unrelated ping already running on the
// machine. Uses PowerShell's CIM cmdlets rather than wmic, which is
// deprecated on current Windows.
func findSpawnedPing(t *testing.T, dir string) int {
	t.Helper()
	lookup := proc.NewDefaultLookup()
	wantCWD := strings.TrimRight(dir, `\`)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			`Get-CimInstance Win32_Process -Filter "Name='PING.EXE'" | Select-Object -ExpandProperty ProcessId`,
		).Output()
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				line = strings.TrimSpace(line)
				pid, convErr := strconv.Atoi(line)
				if convErr != nil {
					continue
				}
				info, lookupErr := lookup.Lookup(pid)
				if lookupErr != nil {
					continue
				}
				if strings.EqualFold(strings.TrimRight(info.CWD, `\`), wantCWD) {
					return pid
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for a spawned PING.EXE with cwd %q to appear", dir)
	return 0
}
