package procfmt

import (
	"bytes"
	"strings"
)

// ParseCmdline parses /proc/[pid]/cmdline: a NUL-separated, NUL-terminated
// list of argv strings (not shell-quoted — argv[0] and each argument are
// exactly as the process received them).
func ParseCmdline(data []byte) []string {
	data = bytes.TrimRight(data, "\x00")
	if len(data) == 0 {
		return nil
	}
	return strings.Split(string(data), "\x00")
}
