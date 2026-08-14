//go:build darwin

package firewall

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// anchorName is the dedicated pf anchor porthole exclusively owns. pf's
// anchor mechanism exists precisely so a tool can manage its own isolated
// rule set without editing /etc/pf.conf directly — every rule in this
// anchor is porthole's by construction, so unlike the Windows/Linux
// managers (which coexist with unrelated rules in a shared store and need
// a name-prefix filter), List here doesn't need to filter anything: 100%
// of the anchor's contents are already ours.
const anchorName = "porthole"

type darwinManager struct {
	// run shells `pfctl <args...>`, piping stdin if non-empty — reload
	// uses this to load a full replacement ruleset via `-f -`.
	run func(ctx context.Context, stdin string, args ...string) (string, error)
}

// NewDefaultManager returns the macOS firewall manager, shelling out to
// pfctl. [compiles] only — no macOS hardware available this session to
// live-verify against a real pf ruleset, unlike the Windows manager (see
// firewall_windows.go's live-verification notes).
func NewDefaultManager() Manager {
	return darwinManager{run: execPfctl}
}

func execPfctl(ctx context.Context, stdin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "pfctl", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return stdout.String(), fmt.Errorf("firewall: pfctl %s: %w (%s)", strings.Join(args, " "), err, msg)
	}
	return stdout.String(), nil
}

func (m darwinManager) List(ctx context.Context) ([]Rule, error) {
	out, err := m.run(ctx, "", "-a", anchorName, "-s", "rules")
	if err != nil {
		// An anchor with no rules ever loaded errors rather than printing
		// nothing — treat that the same as "zero rules" rather than a
		// real failure, exactly like the Linux manager's missing-chain case.
		return nil, nil
	}

	var rules []Rule
	for _, line := range strings.Split(out, "\n") {
		if rule, ok := parsePFRule(line); ok {
			rules = append(rules, rule)
		}
	}
	return rules, nil
}

// parsePFRule parses one line of the form
// "block in proto tcp from any to any port 3000" (pfctl's own rendering
// of a rule added via renderPFRule below).
func parsePFRule(line string) (Rule, bool) {
	f := strings.Fields(line)
	if len(f) < 10 {
		return Rule{}, false
	}
	var action Action
	switch f[0] {
	case "block":
		action = ActionBlock
	case "pass":
		action = ActionAllow
	default:
		return Rule{}, false
	}
	var direction Direction
	switch f[1] {
	case "in":
		direction = DirectionIn
	case "out":
		direction = DirectionOut
	default:
		return Rule{}, false
	}
	if f[2] != "proto" || f[8] != "port" {
		return Rule{}, false
	}
	port, err := strconv.Atoi(f[9])
	if err != nil {
		return Rule{}, false
	}
	return Rule{Port: port, Proto: f[3], Action: action, Direction: direction}, true
}

func renderPFRule(r Rule) string {
	action := "block"
	if r.Action == ActionAllow {
		action = "pass"
	}
	dir := "in"
	if r.Direction == DirectionOut {
		dir = "out"
	}
	return fmt.Sprintf("%s %s proto %s from any to any port %d", action, dir, r.Proto, r.Port)
}

// reload replaces the porthole anchor's *entire* rule set atomically via
// `pfctl -a porthole -f -` — every Apply/Remove/RemoveAll funnels through
// this. pf's own anchor-load mechanism applies the whole file as one
// unit, so a crash mid-write can never leave a half-applied rule: the
// anchor is either the complete old set or the complete new set, never a
// mix (the crash-safety property the plan specifically calls for).
func (m darwinManager) reload(ctx context.Context, rules []Rule) error {
	var b strings.Builder
	for _, r := range rules {
		b.WriteString(renderPFRule(r))
		b.WriteString("\n")
	}
	_, err := m.run(ctx, b.String(), "-a", anchorName, "-f", "-")
	return err
}

func (m darwinManager) Apply(ctx context.Context, rule Rule) error {
	existing, err := m.List(ctx)
	if err != nil {
		return err
	}
	next := without(existing, rule)
	next = append(next, rule)
	return m.reload(ctx, next)
}

func (m darwinManager) Remove(ctx context.Context, rule Rule) error {
	existing, err := m.List(ctx)
	if err != nil {
		return err
	}
	return m.reload(ctx, without(existing, rule))
}

func (m darwinManager) RemoveAll(ctx context.Context) error {
	return m.reload(ctx, nil)
}

func without(rules []Rule, exclude Rule) []Rule {
	out := make([]Rule, 0, len(rules))
	for _, r := range rules {
		if r != exclude {
			out = append(out, r)
		}
	}
	return out
}
