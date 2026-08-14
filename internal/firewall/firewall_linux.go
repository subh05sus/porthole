//go:build linux

package firewall

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// portholeChain is the dedicated iptables chain porthole exclusively
// owns — every rule porthole creates lives here, never directly in
// INPUT/OUTPUT, so porthole never has to reason about or touch a
// pre-existing rule. INPUT/OUTPUT each get exactly one jump rule to it.
const portholeChain = "PORTHOLE"

type linuxManager struct {
	run func(ctx context.Context, args ...string) (string, error)
}

// NewDefaultManager returns the Linux firewall manager, shelling out to
// iptables. [compiles] only — no Linux hardware available this session to
// live-verify against a real netfilter ruleset, unlike the Windows
// manager (see firewall_windows.go's live-verification notes).
func NewDefaultManager() Manager {
	return linuxManager{run: execIptables}
}

func execIptables(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "iptables", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return stdout.String(), fmt.Errorf("firewall: iptables %s: %w (%s)", strings.Join(args, " "), err, msg)
	}
	return stdout.String(), nil
}

func builtinChain(d Direction) string {
	if d == DirectionOut {
		return "OUTPUT"
	}
	return "INPUT"
}

// portMatchFlag reflects a real asymmetry, not an arbitrary choice: a
// locally-listening port shows up as the *destination* port on inbound
// packets (--dport, matched from INPUT) but as the *source* port on this
// process's own outbound traffic (--sport, matched from OUTPUT) — using
// --dport for both directions would silently make an Out rule a no-op.
func portMatchFlag(d Direction) string {
	if d == DirectionOut {
		return "--sport"
	}
	return "--dport"
}

func actionTarget(a Action) string {
	if a == ActionBlock {
		return "DROP"
	}
	return "ACCEPT"
}

// ensureHooked makes sure the dedicated chain exists and that builtin
// jumps to it, idempotently — safe to call before every Apply.
func (m linuxManager) ensureHooked(ctx context.Context, builtin string) error {
	_, _ = m.run(ctx, "-N", portholeChain) // ignore "chain already exists"
	if _, err := m.run(ctx, "-C", builtin, "-j", portholeChain); err != nil {
		// -C (check) errors if the jump doesn't exist yet — add it.
		if _, err := m.run(ctx, "-I", builtin, "-j", portholeChain); err != nil {
			return err
		}
	}
	return nil
}

// Apply is idempotent: it removes any existing rule with this exact name
// first (see Remove) before appending the new one — iptables has no
// upsert, appending twice would duplicate.
func (m linuxManager) Apply(ctx context.Context, rule Rule) error {
	if err := m.ensureHooked(ctx, builtinChain(rule.Direction)); err != nil {
		return err
	}
	_ = m.Remove(ctx, rule)

	_, err := m.run(ctx, "-A", portholeChain,
		"-p", rule.Proto,
		portMatchFlag(rule.Direction), strconv.Itoa(rule.Port),
		"-m", "comment", "--comment", rule.Name(),
		"-j", actionTarget(rule.Action),
	)
	return err
}

// Remove looks the rule up by its exact comment tag and deletes it by
// line number, rather than trying to reconstruct-and-match the full rule
// spec blind — iptables -D needs an exact spec match, which would be
// brittle to keep in sync with Apply's own rule construction.
func (m linuxManager) Remove(ctx context.Context, rule Rule) error {
	line, ok, err := m.findLine(ctx, rule.Name())
	if err != nil {
		return err
	}
	if !ok {
		return nil // already gone — Remove is idempotent
	}
	_, err = m.run(ctx, "-D", portholeChain, strconv.Itoa(line))
	return err
}

func (m linuxManager) RemoveAll(ctx context.Context) error {
	rules, err := m.List(ctx)
	if err != nil {
		return err
	}
	// Delete in reverse so earlier rules' line numbers don't shift out
	// from under a subsequent lookup within this same loop.
	for i := len(rules) - 1; i >= 0; i-- {
		if err := m.Remove(ctx, rules[i]); err != nil {
			return err
		}
	}
	return nil
}

func (m linuxManager) List(ctx context.Context) ([]Rule, error) {
	out, err := m.run(ctx, "-L", portholeChain, "-n", "--line-numbers")
	if err != nil {
		// The chain not existing yet (nothing ever applied) isn't a real
		// error — it just means zero rules.
		return nil, nil
	}

	var rules []Rule
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		name, ok := commentFromLine(line)
		if !ok {
			continue
		}
		if rule, ok := ParseRuleName(name); ok {
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

// findLine re-lists and returns the line number of the rule tagged with
// name, if present.
func (m linuxManager) findLine(ctx context.Context, name string) (int, bool, error) {
	out, err := m.run(ctx, "-L", portholeChain, "-n", "--line-numbers")
	if err != nil {
		return 0, false, nil // chain doesn't exist — nothing to find
	}
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		got, ok := commentFromLine(line)
		if !ok || got != name {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		return n, true, nil
	}
	return 0, false, nil
}

// commentFromLine extracts a `/* ... */` trailing comment from one line
// of `iptables -L ... -n --line-numbers` output.
func commentFromLine(line string) (string, bool) {
	start := strings.Index(line, "/*")
	if start < 0 {
		return "", false
	}
	rest := line[start+2:]
	end := strings.Index(rest, "*/")
	if end < 0 {
		return "", false
	}
	return strings.TrimSpace(rest[:end]), true
}
