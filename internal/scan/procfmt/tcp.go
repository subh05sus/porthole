// Package procfmt parses Linux /proc text formats. It has no build tags and
// does no file I/O of its own — scan_linux.go and proc_linux.go read the
// real files and hand the bytes here, which is what lets this logic get
// real go test coverage on any host OS, including this Windows dev box.
package procfmt

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

// StateListen is the /proc/net/tcp "st" value for a listening socket.
const StateListen uint8 = 0x0A

// TCPEntry is one row of /proc/net/tcp or /proc/net/tcp6.
type TCPEntry struct {
	LocalAddr net.IP
	LocalPort uint16
	State     uint8
	UID       uint32
	Inode     uint64
}

// IsListening reports whether this entry is in the LISTEN state.
func (e TCPEntry) IsListening() bool { return e.State == StateListen }

// ParseTCPTable parses the contents of /proc/net/tcp or /proc/net/tcp6.
// Malformed data rows are skipped rather than failing the whole parse,
// since a transient bad row (e.g. the kernel rewriting the file mid-read)
// shouldn't cost the rest of a scan.
func ParseTCPTable(r io.Reader) ([]TCPEntry, error) {
	var entries []TCPEntry
	scanner := bufio.NewScanner(r)

	first := true
	for scanner.Scan() {
		if first {
			first = false
			continue // header line: "  sl  local_address rem_address ..."
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		entry, err := parseTCPLine(strings.Fields(line))
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// FilterListening returns only the entries in the LISTEN state.
func FilterListening(entries []TCPEntry) []TCPEntry {
	out := make([]TCPEntry, 0, len(entries))
	for _, e := range entries {
		if e.IsListening() {
			out = append(out, e)
		}
	}
	return out
}

func parseTCPLine(fields []string) (TCPEntry, error) {
	// sl local_address rem_address st tx_queue:rx_queue tr:tm->when retrnsmt uid timeout inode
	//  0       1              2      3          4               5        6    7      8     9
	if len(fields) < 10 {
		return TCPEntry{}, fmt.Errorf("procfmt: short tcp line: %d fields", len(fields))
	}

	addrPort := strings.SplitN(fields[1], ":", 2)
	if len(addrPort) != 2 {
		return TCPEntry{}, fmt.Errorf("procfmt: bad local_address %q", fields[1])
	}
	ip, err := parseHexIP(addrPort[0])
	if err != nil {
		return TCPEntry{}, err
	}
	port, err := strconv.ParseUint(addrPort[1], 16, 32)
	if err != nil {
		return TCPEntry{}, fmt.Errorf("procfmt: bad port %q: %w", addrPort[1], err)
	}

	state, err := strconv.ParseUint(fields[3], 16, 8)
	if err != nil {
		return TCPEntry{}, fmt.Errorf("procfmt: bad state %q: %w", fields[3], err)
	}
	uid, err := strconv.ParseUint(fields[7], 10, 32)
	if err != nil {
		return TCPEntry{}, fmt.Errorf("procfmt: bad uid %q: %w", fields[7], err)
	}
	inode, err := strconv.ParseUint(fields[9], 10, 64)
	if err != nil {
		return TCPEntry{}, fmt.Errorf("procfmt: bad inode %q: %w", fields[9], err)
	}

	return TCPEntry{
		LocalAddr: ip,
		LocalPort: uint16(port),
		State:     uint8(state),
		UID:       uint32(uid),
		Inode:     inode,
	}, nil
}

// parseHexIP decodes the hex-encoded, byte-order-swapped address format
// /proc/net/tcp[6] uses: 8 hex chars for IPv4, 32 for IPv6.
func parseHexIP(s string) (net.IP, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("procfmt: bad address hex %q: %w", s, err)
	}
	switch len(b) {
	case 4:
		return net.IPv4(b[3], b[2], b[1], b[0]), nil
	case 16:
		ip := make(net.IP, 16)
		for i := 0; i < 16; i += 4 {
			ip[i], ip[i+1], ip[i+2], ip[i+3] = b[i+3], b[i+2], b[i+1], b[i]
		}
		return ip, nil
	default:
		return nil, fmt.Errorf("procfmt: unexpected address length %d bytes", len(b))
	}
}
