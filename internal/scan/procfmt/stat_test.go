package procfmt

import "testing"

func TestParseStatSimpleComm(t *testing.T) {
	line := "1234 (node) S 1 1234 1234 0 -1 4194304 1234 0 5 0 12 3 0 0 20 0 4 0 987654321 135168000 1234 18446744073709551615 1 1 0 0 0 0 0 4096 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0"

	info, err := ParseStat([]byte(line))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.PID != 1234 {
		t.Errorf("PID = %d, want 1234", info.PID)
	}
	if info.Comm != "node" {
		t.Errorf("Comm = %q, want %q", info.Comm, "node")
	}
	if info.State != 'S' {
		t.Errorf("State = %q, want 'S'", info.State)
	}
	if info.StartTimeTicks != 987654321 {
		t.Errorf("StartTimeTicks = %d, want 987654321", info.StartTimeTicks)
	}
	if info.UTimeTicks != 12 {
		t.Errorf("UTimeTicks = %d, want 12", info.UTimeTicks)
	}
	if info.STimeTicks != 3 {
		t.Errorf("STimeTicks = %d, want 3", info.STimeTicks)
	}
}

func TestParseStatCommWithParens(t *testing.T) {
	// A process can name itself almost anything, including something that
	// looks like it should confuse a naive first-paren/last-paren scan.
	line := "5678 (cat (1) copy) R 1 5678 5678 0 -1 4194304 1 0 0 0 1 1 0 0 20 0 1 0 111222333 0 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0"

	info, err := ParseStat([]byte(line))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Comm != "cat (1) copy" {
		t.Errorf("Comm = %q, want %q", info.Comm, "cat (1) copy")
	}
	if info.StartTimeTicks != 111222333 {
		t.Errorf("StartTimeTicks = %d, want 111222333", info.StartTimeTicks)
	}
}

func TestParseStatMissingCommErrors(t *testing.T) {
	if _, err := ParseStat([]byte("1234 no-parens-here S 1")); err == nil {
		t.Fatalf("expected error for missing comm parens")
	}
}

func TestParseStatTruncatedLineErrors(t *testing.T) {
	if _, err := ParseStat([]byte("1234 (node) S 1")); err == nil {
		t.Fatalf("expected error for truncated stat line")
	}
}
