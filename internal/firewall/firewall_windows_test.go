//go:build windows

package firewall

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// recordingNetsh is a fake runNetsh: it never shells out, just records
// every call and returns scripted output — the same "fake the OS-facing
// seam" pattern kill.Ladder's fake Signaler and scan's fake Lister use.
type recordingNetsh struct {
	calls   [][]string
	outputs []string // returned in order, one per call; last one repeats if exhausted
	err     error
}

func (r *recordingNetsh) run(ctx context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, args)
	if r.err != nil {
		return "", r.err
	}
	if len(r.outputs) == 0 {
		return "", nil
	}
	i := len(r.calls) - 1
	if i >= len(r.outputs) {
		i = len(r.outputs) - 1
	}
	return r.outputs[i], nil
}

const sampleShowRuleOutput = `Rule Name:                            HNS Container Networking - DNS (UDP-In) - 0
----------------------------------------------------------------------
Direction:                            In
Protocol:                             UDP
LocalPort:                            53
Action:                               Allow

Rule Name:                            porthole-in-block-47823-tcp
----------------------------------------------------------------------
Direction:                            In
Protocol:                             TCP
LocalPort:                            47823
Action:                               Block

Rule Name:                            porthole-out-allow-9191-tcp
----------------------------------------------------------------------
Direction:                            Out
Protocol:                             TCP
LocalPort:                            9191
Action:                               Allow
`

func TestListFiltersToPortholeOwnedRulesOnly(t *testing.T) {
	rn := &recordingNetsh{outputs: []string{sampleShowRuleOutput}}
	m := windowsManager{runNetsh: rn.run}

	rules, err := m.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("got %d rules, want 2 (the HNS rule must be excluded), got %+v", len(rules), rules)
	}
	if rules[0].Port != 47823 || rules[0].Action != ActionBlock || rules[0].Direction != DirectionIn || rules[0].Proto != "tcp" {
		t.Errorf("rules[0] = %+v", rules[0])
	}
	if rules[1].Port != 9191 || rules[1].Action != ActionAllow || rules[1].Direction != DirectionOut {
		t.Errorf("rules[1] = %+v", rules[1])
	}

	if len(rn.calls) != 1 || rn.calls[0][0] != "advfirewall" {
		t.Fatalf("expected exactly one 'advfirewall firewall show rule' call, got %+v", rn.calls)
	}
}

func TestApplyDeletesExistingRuleFirstThenAdds(t *testing.T) {
	rn := &recordingNetsh{}
	m := windowsManager{runNetsh: rn.run}

	rule := Rule{Port: 3000, Proto: "tcp", Action: ActionBlock, Direction: DirectionIn}
	if err := m.Apply(context.Background(), rule); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(rn.calls) != 2 {
		t.Fatalf("expected delete-then-add (2 calls), got %d: %+v", len(rn.calls), rn.calls)
	}
	if rn.calls[0][2] != "delete" {
		t.Errorf("expected the first call to delete any existing rule, got %+v", rn.calls[0])
	}
	if rn.calls[1][2] != "add" {
		t.Errorf("expected the second call to add the new rule, got %+v", rn.calls[1])
	}

	addArgs := strings.Join(rn.calls[1], " ")
	for _, want := range []string{"name=porthole-in-block-3000-tcp", "dir=in", "action=block", "protocol=TCP", "localport=3000"} {
		if !strings.Contains(addArgs, want) {
			t.Errorf("expected add-rule args to contain %q, got %q", want, addArgs)
		}
	}
}

func TestApplyReplacesRatherThanDuplicating(t *testing.T) {
	// Applying the same Rule twice must delete-then-add both times, never
	// skip the delete just because "we already applied this" — netsh
	// itself has no memory of that, and a second bare add would duplicate.
	rn := &recordingNetsh{}
	m := windowsManager{runNetsh: rn.run}

	rule := Rule{Port: 3000, Proto: "tcp", Action: ActionBlock, Direction: DirectionIn}
	_ = m.Apply(context.Background(), rule)
	_ = m.Apply(context.Background(), rule)

	if len(rn.calls) != 4 {
		t.Fatalf("expected 2x(delete+add) = 4 calls, got %d", len(rn.calls))
	}
}

func TestRemoveCallsDeleteWithTheRuleName(t *testing.T) {
	rn := &recordingNetsh{}
	m := windowsManager{runNetsh: rn.run}

	rule := Rule{Port: 3000, Proto: "tcp", Action: ActionBlock, Direction: DirectionIn}
	if err := m.Remove(context.Background(), rule); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rn.calls) != 1 {
		t.Fatalf("got %d calls, want 1", len(rn.calls))
	}
	if !strings.Contains(strings.Join(rn.calls[0], " "), "name=porthole-in-block-3000-tcp") {
		t.Errorf("expected delete call to reference the rule's exact name, got %+v", rn.calls[0])
	}
}

func TestRemoveAllOnlyTouchesPortholeOwnedRules(t *testing.T) {
	rn := &recordingNetsh{outputs: []string{sampleShowRuleOutput}}
	m := windowsManager{runNetsh: rn.run}

	if err := m.RemoveAll(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// call 0: the show-rule List(); calls 1-2: one delete per porthole rule.
	if len(rn.calls) != 3 {
		t.Fatalf("expected 1 list + 2 deletes = 3 calls, got %d: %+v", len(rn.calls), rn.calls)
	}
	for _, c := range rn.calls[1:] {
		joined := strings.Join(c, " ")
		if !strings.Contains(joined, "delete") || !strings.Contains(joined, RulePrefix) {
			t.Errorf("expected a delete of a porthole-prefixed rule, got %+v", c)
		}
		if strings.Contains(joined, "HNS") {
			t.Fatalf("must never touch a non-porthole rule, got %+v", c)
		}
	}
}

func TestListPropagatesNetshError(t *testing.T) {
	rn := &recordingNetsh{err: errors.New("requires elevation")}
	m := windowsManager{runNetsh: rn.run}

	if _, err := m.List(context.Background()); err == nil {
		t.Fatalf("expected the netsh error to propagate")
	}
}
