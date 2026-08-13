//go:build windows

package proc

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestWow64ReadAgainstRealSpawnedProcess empirically verifies the WOW64 PEB
// offsets in peb_wow64_windows.go against a real 32-bit process — SysWOW64's
// PING.EXE is a genuine 32-bit binary on any 64-bit Windows install, so no
// custom-built helper is needed. Mirrors TestLookupAgainstRealSpawnedProcess
// (proc_windows_test.go), which did the same for the native 64-bit path.
func TestWow64ReadAgainstRealSpawnedProcess(t *testing.T) {
	ping32 := `C:\Windows\SysWOW64\PING.EXE`
	if _, err := os.Stat(ping32); err != nil {
		t.Skipf("SysWOW64 PING.EXE not present on this system: %v", err)
	}

	dir := t.TempDir()
	cmd := exec.Command(ping32, "-n", "20", "127.0.0.1")
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start 32-bit child process: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	time.Sleep(200 * time.Millisecond)

	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		t.Fatalf("OpenProcess: %v", err)
	}
	defer windows.CloseHandle(h)

	wow64, err := isWow64Process(h)
	if err != nil {
		t.Fatalf("isWow64Process: %v", err)
	}
	if !wow64 {
		t.Fatalf("expected SysWOW64 PING.EXE to be detected as a WOW64 process")
	}

	cmdline, cwd, err := readProcessParametersWow64(h)
	if err != nil {
		t.Fatalf("readProcessParametersWow64: %v", err)
	}

	wantDir, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("failed to stat expected dir: %v", err)
	}
	gotDir, err := os.Stat(strings.TrimRight(cwd, `\`))
	if err != nil {
		t.Fatalf("WOW64 CWD %q does not exist: %v", cwd, err)
	}
	if !os.SameFile(wantDir, gotDir) {
		t.Errorf("CWD = %q, want a path resolving to %q", cwd, dir)
	}

	if !strings.Contains(cmdline, "127.0.0.1") {
		t.Errorf("Cmdline = %q, want it to contain 127.0.0.1", cmdline)
	}
}

// TestReadProcessParametersDispatchesToWow64 confirms the main
// readProcessParameters entry point (peb_windows.go) correctly routes a
// WOW64 target to the 32-bit path rather than misreading it via the native
// 64-bit offsets.
func TestReadProcessParametersDispatchesToWow64(t *testing.T) {
	ping32 := `C:\Windows\SysWOW64\PING.EXE`
	if _, err := os.Stat(ping32); err != nil {
		t.Skipf("SysWOW64 PING.EXE not present on this system: %v", err)
	}

	cmd := exec.Command(ping32, "-n", "10", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start 32-bit child process: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	time.Sleep(200 * time.Millisecond)

	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		t.Fatalf("OpenProcess: %v", err)
	}
	defer windows.CloseHandle(h)

	cmdline, _, err := readProcessParameters(h)
	if err != nil {
		t.Fatalf("readProcessParameters: %v", err)
	}
	if !strings.Contains(cmdline, "127.0.0.1") {
		t.Errorf("Cmdline = %q, want it to contain 127.0.0.1 (dispatch to WOW64 path may have failed)", cmdline)
	}
}
