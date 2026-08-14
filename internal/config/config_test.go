package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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
	if cfg.Theme != "auto" {
		t.Errorf("got Theme %q, want auto", cfg.Theme)
	}
	if cfg.Kill.DevPortRange != "3000-9999" {
		t.Errorf("got Kill.DevPortRange %q, want 3000-9999", cfg.Kill.DevPortRange)
	}
	if time.Duration(cfg.Kill.EscalationTimeout) != 2*time.Second {
		t.Errorf("got Kill.EscalationTimeout %s, want 2s", cfg.Kill.EscalationTimeout)
	}
	if time.Duration(cfg.Watch.Interval) != 2*time.Second {
		t.Errorf("got Watch.Interval %s, want 2s", cfg.Watch.Interval)
	}
	if time.Duration(cfg.AutoKill.Interval) != 5*time.Second {
		t.Errorf("got AutoKill.Interval %s, want 5s", cfg.AutoKill.Interval)
	}
	if cfg.Display != (Display{}) {
		t.Errorf("expected Display to be zero-value (show everything) by default, got %+v", cfg.Display)
	}
}

func TestLoadMixedProtectedPortForms(t *testing.T) {
	path := writeConfig(t, `
protected:
  - 5432
  - 6379
  - port: 3306
    reason: "prod tunnel, do not touch"

theme: plain
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
	if cfg.Theme != "plain" {
		t.Errorf("got Theme %q, want plain", cfg.Theme)
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

func TestLoadKillSectionOverridesDefaults(t *testing.T) {
	path := writeConfig(t, "kill:\n  dev_port_range: \"4000-4999\"\n  escalation_timeout: 5s\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Kill.DevPortRange != "4000-4999" {
		t.Errorf("got Kill.DevPortRange %q, want 4000-4999", cfg.Kill.DevPortRange)
	}
	if time.Duration(cfg.Kill.EscalationTimeout) != 5*time.Second {
		t.Errorf("got Kill.EscalationTimeout %s, want 5s", cfg.Kill.EscalationTimeout)
	}
}

func TestLoadInvalidDevPortRangeReturnsError(t *testing.T) {
	path := writeConfig(t, "kill:\n  dev_port_range: \"abc\"\n")

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected an error for an invalid dev_port_range")
	}
}

func TestLoadNegativeEscalationTimeoutReturnsError(t *testing.T) {
	// Syntactically valid duration, semantically invalid (must be > 0) —
	// caught by validate(), not by Duration's own UnmarshalYAML.
	path := writeConfig(t, "kill:\n  escalation_timeout: \"-5s\"\n")

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected an error for a negative escalation_timeout")
	}
}

func TestLoadInvalidDurationStringReturnsError(t *testing.T) {
	// Syntactically invalid duration — caught during yaml.Unmarshal by
	// Duration.UnmarshalYAML itself, a distinct failure stage from
	// validate()'s semantic checks (proven as a separate test case).
	path := writeConfig(t, "watch:\n  interval: \"banana\"\n")

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected an error for a syntactically invalid duration")
	}
}

func TestLoadWatchIntervalOverride(t *testing.T) {
	path := writeConfig(t, "watch:\n  interval: 10s\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Duration(cfg.Watch.Interval) != 10*time.Second {
		t.Errorf("got Watch.Interval %s, want 10s", cfg.Watch.Interval)
	}
}

func TestLoadZeroWatchIntervalReturnsError(t *testing.T) {
	path := writeConfig(t, "watch:\n  interval: \"0s\"\n")

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected an error for a zero watch.interval")
	}
}

func TestLoadAutoKillIntervalOverride(t *testing.T) {
	path := writeConfig(t, "auto_kill:\n  interval: 30s\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Duration(cfg.AutoKill.Interval) != 30*time.Second {
		t.Errorf("got AutoKill.Interval %s, want 30s", cfg.AutoKill.Interval)
	}
}

func TestLoadZeroAutoKillIntervalReturnsError(t *testing.T) {
	path := writeConfig(t, "auto_kill:\n  interval: \"0s\"\n")

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected an error for a zero auto_kill.interval")
	}
}

func TestLoadThemeAcceptsColorAndPlain(t *testing.T) {
	for _, theme := range []string{"auto", "color", "plain"} {
		path := writeConfig(t, "theme: "+theme+"\n")
		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("theme %q: unexpected error: %v", theme, err)
		}
		if cfg.Theme != theme {
			t.Errorf("theme %q: got %q", theme, cfg.Theme)
		}
	}
}

func TestLoadThemeRejectsInvalidValue(t *testing.T) {
	path := writeConfig(t, "theme: minimal\n")

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected an error for an invalid theme value")
	}
}

func TestLoadDisplaySectionHidesWhenSet(t *testing.T) {
	path := writeConfig(t, "display:\n  hide_system_processes: true\n  hide_privileged_ports: true\n")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Display.HideSystemProcesses || !cfg.Display.HidePrivilegedPorts {
		t.Errorf("got %+v, want both hide_* true", cfg.Display)
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

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", ".porthole.yaml")
	cfg := Config{
		Protected: []ProtectedPort{
			{Port: 5432},
			{Port: 3306, Reason: "prod tunnel"},
		},
		Theme:         "plain",
		Animations:    false,
		DefaultSignal: "SIGTERM",
		AutoKill: AutoKill{
			Enabled:  true,
			Allow:    []AutoKillEntry{{Port: 3000, Process: "node"}},
			Interval: Duration(10 * time.Second),
		},
		Kill: Kill{
			DevPortRange:      "9000-9010",
			EscalationTimeout: Duration(7 * time.Second),
		},
		Watch:   Watch{Interval: Duration(3 * time.Second)},
		Display: Display{HideSystemProcesses: true, HidePrivilegedPorts: true},
	}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Errorf("round-tripped config differs:\n got  %+v\n want %+v", got, cfg)
	}
}

func TestSaveRefusesInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".porthole.yaml")
	cfg := Config{Theme: "not-a-real-theme"}

	if err := Save(path, cfg); err == nil {
		t.Fatalf("expected Save to refuse an invalid config")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected no file to be written on a refused save, stat err = %v", err)
	}
}

func TestMarshalYAMLProtectedPortOmitsEmptyReason(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".porthole.yaml")
	cfg := Config{Protected: []ProtectedPort{{Port: 5432}}}
	cfg = withValidDefaults(cfg)

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "- 5432\n") {
		t.Errorf("expected a bare port entry in output YAML, got:\n%s", data)
	}
}

func TestMarshalYAMLDurationWritesHumanString(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".porthole.yaml")
	cfg := withValidDefaults(Config{Watch: Watch{Interval: Duration(90 * time.Second)}})

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: unexpected error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "interval: 1m30s") {
		t.Errorf("expected a human duration string in output YAML, got:\n%s", data)
	}
}

// withValidDefaults fills in whichever fields validate() requires but the
// caller doesn't care about, so tests can focus on the one field under
// test without Save rejecting the whole config as invalid.
func withValidDefaults(cfg Config) Config {
	base := defaults()
	if cfg.Theme == "" {
		cfg.Theme = base.Theme
	}
	if cfg.Kill.DevPortRange == "" {
		cfg.Kill.DevPortRange = base.Kill.DevPortRange
	}
	if cfg.Kill.EscalationTimeout <= 0 {
		cfg.Kill.EscalationTimeout = base.Kill.EscalationTimeout
	}
	if cfg.Watch.Interval <= 0 {
		cfg.Watch.Interval = base.Watch.Interval
	}
	if cfg.AutoKill.Interval <= 0 {
		cfg.AutoKill.Interval = base.AutoKill.Interval
	}
	return cfg
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
