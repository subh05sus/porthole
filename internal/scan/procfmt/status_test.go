package procfmt

import "testing"

func TestParseVmRSSFindsAndConvertsToBytes(t *testing.T) {
	data := []byte("Name:\tnode\nVmPeak:\t   123456 kB\nVmRSS:\t    45678 kB\nThreads:\t4\n")
	got, err := ParseVmRSS(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := uint64(45678 * 1024); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

func TestParseVmRSSMissingErrors(t *testing.T) {
	if _, err := ParseVmRSS([]byte("Name:\tnode\nThreads:\t4\n")); err == nil {
		t.Fatalf("expected an error when VmRSS is absent")
	}
}

func TestParseVmRSSMalformedLineErrors(t *testing.T) {
	if _, err := ParseVmRSS([]byte("VmRSS:\n")); err == nil {
		t.Fatalf("expected an error for a VmRSS line with no value")
	}
}
