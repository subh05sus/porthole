package lsoffmt

import (
	"os"
	"strings"
	"testing"
)

func TestParseLsofFieldsFixture(t *testing.T) {
	f, err := os.Open("testdata/lsof_sample.txt")
	if err != nil {
		t.Fatalf("failed to open fixture: %v", err)
	}
	defer f.Close()

	sockets, err := ParseLsofFields(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sockets) != 3 {
		t.Fatalf("got %d sockets, want 3", len(sockets))
	}

	want := Socket{PID: 1234, Command: "node", UID: 1000, Proto: "TCP", Addr: "*:3000", Host: "*", Port: 3000}
	if sockets[0] != want {
		t.Errorf("sockets[0] = %+v, want %+v", sockets[0], want)
	}

	want = Socket{PID: 5678, Command: "postgres", UID: 999, Proto: "TCP", Addr: "127.0.0.1:5432", Host: "127.0.0.1", Port: 5432}
	if sockets[1] != want {
		t.Errorf("sockets[1] = %+v, want %+v", sockets[1], want)
	}

	want = Socket{PID: 9012, Command: "SomeApp", UID: 1000, Proto: "TCP", Addr: "[::1]:8443", Host: "[::1]", Port: 8443}
	if sockets[2] != want {
		t.Errorf("sockets[2] = %+v, want %+v", sockets[2], want)
	}
}

func TestParseLsofFieldsMultipleSocketsSameProcess(t *testing.T) {
	sample := "p111\ncnode\nu0\nPTCP\nn*:3000\nPTCP\nn*:3001\n"

	sockets, err := ParseLsofFields(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sockets) != 2 {
		t.Fatalf("got %d sockets, want 2", len(sockets))
	}
	if sockets[0].PID != 111 || sockets[1].PID != 111 {
		t.Fatalf("both sockets should belong to pid 111: %+v", sockets)
	}
	if sockets[0].Port != 3000 || sockets[1].Port != 3001 {
		t.Fatalf("got ports %d,%d, want 3000,3001", sockets[0].Port, sockets[1].Port)
	}
}

func TestParseLsofFieldsNBeforePErrors(t *testing.T) {
	_, err := ParseLsofFields(strings.NewReader("n*:3000\n"))
	if err == nil {
		t.Fatalf("expected error for n field with no preceding p field")
	}
}

func TestSplitLsofAddrUnparseablePort(t *testing.T) {
	// When the port segment isn't a number, the raw string is preserved as
	// Host rather than guessing where to truncate it.
	host, port := splitLsofAddr("*:*")
	if host != "*:*" || port != -1 {
		t.Fatalf("got host=%q port=%d, want host=%q port=-1", host, port, "*:*")
	}
}

func TestSplitLsofAddrNoColon(t *testing.T) {
	host, port := splitLsofAddr("garbage")
	if host != "garbage" || port != -1 {
		t.Fatalf("got host=%q port=%d, want host=%q port=-1", host, port, "garbage")
	}
}
