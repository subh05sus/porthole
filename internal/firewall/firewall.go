// Package firewall manages a small, isolated set of firewall rules scoped
// to exactly what porthole already discovered in a scan — never arbitrary
// rules, CIDRs, or ports the user didn't already see. Every rule porthole
// creates carries the fixed RulePrefix name prefix, which is the only
// criterion porthole trusts to recognize "its own" rules for listing or
// cleanup; it never touches a rule it didn't create itself, no matter what
// the underlying OS firewall's own grouping mechanism does.
package firewall

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// RulePrefix is the fixed name prefix every rule porthole creates uses.
const RulePrefix = "porthole-"

// Action is what a Rule does to matching traffic.
type Action string

const (
	ActionBlock Action = "block"
	ActionAllow Action = "allow"
)

// Direction is which traffic direction a Rule applies to.
type Direction string

const (
	DirectionIn  Direction = "in"
	DirectionOut Direction = "out"
)

// Rule is one firewall rule porthole manages: exactly one port, one
// protocol — the same granularity a scan.Service already has, and no
// coarser or broader than that.
type Rule struct {
	Port      int
	Proto     string // "tcp" or "udp", matching scan.Proto's base value
	Action    Action
	Direction Direction
}

// Name returns the fixed, deterministic name porthole uses for this Rule.
// The same (direction, action, port, proto) always produces the same
// name, so re-applying an identical rule is idempotent — a second Apply
// call replaces the first rather than piling up duplicates.
func (r Rule) Name() string {
	return fmt.Sprintf("%s%s-%s-%d-%s", RulePrefix, r.Direction, r.Action, r.Port, r.Proto)
}

// ParseRuleName decodes a Name() string back into its Rule fields, and
// reports false unless it round-trips exactly — i.e. re-encoding the
// decoded Rule reproduces the exact same name. Used by managers whose OS
// tool doesn't hand back independently structured (direction, action,
// port, proto) fields the way Windows' `netsh show rule` does (see
// firewall_windows.go's extra defense-in-depth cross-check against
// those) — Linux's iptables comment-tag only round-trips through the
// encoded name itself, so this is the sole source of truth there.
func ParseRuleName(name string) (Rule, bool) {
	rest, ok := strings.CutPrefix(name, RulePrefix)
	if !ok {
		return Rule{}, false
	}
	parts := strings.SplitN(rest, "-", 4)
	if len(parts) != 4 {
		return Rule{}, false
	}

	direction := Direction(parts[0])
	if direction != DirectionIn && direction != DirectionOut {
		return Rule{}, false
	}
	action := Action(parts[1])
	if action != ActionBlock && action != ActionAllow {
		return Rule{}, false
	}
	port, err := strconv.Atoi(parts[2])
	if err != nil {
		return Rule{}, false
	}
	proto := parts[3]
	if proto != "tcp" && proto != "udp" {
		return Rule{}, false
	}

	rule := Rule{Port: port, Proto: proto, Action: action, Direction: direction}
	if rule.Name() != name {
		return Rule{}, false
	}
	return rule, true
}

// Manager applies, removes, and lists porthole-owned firewall rules. Every
// implementation must guarantee: (1) List/Remove/RemoveAll only ever touch
// rules porthole itself created (via the RulePrefix name), never anything
// pre-existing; (2) Apply replaces rather than duplicates an existing rule
// for the same (direction, action, port, proto).
type Manager interface {
	List(ctx context.Context) ([]Rule, error)
	Apply(ctx context.Context, rule Rule) error
	Remove(ctx context.Context, rule Rule) error
	RemoveAll(ctx context.Context) error
}
