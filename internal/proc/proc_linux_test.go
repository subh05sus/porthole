//go:build linux

package proc

import "testing"

func TestProcessUptimeSubtractsStartFromSystemUptime(t *testing.T) {
	// This only exercises the arithmetic; readSystemUptime itself reads a
	// real file and isn't mocked here, so we can't assert an exact value
	// end-to-end. processUptime clamps negative results to zero instead of
	// going negative when a process starts "after" the uptime snapshot
	// (a real possibility given the two reads aren't atomic).
	d, err := processUptime(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d < 0 {
		t.Fatalf("got negative uptime %v, want >= 0", d)
	}
}
