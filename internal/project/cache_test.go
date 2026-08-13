package project

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentCacheSurvivesAcrossDetectorInstances(t *testing.T) {
	dir := t.TempDir()
	markerPath := filepath.Join(dir, "package.json")
	writeFile(t, dir, "package.json", `{"name": "persisted-name"}`)
	cachePath := filepath.Join(t.TempDir(), "project-cache.json")

	first := newDetectorWithCache(loadDiskCache(cachePath))
	if got := first.Detect(dir); got != "persisted-name" {
		t.Fatalf("got %q, want persisted-name", got)
	}

	origInfo, err := os.Stat(markerPath)
	if err != nil {
		t.Fatalf("failed to stat marker: %v", err)
	}
	origModTime := origInfo.ModTime()

	// Overwrite the marker's *content* but restore its original mtime
	// afterward. A fresh Detector reading the disk cache should trust the
	// unchanged mtime and return the originally cached name — proving it
	// reused the persisted entry rather than re-parsing package.json,
	// which would return "changed-name" instead.
	writeFile(t, dir, "package.json", `{"name": "changed-name"}`)
	if err := os.Chtimes(markerPath, origModTime, origModTime); err != nil {
		t.Fatalf("failed to restore marker mtime: %v", err)
	}

	second := newDetectorWithCache(loadDiskCache(cachePath))
	if got := second.Detect(dir); got != "persisted-name" {
		t.Fatalf("got %q from a fresh Detector instance with unchanged marker mtime, want the disk-persisted persisted-name (re-read the file instead of trusting the cache)", got)
	}
}

func TestPersistentCacheInvalidatesOnMarkerChange(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name": "old-name"}`)
	cachePath := filepath.Join(t.TempDir(), "project-cache.json")

	first := newDetectorWithCache(loadDiskCache(cachePath))
	if got := first.Detect(dir); got != "old-name" {
		t.Fatalf("got %q, want old-name", got)
	}

	// Ensure a detectable mtime change (some filesystems have 1s+
	// resolution), then rewrite the marker with a different name.
	time.Sleep(1100 * time.Millisecond)
	writeFile(t, dir, "package.json", `{"name": "new-name"}`)

	second := newDetectorWithCache(loadDiskCache(cachePath))
	if got := second.Detect(dir); got != "new-name" {
		t.Fatalf("got %q, want new-name (stale disk cache entry must be invalidated by mtime change)", got)
	}
}

func TestPersistentCacheMissingCacheDirDegradesGracefully(t *testing.T) {
	// A cache path under a directory that doesn't exist and can't be
	// created (empty path signals "no persistence" per cacheFilePath's
	// contract) must never break Detect — just skip persistence.
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/nopersist\n")

	d := newDetectorWithCache(loadDiskCache(""))
	if got := d.Detect(dir); got != "nopersist" {
		t.Fatalf("got %q, want nopersist even with persistence disabled", got)
	}
}

func TestPersistentCacheCorruptFileStartsEmpty(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "project-cache.json")
	if err := os.WriteFile(cachePath, []byte("not valid json{{{"), 0o644); err != nil {
		t.Fatalf("failed to write corrupt fixture: %v", err)
	}

	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/afterCorrupt\n")

	d := newDetectorWithCache(loadDiskCache(cachePath))
	if got := d.Detect(dir); got != "afterCorrupt" {
		t.Fatalf("got %q, want afterCorrupt (corrupt cache file must not break Detect)", got)
	}
}
