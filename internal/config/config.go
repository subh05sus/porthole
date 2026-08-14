// Package config loads porthole's optional ~/.porthole.yaml — protected
// ports (FUTURE_PLANS.md v1.3) plus a few display/behavior settings. The
// file is entirely optional: an absent file yields sensible defaults, not
// an error, per that doc's own "config is optional, tool works fine
// without it" principle.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/subh05sus/porthole/internal/portrange"
)

// Config is the parsed contents of ~/.porthole.yaml.
type Config struct {
	Protected     []ProtectedPort `yaml:"protected"`
	Theme         string          `yaml:"theme"` // "auto" | "color" | "plain"
	Animations    bool            `yaml:"animations"`
	DefaultSignal string          `yaml:"default_signal"` // parsed, not yet wired to any behavior — see ROADMAP.md
	AutoKill      AutoKill        `yaml:"auto_kill"`
	Kill          Kill            `yaml:"kill"`
	Watch         Watch           `yaml:"watch"`
	Display       Display         `yaml:"display"`
}

// AutoKill configures `porthole daemon`. Disabled by default even if this
// section is present but Enabled is left unset, and Allow is the *only*
// way a (port, process) pair becomes eligible: no wildcards, no ranges,
// ever.
type AutoKill struct {
	Enabled  bool            `yaml:"enabled"`
	Allow    []AutoKillEntry `yaml:"allow"`
	Interval Duration        `yaml:"interval"` // daemon poll cadence; the 30s per-port cooldown after a
	// kill attempt is a separate, unconfigurable constant in
	// internal/daemon — not this field.
}

// Kill configures behavior shared by the kill ladder and the --dev sweep.
type Kill struct {
	// DevPortRange is the port range `porthole kill --dev` sweeps, as a
	// single "N-M" (or bare "N") token in the same grammar --dev's own
	// range parsing already accepts.
	DevPortRange string `yaml:"dev_port_range"`
	// EscalationTimeout is how long the kill ladder waits after a graceful
	// signal before it's eligible to escalate to a forceful one.
	EscalationTimeout Duration `yaml:"escalation_timeout"`
}

// Watch configures `porthole watch`'s default poll cadence.
type Watch struct {
	Interval Duration `yaml:"interval"`
}

// Display holds display-only filters — they affect what list/watch/the TUI
// render, never what `porthole kill <port>` can target. Fields are named
// Hide*, not Show*, so the Go zero value (false) means "hide nothing,"
// i.e. today's behavior: every discovered service is always shown.
type Display struct {
	// HideSystemProcesses hides rows the current invocation doesn't own
	// (Service.Owned == false) — typically system services running as
	// another user/root. This is relative to *this run's* privilege
	// level, not an absolute property of a process: running porthole
	// elevated reveals more rows, not fewer, since more of them become
	// yours.
	HideSystemProcesses bool `yaml:"hide_system_processes"`
	// HidePrivilegedPorts hides rows on the classic Unix "well-known"
	// port range (below 1024), independent of ownership.
	HidePrivilegedPorts bool `yaml:"hide_privileged_ports"`
}

// Duration unmarshals from a human duration string ("5s", "2m30s" — the
// same syntax time.ParseDuration/Cobra's DurationVar flags accept) rather
// than the bare integer-nanosecond count yaml.v3 would otherwise expect
// for a plain time.Duration field.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return fmt.Errorf("config: invalid duration at line %d: %w", node.Line, err)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("config: invalid duration %q at line %d: %w", s, node.Line, err)
	}
	*d = Duration(parsed)
	return nil
}

// String lets Duration format as "5s" rather than a raw nanosecond count
// in error messages — it doesn't inherit time.Duration's String() since
// it's a distinct defined type.
func (d Duration) String() string { return time.Duration(d).String() }

// AutoKillEntry is one exact (port, process-name) pair the daemon is
// allowed to act on. Process is matched case-insensitively (process-name
// casing varies across platforms, e.g. "node" vs "Node.exe") but
// otherwise exactly — never a prefix, substring, or pattern match.
type AutoKillEntry struct {
	Port    int    `yaml:"port"`
	Process string `yaml:"process"`
}

// ProtectedPort is one entry in the protected list. The YAML form accepts
// either a bare port number or a {port, reason} mapping — see
// UnmarshalYAML.
type ProtectedPort struct {
	Port   int
	Reason string
}

// UnmarshalYAML accepts either a bare scalar port ("- 5432") or a mapping
// with an optional reason ("- port: 3306\n  reason: ..."), matching
// FUTURE_PLANS.md v1.3's example config, which mixes both forms in the
// same list.
func (p *ProtectedPort) UnmarshalYAML(node *yaml.Node) error {
	var port int
	if err := node.Decode(&port); err == nil {
		p.Port = port
		p.Reason = ""
		return nil
	}

	var raw struct {
		Port   int    `yaml:"port"`
		Reason string `yaml:"reason"`
	}
	if err := node.Decode(&raw); err != nil {
		return fmt.Errorf("config: invalid protected port entry at line %d: %w", node.Line, err)
	}
	p.Port = raw.Port
	p.Reason = raw.Reason
	return nil
}

// DefaultPath returns ~/.porthole.yaml for the current user.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: resolving home directory: %w", err)
	}
	return filepath.Join(home, ".porthole.yaml"), nil
}

// defaults returns a Config with sensible defaults, used both when the
// file is absent and as the base a present file's fields overlay onto —
// yaml.Unmarshal only touches fields actually present in the document, so
// pre-populating these here is what makes "animations: true unless the
// file explicitly disables it" work without needing pointer fields.
func defaults() Config {
	return Config{
		Theme:         "auto",
		Animations:    true,
		DefaultSignal: "SIGTERM",
		Kill: Kill{
			DevPortRange:      "3000-9999",
			EscalationTimeout: Duration(2 * time.Second),
		},
		Watch: Watch{
			Interval: Duration(2 * time.Second),
		},
		AutoKill: AutoKill{
			Interval: Duration(5 * time.Second),
		},
	}
}

// Load reads and parses the config file at path. A missing file returns
// defaults with no error; a present-but-invalid file returns an error —
// the tool should fail loudly on a config the user clearly tried to write
// but got wrong, rather than silently ignoring it. This includes semantic
// validation (validate()), not just YAML syntax: a config that parses
// fine but sets e.g. a zero watch interval must still fail loudly here,
// not panic later inside a background goroutine.
func Load(path string) (Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("config: reading %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config: parsing %s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return cfg, fmt.Errorf("config: validating %s: %w", path, err)
	}
	return cfg, nil
}

// validate checks semantic constraints yaml.Unmarshal's own field-level
// UnmarshalYAML hooks can't (those only see one field's raw value, not the
// full picture, and some checks — like "must be positive" — apply after
// successful parsing, not during it).
func (c Config) validate() error {
	switch c.Theme {
	case "auto", "color", "plain":
	default:
		return fmt.Errorf("theme: invalid value %q, want \"auto\", \"color\", or \"plain\"", c.Theme)
	}

	if _, err := portrange.Parse(c.Kill.DevPortRange); err != nil {
		return fmt.Errorf("kill.dev_port_range: %w", err)
	}
	if c.Kill.EscalationTimeout <= 0 {
		return fmt.Errorf("kill.escalation_timeout: must be greater than zero, got %s", c.Kill.EscalationTimeout)
	}
	// A zero/negative interval reaches time.NewTicker (in scan.Watch and
	// daemon.Daemon.Run) and panics there — this must fail loudly here
	// instead, at config-load time, not at runtime inside a goroutine.
	if c.Watch.Interval <= 0 {
		return fmt.Errorf("watch.interval: must be greater than zero, got %s", c.Watch.Interval)
	}
	if c.AutoKill.Interval <= 0 {
		return fmt.Errorf("auto_kill.interval: must be greater than zero, got %s", c.AutoKill.Interval)
	}
	return nil
}

// LoadDefault loads from DefaultPath.
func LoadDefault() (Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return defaults(), err
	}
	return Load(path)
}

// IsProtected reports whether port is in the protected list, and its
// reason if one was given.
func (c Config) IsProtected(port int) (protected bool, reason string) {
	for _, p := range c.Protected {
		if p.Port == port {
			return true, p.Reason
		}
	}
	return false, ""
}

// IsAutoKillAllowed reports whether (port, process) exactly matches an
// entry in the auto-kill allow-list. Checks AutoKill.Enabled itself, so
// callers never need to remember to check it separately — a Config with
// Enabled left false always returns false here regardless of what's in
// Allow, matching the "disabled by default" design.
func (c Config) IsAutoKillAllowed(port int, process string) bool {
	if !c.AutoKill.Enabled {
		return false
	}
	for _, e := range c.AutoKill.Allow {
		if e.Port == port && strings.EqualFold(e.Process, process) {
			return true
		}
	}
	return false
}
