package firewallfmt

import (
	"os"
	"strings"
	"testing"
)

func TestParseNetshRulesFixture(t *testing.T) {
	f, err := os.Open("testdata/netsh_show_rule_sample.txt")
	if err != nil {
		t.Fatalf("failed to open fixture: %v", err)
	}
	defer f.Close()

	rules, err := ParseNetshRules(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 4 {
		t.Fatalf("got %d rules, want 4", len(rules))
	}

	// Rule 0: a real pre-existing HNS rule, not porthole's.
	if rules[0].Name != "HNS Container Networking - DNS (UDP-In) - 790E58B4-7939-4434-9358-89AE7DDBE87F - 0" {
		t.Errorf("rules[0].Name = %q", rules[0].Name)
	}
	if rules[0].Protocol != "UDP" || rules[0].LocalPort != "53" || rules[0].Action != "Allow" {
		t.Errorf("rules[0] = %+v", rules[0])
	}

	// Rule 1: LocalPort "Any" — not a single literal port.
	if port, ok := rules[1].LocalPortNumber(); ok {
		t.Errorf("rules[1].LocalPortNumber() = %d, ok=true; want ok=false for 'Any'", port)
	}

	// Rule 2: a porthole-owned rule.
	if rules[2].Name != "porthole-in-block-47823-tcp" {
		t.Errorf("rules[2].Name = %q", rules[2].Name)
	}
	if port, ok := rules[2].LocalPortNumber(); !ok || port != 47823 {
		t.Errorf("rules[2].LocalPortNumber() = %d, %v; want 47823, true", port, ok)
	}
	if rules[2].Direction != "In" || rules[2].Action != "Block" {
		t.Errorf("rules[2] = %+v", rules[2])
	}

	// Rule 3: a second porthole-owned rule, outbound allow.
	if rules[3].Direction != "Out" || rules[3].Action != "Allow" {
		t.Errorf("rules[3] = %+v", rules[3])
	}
}

func TestParseNetshRulesEmptyInput(t *testing.T) {
	rules, err := ParseNetshRules(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("got %d rules, want 0", len(rules))
	}
}

func TestParseNetshRulesSkipsFieldsBeforeAnyRuleName(t *testing.T) {
	sample := "Some preamble line\nDirection:    In\n\nRule Name:    porthole-in-block-80-tcp\nDirection:    In\nAction:       Block\n"
	rules, err := ParseNetshRules(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1", len(rules))
	}
	if rules[0].Name != "porthole-in-block-80-tcp" || rules[0].Direction != "In" {
		t.Errorf("rules[0] = %+v", rules[0])
	}
}

func TestParseNetshRulesSkipsUnnamedTrailingBlock(t *testing.T) {
	// A malformed/truncated final block with fields but no name should not
	// produce a bogus zero-value rule.
	sample := "Rule Name:    porthole-in-block-80-tcp\nAction:       Block\n\nDirection:    In\nAction:       Allow\n"
	rules, err := ParseNetshRules(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("got %d rules, want 1 (trailing unnamed block skipped): %+v", len(rules), rules)
	}
}
