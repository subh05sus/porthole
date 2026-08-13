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

	"gopkg.in/yaml.v3"
)

// Config is the parsed contents of ~/.porthole.yaml.
type Config struct {
	Protected     []ProtectedPort `yaml:"protected"`
	Theme         string          `yaml:"theme"` // "minimal" | "full" | "plain"
	Animations    bool            `yaml:"animations"`
	DefaultSignal string          `yaml:"default_signal"`
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
	return Config{Animations: true, DefaultSignal: "SIGTERM"}
}

// Load reads and parses the config file at path. A missing file returns
// defaults with no error; a present-but-invalid file returns an error —
// the tool should fail loudly on a config the user clearly tried to write
// but got wrong, rather than silently ignoring it.
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
	return cfg, nil
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
