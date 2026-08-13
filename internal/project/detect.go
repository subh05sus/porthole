// Package project resolves a project name from a process's working
// directory — the thing PRD.md calls out as porthole's actual
// differentiator: "node on port 3000" is useless, "zapmail-web on port
// 3000" is the answer you wanted.
package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Detector resolves project names, caching by cwd for the session (PRD
// §7.3). Safe for concurrent use.
type Detector struct {
	mu    sync.Mutex
	cache map[string]string
}

// NewDetector returns a Detector with an empty session cache.
func NewDetector() *Detector {
	return &Detector{cache: make(map[string]string)}
}

// Detect resolves cwd to a project name. Walking upward from cwd, the
// first marker file found wins: package.json (its "name" field) ->
// go.mod (module path's last segment) -> Cargo.toml -> pyproject.toml ->
// .git (that directory's own name). If nothing matches anywhere up to the
// filesystem root, it falls back to cwd's own directory name; an empty,
// root, or unreadable cwd resolves to "system".
func (d *Detector) Detect(cwd string) string {
	if cwd == "" {
		return "system"
	}

	d.mu.Lock()
	if name, ok := d.cache[cwd]; ok {
		d.mu.Unlock()
		return name
	}
	d.mu.Unlock()

	name := detect(cwd)

	d.mu.Lock()
	d.cache[cwd] = name
	d.mu.Unlock()
	return name
}

func detect(cwd string) string {
	dir := cwd
	for {
		if name, ok := tryMarkers(dir); ok {
			return name
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached the filesystem root without finding a marker
		}
		dir = parent
	}
	return fallbackName(cwd)
}

func tryMarkers(dir string) (string, bool) {
	if name, ok := readPackageJSONName(filepath.Join(dir, "package.json")); ok {
		return name, true
	}
	if name, ok := readGoModName(filepath.Join(dir, "go.mod")); ok {
		return name, true
	}
	if name, ok := readTOMLNameField(filepath.Join(dir, "Cargo.toml")); ok {
		return name, true
	}
	if name, ok := readTOMLNameField(filepath.Join(dir, "pyproject.toml")); ok {
		return name, true
	}
	if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && info != nil {
		return filepath.Base(dir), true
	}
	return "", false
}

// fallbackName is PRD §7.3's last resort: the enclosing directory's own
// name, or "system" if cwd is unreadable or the filesystem root.
func fallbackName(cwd string) string {
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		return "system"
	}
	if parent := filepath.Dir(cwd); parent == cwd {
		return "system" // cwd is the filesystem root
	}
	base := filepath.Base(cwd)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "system"
	}
	return base
}

func readPackageJSONName(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil || pkg.Name == "" {
		return "", false
	}
	return pkg.Name, true
}

func readGoModName(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		after, ok := strings.CutPrefix(line, "module ")
		if !ok {
			continue
		}
		modPath := strings.Trim(strings.TrimSpace(after), `"`)
		if modPath == "" {
			return "", false
		}
		segments := strings.Split(modPath, "/")
		return segments[len(segments)-1], true
	}
	return "", false
}

// readTOMLNameField extracts a "name = "value"" line via minimal line
// scanning rather than a full TOML parser — this is the only field either
// Cargo.toml or pyproject.toml ever needs here, so a real TOML dependency
// isn't worth adding for it (see FUTURE_PLANS.md tech debt notes).
func readTOMLNameField(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "name") {
			continue
		}
		rest, ok := strings.CutPrefix(strings.TrimSpace(strings.TrimPrefix(line, "name")), "=")
		if !ok {
			continue
		}
		if name, ok := unquoteTOMLString(strings.TrimSpace(rest)); ok && name != "" {
			return name, true
		}
	}
	return "", false
}

func unquoteTOMLString(s string) (string, bool) {
	if len(s) < 2 {
		return "", false
	}
	q := s[0]
	if q != '"' && q != '\'' {
		return "", false
	}
	end := strings.IndexByte(s[1:], q)
	if end < 0 {
		return "", false
	}
	return s[1 : end+1], true
}
