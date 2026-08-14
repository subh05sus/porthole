package firewall

import "testing"

func TestRuleNameRoundTrip(t *testing.T) {
	cases := []Rule{
		{Port: 3000, Proto: "tcp", Action: ActionBlock, Direction: DirectionIn},
		{Port: 9191, Proto: "tcp", Action: ActionAllow, Direction: DirectionOut},
		{Port: 53, Proto: "udp", Action: ActionBlock, Direction: DirectionIn},
	}
	for _, want := range cases {
		name := want.Name()
		got, ok := ParseRuleName(name)
		if !ok {
			t.Fatalf("ParseRuleName(%q) failed to parse", name)
		}
		if got != want {
			t.Fatalf("ParseRuleName(%q) = %+v, want %+v", name, got, want)
		}
	}
}

func TestParseRuleNameRejectsForeignNames(t *testing.T) {
	cases := []string{
		"",
		"not-porthole-in-block-3000-tcp",
		"porthole-sideways-block-3000-tcp", // bad direction
		"porthole-in-maybe-3000-tcp",       // bad action
		"porthole-in-block-notaport-tcp",   // bad port
		"porthole-in-block-3000-icmp",      // bad proto
		"porthole-in-block-3000",           // too few fields
	}
	for _, name := range cases {
		if _, ok := ParseRuleName(name); ok {
			t.Errorf("ParseRuleName(%q) unexpectedly succeeded", name)
		}
	}
}
