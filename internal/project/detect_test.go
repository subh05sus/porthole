package project

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write fixture %s: %v", name, err)
	}
}

func TestDetectPackageJSON(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "zapmail-web", "version": "1.0.0"}`)

	if got := NewDetector().Detect(dir); got != "zapmail-web" {
		t.Fatalf("got %q, want zapmail-web", got)
	}
}

func TestDetectGoMod(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module github.com/subh05sus/porthole\n\ngo 1.23\n")

	if got := NewDetector().Detect(dir); got != "porthole" {
		t.Fatalf("got %q, want porthole", got)
	}
}

func TestDetectCargoToml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", "[package]\nname = \"anvil-js\"\nversion = \"0.1.0\"\n")

	if got := NewDetector().Detect(dir); got != "anvil-js" {
		t.Fatalf("got %q, want anvil-js", got)
	}
}

func TestDetectPyprojectToml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "[project]\nname = \"slotli\"\nversion = \"0.1.0\"\n")

	if got := NewDetector().Detect(dir); got != "slotli" {
		t.Fatalf("got %q, want slotli", got)
	}
}

func TestDetectGitDirectoryUsesDirName(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "my-cool-app")
	if err := os.MkdirAll(filepath.Join(projectDir, ".git"), 0o755); err != nil {
		t.Fatalf("failed to create fixture .git dir: %v", err)
	}

	if got := NewDetector().Detect(projectDir); got != "my-cool-app" {
		t.Fatalf("got %q, want my-cool-app", got)
	}
}

func TestDetectWalksUpToFindMarker(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/rootproject\n")
	nested := filepath.Join(dir, "cmd", "server")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	if got := NewDetector().Detect(nested); got != "rootproject" {
		t.Fatalf("got %q, want rootproject (found by walking up to the module root)", got)
	}
}

func TestDetectPackageJSONBeatsGoModWhenBothPresent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "wins"}`)
	writeFile(t, dir, "go.mod", "module example.com/loses\n")

	if got := NewDetector().Detect(dir); got != "wins" {
		t.Fatalf("got %q, want package.json's name to take priority per PRD §7.3", got)
	}
}

func TestDetectFallsBackToDirNameWhenNoMarkerFound(t *testing.T) {
	dir := t.TempDir()
	leaf := filepath.Join(dir, "some-random-folder")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatalf("failed to create fixture dir: %v", err)
	}

	got := NewDetector().Detect(leaf)
	if got != "some-random-folder" {
		t.Fatalf("got %q, want the fallback to be the leaf directory's own name", got)
	}
}

func TestDetectEmptyCWDIsSystem(t *testing.T) {
	if got := NewDetector().Detect(""); got != "system" {
		t.Fatalf("got %q, want system", got)
	}
}

func TestDetectUnreadableCWDIsSystem(t *testing.T) {
	if got := NewDetector().Detect(filepath.Join(t.TempDir(), "does-not-exist")); got != "system" {
		t.Fatalf("got %q, want system", got)
	}
}

func TestDetectCachesPerCWDForSession(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "cached-name"}`)

	d := NewDetector()
	first := d.Detect(dir)
	if first != "cached-name" {
		t.Fatalf("got %q, want cached-name", first)
	}

	// Remove the marker; a cached Detector must not re-read the filesystem.
	if err := os.Remove(filepath.Join(dir, "package.json")); err != nil {
		t.Fatalf("failed to remove fixture: %v", err)
	}

	second := d.Detect(dir)
	if second != "cached-name" {
		t.Fatalf("got %q on second call, want the cached value cached-name (cache not honored)", second)
	}
}
