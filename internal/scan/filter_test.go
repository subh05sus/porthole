package scan

import "testing"

func TestFilterDisplayNoFiltersReturnsAllUnchanged(t *testing.T) {
	services := []Service{{Port: 80, Owned: false}, {Port: 3000, Owned: true}}
	got := FilterDisplay(services, false, false)
	if len(got) != 2 {
		t.Fatalf("got %d services, want 2 (no filtering)", len(got))
	}
}

func TestFilterDisplayHidesUnowned(t *testing.T) {
	services := []Service{
		{Port: 3000, Owned: true},
		{Port: 22, Owned: false},
	}
	got := FilterDisplay(services, true, false)
	if len(got) != 1 || got[0].Port != 3000 {
		t.Fatalf("got %+v, want only the owned port 3000", got)
	}
}

func TestFilterDisplayHidesPrivilegedPorts(t *testing.T) {
	services := []Service{
		{Port: 80, Owned: true},
		{Port: 3000, Owned: true},
	}
	got := FilterDisplay(services, false, true)
	if len(got) != 1 || got[0].Port != 3000 {
		t.Fatalf("got %+v, want only the non-privileged port 3000", got)
	}
}

func TestFilterDisplayHidesIndependently(t *testing.T) {
	// An owned row on a privileged port must still be hidden when only
	// hidePrivilegedPorts is set — the two filters are independent, not
	// "hide only if both conditions match."
	services := []Service{{Port: 443, Owned: true}}
	got := FilterDisplay(services, false, true)
	if len(got) != 0 {
		t.Fatalf("got %+v, want the owned-but-privileged row hidden", got)
	}

	// And a privileged-but-unowned row must still be hidden when only
	// hideSystemProcesses is set.
	services = []Service{{Port: 443, Owned: false}}
	got = FilterDisplay(services, true, false)
	if len(got) != 0 {
		t.Fatalf("got %+v, want the unowned-but-privileged row hidden", got)
	}
}

func TestFilterDisplayEmptyInputReturnsEmpty(t *testing.T) {
	got := FilterDisplay(nil, true, true)
	if len(got) != 0 {
		t.Fatalf("got %+v, want empty", got)
	}
}
