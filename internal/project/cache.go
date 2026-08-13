package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// diskCacheEntry is one persisted resolution: the resolved name, plus
// enough to know when it's stale (the marker file that produced it, and
// its mtime at resolution time).
type diskCacheEntry struct {
	Name          string    `json:"name"`
	MarkerPath    string    `json:"marker_path,omitempty"`
	MarkerModTime time.Time `json:"marker_mod_time,omitempty"`
}

// diskCache persists project-name resolutions across porthole invocations,
// so a machine with many repos doesn't re-walk the filesystem on every cold
// start. It is a pure optimization: any failure to read or write it must
// degrade silently to "no persistent cache" rather than surface as an
// error, since Detect still works correctly (just slower) without it.
type diskCache struct {
	mu      sync.Mutex
	path    string
	entries map[string]diskCacheEntry
}

// cacheFilePath returns the real on-disk cache location, or "" if the
// user's cache directory can't be determined (e.g. a minimal container
// environment) — callers must treat "" as "persistence unavailable."
func cacheFilePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "porthole", "project-cache.json")
}

func loadDiskCache(path string) *diskCache {
	c := &diskCache{path: path, entries: make(map[string]diskCacheEntry)}
	if path == "" {
		return c
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	// A corrupt cache file starts fresh rather than failing — it's
	// disposable, derived data.
	_ = json.Unmarshal(data, &c.entries)
	return c
}

func (c *diskCache) get(cwd string) (diskCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[cwd]
	return e, ok
}

// put stores an entry and immediately persists the whole cache (write-
// through). Simpler and safer than batching + flush-on-exit, which would
// need a process-exit hook this package has no natural place to install;
// real-world entry counts (tens of projects per machine) make a full
// rewrite per new entry cheap enough not to matter.
func (c *diskCache) put(cwd string, entry diskCacheEntry) {
	c.mu.Lock()
	c.entries[cwd] = entry
	data, err := json.Marshal(c.entries)
	path := c.path
	c.mu.Unlock()

	if path == "" || err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// markerStillValid reports whether entry's marker file is unchanged since
// it was cached. An entry with no marker (the directory-name fallback) is
// always considered valid, since there's nothing to invalidate it against.
func markerStillValid(entry diskCacheEntry) bool {
	if entry.MarkerPath == "" {
		return true
	}
	info, err := os.Stat(entry.MarkerPath)
	if err != nil {
		return false
	}
	return info.ModTime().Equal(entry.MarkerModTime)
}
