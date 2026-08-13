package procfmt

import (
	"fmt"
	"net"
	"os"
	"strings"
	"testing"
)

func TestParseTCPTableFixture(t *testing.T) {
	f, err := os.Open("testdata/proc_net_tcp_sample.txt")
	if err != nil {
		t.Fatalf("failed to open fixture: %v", err)
	}
	defer f.Close()

	entries, err := ParseTCPTable(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	// Line 0: 0100007F:0BB8 -> 127.0.0.1:3000, LISTEN, uid 0, inode 12345.
	if got := entries[0].LocalAddr.String(); got != "127.0.0.1" {
		t.Errorf("entries[0].LocalAddr = %q, want 127.0.0.1", got)
	}
	if entries[0].LocalPort != 3000 {
		t.Errorf("entries[0].LocalPort = %d, want 3000", entries[0].LocalPort)
	}
	if !entries[0].IsListening() {
		t.Errorf("entries[0] should be listening")
	}
	if entries[0].UID != 0 {
		t.Errorf("entries[0].UID = %d, want 0", entries[0].UID)
	}
	if entries[0].Inode != 12345 {
		t.Errorf("entries[0].Inode = %d, want 12345", entries[0].Inode)
	}

	// Line 1: 00000000:1F90 -> 0.0.0.0:8080, LISTEN, uid 1000.
	if got := entries[1].LocalAddr.String(); got != "0.0.0.0" {
		t.Errorf("entries[1].LocalAddr = %q, want 0.0.0.0", got)
	}
	if entries[1].LocalPort != 8080 {
		t.Errorf("entries[1].LocalPort = %d, want 8080", entries[1].LocalPort)
	}
	if entries[1].UID != 1000 {
		t.Errorf("entries[1].UID = %d, want 1000", entries[1].UID)
	}

	// Line 2: port 22, state 01 ESTABLISHED, not listening.
	if entries[2].LocalPort != 22 {
		t.Errorf("entries[2].LocalPort = %d, want 22", entries[2].LocalPort)
	}
	if entries[2].IsListening() {
		t.Errorf("entries[2] should not be listening (state 01 ESTABLISHED)")
	}
}

func TestFilterListeningKeepsOnlyListenState(t *testing.T) {
	entries := []TCPEntry{
		{LocalPort: 3000, State: StateListen},
		{LocalPort: 22, State: 0x01},
		{LocalPort: 8080, State: StateListen},
	}

	got := FilterListening(entries)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0].LocalPort != 3000 || got[1].LocalPort != 8080 {
		t.Fatalf("got %+v, want ports 3000 and 8080", got)
	}
}

func TestParseTCPTableIPv6(t *testing.T) {
	// Build the kernel's 4x32-bit-word little-endian hex encoding of ::1
	// programmatically rather than hand-typing 32 hex chars.
	raw := net.ParseIP("::1").To16()
	var hexAddr strings.Builder
	for i := 0; i < 16; i += 4 {
		fmt.Fprintf(&hexAddr, "%02X%02X%02X%02X", raw[i+3], raw[i+2], raw[i+1], raw[i])
	}

	sample := "  sl  local_address                         rem_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		fmt.Sprintf("   0: %s:20FB 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 99999 1 0000000000000000 100 0 0 10 0\n", hexAddr.String())

	entries, err := ParseTCPTable(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if got := entries[0].LocalAddr.String(); got != "::1" {
		t.Errorf("LocalAddr = %q, want ::1", got)
	}
	if entries[0].LocalPort != 0x20FB {
		t.Errorf("LocalPort = %d, want %d", entries[0].LocalPort, 0x20FB)
	}
}

func TestParseTCPTableSkipsMalformedLines(t *testing.T) {
	sample := "header line ignored\nnot enough fields\n   0: 0100007F:0BB8 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0\n"

	entries, err := ParseTCPTable(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (malformed line skipped)", len(entries))
	}
}
