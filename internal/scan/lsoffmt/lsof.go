// Package lsoffmt parses macOS lsof/ps text formats. Like scan/procfmt, it
// has no build tags and does no shelling out of its own — scan_darwin.go
// and proc_darwin.go run the real commands and hand the output here, which
// is what lets this logic get real go test coverage on any host OS.
package lsoffmt

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Socket is one listening socket reported by
// `lsof -iTCP -sTCP:LISTEN -P -n -F pcnPu`.
type Socket struct {
	PID     int
	Command string
	UID     uint32
	Proto   string // raw "P" field value, e.g. "TCP"
	Addr    string // raw "n" field value, e.g. "*:3000" or "127.0.0.1:5432"
	Host    string
	Port    int // -1 if the port segment of Addr wasn't parseable
}

// ParseLsofFields parses lsof's `-F pcnPu` field output. Each line is a
// single-character field identifier immediately followed by its value (no
// separator). "p"/"c"/"u" describe the current process and apply to every
// socket line that follows until the next "p"; "P"/"n" describe one socket
// each, with "n" taken as the socket's terminator field (lsof always emits
// the fields in the order requested, and a matched listening socket has
// exactly one address to report).
func ParseLsofFields(r io.Reader) ([]Socket, error) {
	var out []Socket
	scanner := bufio.NewScanner(r)

	var pid int
	var command string
	var uid uint32
	var proto string
	haveProcess := false

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		key, val := line[0], line[1:]
		switch key {
		case 'p':
			n, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("lsoffmt: bad pid field %q: %w", line, err)
			}
			pid, command, uid, proto = n, "", 0, ""
			haveProcess = true
		case 'c':
			command = val
		case 'u':
			n, err := strconv.ParseUint(val, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("lsoffmt: bad uid field %q: %w", line, err)
			}
			uid = uint32(n)
		case 'P':
			proto = val
		case 'n':
			if !haveProcess {
				return nil, fmt.Errorf("lsoffmt: %q field before any process (p) field", line)
			}
			host, port := splitLsofAddr(val)
			out = append(out, Socket{
				PID: pid, Command: command, UID: uid,
				Proto: proto, Addr: val, Host: host, Port: port,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// splitLsofAddr splits an lsof "n" field value into host and port. Handles
// bare addresses ("*:3000", "127.0.0.1:5432") and bracketed IPv6
// ("[::1]:8080"). Returns port -1 when no parseable port segment exists.
func splitLsofAddr(s string) (host string, port int) {
	if strings.HasPrefix(s, "[") {
		if end := strings.Index(s, "]"); end >= 0 {
			host = s[:end+1]
			rest := s[end+1:]
			if strings.HasPrefix(rest, ":") {
				if p, err := strconv.Atoi(rest[1:]); err == nil {
					return host, p
				}
			}
			return host, -1
		}
	}

	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return s, -1
	}
	p, err := strconv.Atoi(s[idx+1:])
	if err != nil {
		return s, -1
	}
	return s[:idx], p
}
