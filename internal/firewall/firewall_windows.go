//go:build windows

package firewall

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/subh05sus/porthole/internal/firewall/firewallfmt"
)

type windowsManager struct {
	// runNetsh is overridable in tests so Apply/Remove/List's logic (name
	// construction, idempotent-replace behavior, prefix-only filtering) is
	// testable without a real elevated netsh — see firewall_windows_test.go.
	runNetsh func(ctx context.Context, args ...string) (stdout string, err error)
}

// NewDefaultManager returns the Windows firewall manager, shelling out to
// netsh advfirewall. [tested, live] — Apply/List/RemoveAll all verified
// against this real machine's Windows Firewall in an elevated terminal
// (Apply/Remove require Administrator elevation; List does not): applied
// a block rule on a disposable unused port, confirmed it via both
// `porthole firewall list` and netsh's own `show rule` directly (exact
// field match), then ran `firewall clean` and confirmed zero rules
// remained — including a final direct netsh check outside porthole
// entirely, confirming no trace was left behind.
func NewDefaultManager() Manager {
	return windowsManager{runNetsh: execNetsh}
}

func execNetsh(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "netsh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return stdout.String(), fmt.Errorf("firewall: netsh %s: %w (%s)", strings.Join(args, " "), err, msg)
	}
	return stdout.String(), nil
}

func (m windowsManager) List(ctx context.Context) ([]Rule, error) {
	out, err := m.runNetsh(ctx, "advfirewall", "firewall", "show", "rule", "name=all")
	if err != nil {
		return nil, err
	}

	raw, err := firewallfmt.ParseNetshRules(strings.NewReader(out))
	if err != nil {
		return nil, err
	}

	var rules []Rule
	for _, r := range raw {
		if !strings.HasPrefix(r.Name, RulePrefix) {
			continue // not ours — never surface or touch a pre-existing rule
		}
		port, ok := r.LocalPortNumber()
		if !ok {
			continue // a malformed/foreign rule that happens to share our prefix; ignore rather than guess
		}
		rule := Rule{
			Port:      port,
			Proto:     strings.ToLower(r.Protocol),
			Action:    parseAction(r.Action),
			Direction: parseDirection(r.Direction),
		}
		// Only trust a parsed rule if it round-trips to the exact name we
		// generate — belt-and-suspenders against a hand-crafted rule that
		// merely starts with our prefix but doesn't match our own naming
		// scheme in every field.
		if rule.Name() == r.Name {
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

// Apply is idempotent: it deletes any existing rule with the same
// deterministic name first (netsh add doesn't upsert — adding a
// same-named rule twice creates two separate entries) before adding the
// new one, so re-running Apply for an unchanged Rule doesn't accumulate
// duplicates.
func (m windowsManager) Apply(ctx context.Context, rule Rule) error {
	_, _ = m.runNetsh(ctx, "advfirewall", "firewall", "delete", "rule", "name="+rule.Name())

	_, err := m.runNetsh(ctx, "advfirewall", "firewall", "add", "rule",
		"name="+rule.Name(),
		"dir="+string(rule.Direction),
		"action="+string(rule.Action),
		"protocol="+strings.ToUpper(rule.Proto),
		"localport="+strconv.Itoa(rule.Port),
		"profile=any",
	)
	return err
}

func (m windowsManager) Remove(ctx context.Context, rule Rule) error {
	_, err := m.runNetsh(ctx, "advfirewall", "firewall", "delete", "rule", "name="+rule.Name())
	return err
}

func (m windowsManager) RemoveAll(ctx context.Context) error {
	rules, err := m.List(ctx)
	if err != nil {
		return err
	}
	for _, r := range rules {
		if err := m.Remove(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func parseAction(s string) Action {
	if strings.EqualFold(s, "Block") {
		return ActionBlock
	}
	return ActionAllow
}

func parseDirection(s string) Direction {
	if strings.EqualFold(s, "Out") {
		return DirectionOut
	}
	return DirectionIn
}
