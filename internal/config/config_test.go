package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".porthole.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write fixture config: %v", err)
	}
	return path
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Animations {
		t.Errorf("expected Animations default true")
	}
	if cfg.DefaultSignal != "SIGTERM" {
		t.Errorf("got DefaultSignal %q, want SIGTERM", cfg.DefaultSignal)
	}
	if len(cfg.Protected) != 0 {
		t.Errorf("expected no protected ports by default, got %+v", cfg.Protected)
	}
}

func TestLoadMixedProtectedPortForms(t *testing.T) {
	path := writeConfig(t, `
protected:
  - 5432
  - 6379
  - port: 3306
    reason: "prod tunnel, do not touch"

theme: minimal
animations: true
default_signal: SIGTERM
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Protected) != 3 {
		t.Fatalf("got %d protected entries, want 3: %+v", len(cfg.Protected), cfg.Protected)
	}
	if cfg.Protected[0].Port != 5432 || cfg.Protected[0].Reason != "" {
		t.Errorf("got %+v, want bare port 5432 with no reason", cfg.Protected[0])
	}
	if cfg.Protected[1].Port != 6379 || cfg.Protected[1].Reason != "" {
		t.Errorf("got %+v, want bare port 6379 with no reason", cfg.Protected[1])
	}
	if cfg.Protected[2].Port != 3306 || cfg.Protected[2].Reason != "prod tunnel, do not touch" {
		t.Errorf("got %+v, want port 3306 with a reason", cfg.Protected[2])
	}
	if cfg.Theme != "minimal" {
		t.Errorf("got Theme %q, want minimal", cfg.Theme)
	}
}

func TestLoadAnimationsFalseOverridesDefault(t *testing.T) {
	path := writeConfig(t, "animations: false\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Animations {
		t.Errorf("expected animations: false to override the true default")
	}
}

func TestLoadInvalidYAMLReturnsError(t *testing.T) {
	path := writeConfig(t, "protected: [this is not: valid: yaml:::")

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected an error for invalid YAML")
	}
}

func TestLoadInvalidProtectedEntryReturnsError(t *testing.T) {
	// A protected entry that's neither a bare scalar nor a {port, reason}
	// mapping (here, a YAML sequence) must error rather than silently
	// zeroing out.
	path := writeConfig(t, "protected:\n  - [1, 2, 3]\n")

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected an error for a protected entry that's neither a scalar nor a mapping")
	}
}

func TestLoadAutoKillSection(t *testing.T) {
	path := writeConfig(t, `
auto_kill:
  enabled: true
  allow:
    - port: 3000
      process: node
    - port: 8080
      process: python
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.AutoKill.Enabled {
		t.Fatalf("expected AutoKill.Enabled true")
	}
	if len(cfg.AutoKill.Allow) != 2 {
		t.Fatalf("got %d allow entries, want 2: %+v", len(cfg.AutoKill.Allow), cfg.AutoKill.Allow)
	}
	if cfg.AutoKill.Allow[0] != (AutoKillEntry{Port: 3000, Process: "node"}) {
		t.Errorf("got %+v", cfg.AutoKill.Allow[0])
	}
}

func TestAutoKillDisabledByDefault(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AutoKill.Enabled {
		t.Fatalf("expected AutoKill.Enabled false by default")
	}
}

func TestIsAutoKillAllowed(t *testing.T) {
	cfg := Config{AutoKill: AutoKill{
		Enabled: true,
		Allow:   []AutoKillEntry{{Port: 3000, Process: "node"}},
	}}

	if !cfg.IsAutoKillAllowed(3000, "node") {
		t.Errorf("expected an exact (port, process) match to be allowed")
	}
	if !cfg.IsAutoKillAllowed(3000, "Node") {
		t.Errorf("expected process matching to be case-insensitive")
	}
	if cfg.IsAutoKillAllowed(3000, "python") {
		t.Errorf("expected a different process on the same port to be disallowed")
	}
	if cfg.IsAutoKillAllowed(4000, "node") {
		t.Errorf("expected a different port to be disallowed")
	}
}

func TestIsAutoKillAllowedFalseWhenDisabledEvenWithMatchingEntry(t *testing.T) {
	// The single most important safety property: an allow-list entry
	// alone is never enough — Enabled must also be explicitly true.
	cfg := Config{AutoKill: AutoKill{
		Enabled: false,
		Allow:   []AutoKillEntry{{Port: 3000, Process: "node"}},
	}}

	if cfg.IsAutoKillAllowed(3000, "node") {
		t.Fatalf("expected IsAutoKillAllowed to refuse when AutoKill.Enabled is false, regardless of Allow contents")
	}
}

func TestIsProtected(t *testing.T) {
	cfg := Config{Protected: []ProtectedPort{{Port: 5432, Reason: "postgres"}, {Port: 6379}}}

	if protected, reason := cfg.IsProtected(5432); !protected || reason != "postgres" {
		t.Errorf("got protected=%v reason=%q, want true/postgres", protected, reason)
	}
	if protected, _ := cfg.IsProtected(6379); !protected {
		t.Errorf("expected 6379 to be protected")
	}
	if protected, _ := cfg.IsProtected(3000); protected {
		t.Errorf("expected 3000 to not be protected")
	}
}
