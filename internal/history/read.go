package history

import (
	"encoding/json"
	"os"
	"strings"
)

// ReadAll reads and parses every entry in the history file at path, in
// file order (oldest first). A missing file returns an empty slice, not an
// error — no history yet is a normal, expected state. Malformed individual
// lines are skipped rather than failing the whole read, since this is a
// disposable log, not a critical data store.
func ReadAll(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []Entry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}
