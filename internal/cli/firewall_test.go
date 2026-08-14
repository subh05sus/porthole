package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/subh05sus/porthole/internal/firewall"
	"github.com/subh05sus/porthole/internal/firewall/firewalltest"
)

func newFirewallTestApp(input string, mgr *firewalltest.FakeManager) (*App, *bytes.Buffer) {
	var stdout bytes.Buffer
	app := &App{
		Firewall: mgr,
		Stdin:    strings.NewReader(input),
		Stdout:   &stdout,
		Stderr:   &stdout,
	}
	return app, &stdout
}

func TestFirewallBlockRequiresTypedPortConfirmation(t *testing.T) {
	mgr := &firewalltest.FakeManager{}
	app, stdout := newFirewallTestApp("3000\n", mgr)

	code := Execute(app, []string{"firewall", "block", "3000"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0: %s", code, stdout.String())
	}
	if len(mgr.ApplyCalls) != 1 {
		t.Fatalf("expected exactly one Apply call, got %d", len(mgr.ApplyCalls))
	}
	got := mgr.ApplyCalls[0]
	want := firewall.Rule{Port: 3000, Proto: "tcp", Action: firewall.ActionBlock, Direction: firewall.DirectionIn}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if !strings.Contains(stdout.String(), "applied:") {
		t.Fatalf("expected an applied confirmation, got %q", stdout.String())
	}
}

func TestFirewallBlockWrongConfirmationCancels(t *testing.T) {
	mgr := &firewalltest.FakeManager{}
	app, stdout := newFirewallTestApp("nope\n", mgr)

	code := Execute(app, []string{"firewall", "block", "3000"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if len(mgr.ApplyCalls) != 0 {
		t.Fatalf("expected no Apply call on a mismatched confirmation, got %d", len(mgr.ApplyCalls))
	}
	if !strings.Contains(stdout.String(), "cancelled") {
		t.Fatalf("expected a cancellation message, got %q", stdout.String())
	}
}

func TestFirewallAllowWithOutAndUDPFlags(t *testing.T) {
	mgr := &firewalltest.FakeManager{}
	app, _ := newFirewallTestApp("53\n", mgr)

	code := Execute(app, []string{"firewall", "allow", "53", "--proto", "udp", "--out"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if len(mgr.ApplyCalls) != 1 {
		t.Fatalf("expected exactly one Apply call, got %d", len(mgr.ApplyCalls))
	}
	want := firewall.Rule{Port: 53, Proto: "udp", Action: firewall.ActionAllow, Direction: firewall.DirectionOut}
	if mgr.ApplyCalls[0] != want {
		t.Fatalf("got %+v, want %+v", mgr.ApplyCalls[0], want)
	}
}

func TestFirewallBlockRejectsInvalidPort(t *testing.T) {
	mgr := &firewalltest.FakeManager{}
	app, _ := newFirewallTestApp("", mgr)

	code := Execute(app, []string{"firewall", "block", "notaport"})
	if code == ExitSuccess {
		t.Fatalf("expected a non-zero exit for an invalid port")
	}
	if len(mgr.ApplyCalls) != 0 {
		t.Fatalf("must never call Apply for an invalid port")
	}
}

func TestFirewallListShowsOwnedRules(t *testing.T) {
	mgr := &firewalltest.FakeManager{Rules: []firewall.Rule{
		{Port: 3000, Proto: "tcp", Action: firewall.ActionBlock, Direction: firewall.DirectionIn},
	}}
	app, stdout := newFirewallTestApp("", mgr)

	code := Execute(app, []string{"firewall", "list"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "3000") || !strings.Contains(stdout.String(), "block") {
		t.Fatalf("expected the rule listed, got %q", stdout.String())
	}
}

func TestFirewallListEmptyReportsNone(t *testing.T) {
	mgr := &firewalltest.FakeManager{}
	app, stdout := newFirewallTestApp("", mgr)

	code := Execute(app, []string{"firewall", "list"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "no porthole-owned firewall rules") {
		t.Fatalf("got %q", stdout.String())
	}
}

func TestFirewallCleanRequiresTypedCLEANConfirmation(t *testing.T) {
	mgr := &firewalltest.FakeManager{Rules: []firewall.Rule{
		{Port: 3000, Proto: "tcp", Action: firewall.ActionBlock, Direction: firewall.DirectionIn},
		{Port: 53, Proto: "udp", Action: firewall.ActionAllow, Direction: firewall.DirectionOut},
	}}
	app, stdout := newFirewallTestApp("CLEAN\n", mgr)

	code := Execute(app, []string{"firewall", "clean"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0: %s", code, stdout.String())
	}
	if mgr.RemoveAllCalls != 1 {
		t.Fatalf("expected exactly one RemoveAll call, got %d", mgr.RemoveAllCalls)
	}
	if !strings.Contains(stdout.String(), "removed 2 rule(s)") {
		t.Fatalf("expected a removal confirmation, got %q", stdout.String())
	}
}

func TestFirewallCleanWrongConfirmationCancels(t *testing.T) {
	mgr := &firewalltest.FakeManager{Rules: []firewall.Rule{
		{Port: 3000, Proto: "tcp", Action: firewall.ActionBlock, Direction: firewall.DirectionIn},
	}}
	app, stdout := newFirewallTestApp("yes\n", mgr)

	code := Execute(app, []string{"firewall", "clean"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if mgr.RemoveAllCalls != 0 {
		t.Fatalf("expected no RemoveAll call on a mismatched confirmation ('yes' is not 'CLEAN'), got %d", mgr.RemoveAllCalls)
	}
	if !strings.Contains(stdout.String(), "cancelled") {
		t.Fatalf("expected a cancellation message, got %q", stdout.String())
	}
}

func TestFirewallCleanWithNothingToRemove(t *testing.T) {
	mgr := &firewalltest.FakeManager{}
	app, stdout := newFirewallTestApp("", mgr)

	code := Execute(app, []string{"firewall", "clean"})
	if code != ExitSuccess {
		t.Fatalf("got exit code %d, want 0", code)
	}
	if mgr.RemoveAllCalls != 0 {
		t.Fatalf("expected no RemoveAll call when there is nothing to remove")
	}
	if !strings.Contains(stdout.String(), "no porthole-owned firewall rules to remove") {
		t.Fatalf("got %q", stdout.String())
	}
}
