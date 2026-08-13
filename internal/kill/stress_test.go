//go:build windows

package kill

import (
	"context"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// TestKillLadderConcurrentStress hammers the real Windows kill path (not
// the fake Signaler kill_test.go uses) with many concurrent real process
// kills, each through its own captured (PID, StartTime) target. This is a
// soak/regression test for the production path under load, not a proof
// that no bug exists — genuine PID-reuse detection under a forced race is
// covered deterministically by kill_test.go's fake-Signaler tests
// (TestExecutePIDReusedAborts etc); this test can't force the OS to
// recycle a specific PID on demand.
func TestKillLadderConcurrentStress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in -short mode")
	}

	const n = 30
	signaler := windowsSignaler{}

	type spawned struct {
		cmd    *exec.Cmd
		target Target
	}

	procs := make([]spawned, n)
	for i := range procs {
		cmd := exec.Command("ping", "-n", "30", "127.0.0.1")
		if err := cmd.Start(); err != nil {
			t.Fatalf("spawning process %d: %v", i, err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

		st, alive, err := signaler.StillAlive(cmd.Process.Pid)
		if err != nil {
			t.Fatalf("StillAlive after spawning process %d: %v", i, err)
		}
		if !alive {
			t.Fatalf("process %d reported not alive immediately after spawning", i)
		}
		procs[i] = spawned{cmd: cmd, target: Target{PID: cmd.Process.Pid, StartTime: st, Owned: true}}
	}

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i, p := range procs {
		wg.Add(1)
		go func(i int, p spawned) {
			defer wg.Done()
			ladder := &Ladder{Signaler: signaler}
			_, err := ladder.Execute(context.Background(), p.target, Options{PollInterval: 20 * time.Millisecond, PollTimeout: 3 * time.Second})
			errs[i] = err
		}(i, p)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("process %d: unexpected error from concurrent Execute: %v", i, err)
		}
	}
	for i, p := range procs {
		_, alive, err := signaler.StillAlive(p.target.PID)
		if err != nil {
			t.Errorf("process %d: StillAlive after kill: %v", i, err)
			continue
		}
		if alive {
			t.Errorf("process %d (pid %d) is still alive — concurrent kill did not take effect", i, p.target.PID)
		}
	}
}

// TestPIDReuseOpportunistic rapidly spawns and reaps short-lived processes,
// opportunistically checking whenever the OS happens to recycle a PID
// within the test window: when it does, the ladder must abort against the
// stale target rather than touching the new occupant. Windows doesn't
// expose a way to force this on demand, so this is genuinely probabilistic
// — it may observe zero recycling events on a quiet system, in which case
// it only exercises the spawn/reap loop itself. The deterministic guarantee
// still comes from kill_test.go's fake-Signaler tests.
func TestPIDReuseOpportunistic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in -short mode")
	}

	const iterations = 300
	signaler := windowsSignaler{}
	reuseObserved := 0

	for i := 0; i < iterations; i++ {
		cmdA := exec.Command("cmd", "/c", "exit", "0")
		if err := cmdA.Start(); err != nil {
			t.Fatalf("iteration %d: starting process A: %v", i, err)
		}
		pidA := cmdA.Process.Pid
		startA, aliveA, err := signaler.StillAlive(pidA)
		if err != nil {
			t.Fatalf("iteration %d: StillAlive(A): %v", i, err)
		}
		if !aliveA {
			// A already exited before we could read its start time —
			// nothing to check this iteration, move on.
			_ = cmdA.Wait()
			continue
		}
		_ = cmdA.Wait()

		cmdB := exec.Command("ping", "-n", "2", "127.0.0.1")
		if err := cmdB.Start(); err != nil {
			t.Fatalf("iteration %d: starting process B: %v", i, err)
		}
		pidB := cmdB.Process.Pid

		if pidB == pidA {
			reuseObserved++
			ladder := &Ladder{Signaler: signaler}
			target := Target{PID: pidA, StartTime: startA, Owned: true}

			_, err := ladder.Execute(context.Background(), target, Options{PollInterval: time.Millisecond, PollTimeout: 20 * time.Millisecond})
			if err == nil {
				t.Errorf("iteration %d: expected an error when PID %d was recycled, got none", i, pidA)
			}

			_, aliveB, err := signaler.StillAlive(pidB)
			if err != nil {
				t.Errorf("iteration %d: StillAlive(B): %v", i, err)
			} else if !aliveB {
				t.Errorf("iteration %d: process B (recycled pid %d) was killed — PID-reuse guard failed", i, pidB)
			}
		}

		_ = cmdB.Process.Kill()
		_ = cmdB.Wait()
	}

	t.Logf("PID reuse observed in %d/%d iterations", reuseObserved, iterations)
}
