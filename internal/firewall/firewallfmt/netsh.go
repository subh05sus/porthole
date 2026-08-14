// Package firewallfmt parses `netsh advfirewall firewall show rule`
// output. Like scan/procfmt and scan/lsoffmt, it has no build tags and
// does no shelling out of its own, which is what lets this logic get real
// go test coverage on any host OS, including this Windows dev box.
package firewallfmt

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// NetshRule is one rule block from `netsh advfirewall firewall show rule`.
type NetshRule struct {
	Name      string
	Direction string // "In" or "Out"
	Protocol  string // "TCP", "UDP", ...
	LocalPort string // a literal port number, "Any", or a comma list — kept as the raw string, callers decide what to trust
	Action    string // "Allow" or "Block"
}

// ParseNetshRules parses the block-formatted output of
// `netsh advfirewall firewall show rule name=all`. Each rule is a
// "Key:    Value" block separated by blank lines (or a "---" divider
// directly under "Rule Name:"); unrecognized keys are ignored, and a rule
// missing its name is skipped rather than failing the whole parse — the
// same "skip the bad row, keep going" resilience procfmt.ParseTCPTable
// uses for a transient kernel-file read anomaly.
func ParseNetshRules(r io.Reader) ([]NetshRule, error) {
	var rules []NetshRule
	var cur NetshRule
	have := false

	flush := func() {
		if have && cur.Name != "" {
			rules = append(rules, cur)
		}
		cur = NetshRule{}
		have = false
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "----") {
			continue
		}
		if trimmed == "" {
			// A blank line ends the current block. Flushing here (not just
			// on the next "Rule Name:") matters: without it, stray fields
			// in a malformed trailing block with no name of its own would
			// silently merge into whatever rule came before it instead of
			// being discarded.
			flush()
			continue
		}

		key, val, ok := splitKeyValue(line)
		if !ok {
			continue
		}

		if key == "Rule Name" {
			flush()
			cur.Name = val
			have = true
			continue
		}
		if !have {
			continue // a field before any "Rule Name:" line — not a rule block we recognize
		}

		switch key {
		case "Direction":
			cur.Direction = val
		case "Protocol":
			cur.Protocol = val
		case "LocalPort":
			cur.LocalPort = val
		case "Action":
			cur.Action = val
		}
	}
	flush()

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

// LocalPortNumber returns the rule's LocalPort as an int, and false if
// it's "Any", a range, or otherwise not a single literal port number —
// which is all porthole itself ever creates, but a rule listed alongside
// pre-existing ones (before name-prefix filtering) might not be.
func (r NetshRule) LocalPortNumber() (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(r.LocalPort))
	if err != nil {
		return 0, false
	}
	return n, true
}

// splitKeyValue splits a "Key:    Value" line. netsh pads keys with
// spaces before the colon in some locales/versions and not others, so
// this trims rather than assuming a fixed column width.
func splitKeyValue(line string) (key, val string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	val = strings.TrimSpace(line[idx+1:])
	if key == "" {
		return "", "", false
	}
	return key, val, true
}
