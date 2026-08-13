//go:build linux

package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/subh05sus/porthole/internal/proc"
	"github.com/subh05sus/porthole/internal/project"
	"github.com/subh05sus/porthole/internal/scan/procfmt"
)

type linuxLister struct {
	lookup   proc.Lookup
	detector *project.Detector
}

// NewDefaultLister returns the Linux scanner: parses /proc/net/tcp[6] for
// listening sockets, then walks /proc/[pid]/fd/* to map socket inodes back
// to PIDs, in one pass per PRD §7.2. Process metadata (name, cmdline, cwd,
// user, start time) for each resolved PID comes from internal/proc.
func NewDefaultLister() Lister {
	return linuxLister{lookup: proc.NewDefaultLookup(), detector: project.NewDetector()}
}

func (l linuxLister) List(ctx context.Context) ([]Service, error) {
	inodeToPID, err := buildInodeToPIDMap()
	if err != nil {
		return nil, err
	}

	var services []Service
	tables := []struct {
		path       string
		kind       Proto
		listenOnly bool
	}{
		{"/proc/net/tcp", ProtoTCP, true},
		{"/proc/net/tcp6", ProtoTCP6, true},
		{"/proc/net/udp", ProtoUDP, false},
		{"/proc/net/udp6", ProtoUDP6, false},
	}
	for _, table := range tables {
		select {
		case <-ctx.Done():
			return services, ctx.Err()
		default:
		}

		entries, err := readTCPTable(table.path)
		if err != nil {
			if os.IsNotExist(err) {
				continue // e.g. tcp6 absent when IPv6 is disabled
			}
			return services, err
		}

		if table.listenOnly {
			entries = procfmt.FilterListening(entries)
		}
		// UDP has no LISTEN state; every bound entry in /proc/net/udp[6] is
		// a locally-bound socket worth reporting, per PRD's "relevant"
		// redefinition for connectionless protocols.

		for _, e := range entries {
			svc := Service{
				Port:  int(e.LocalPort),
				Proto: table.kind,
				Addr:  e.LocalAddr.String(),
			}
			if pid, ok := inodeToPID[e.Inode]; ok {
				svc.PID = pid

				owned, permErr := checkOwned(pid)
				svc.Owned = owned

				if info, err := l.lookup.Lookup(pid); err == nil {
					svc.Process = info.Process
					svc.Cmdline = info.Cmdline
					svc.User = info.User
					svc.CWD = info.CWD
					svc.StartTime = info.StartTime
					svc.Uptime = info.Uptime
				} else if svc.ResolveErr == nil {
					svc.ResolveErr = err
				}
				if !owned && svc.ResolveErr == nil {
					svc.ResolveErr = permErr
				}
			} else {
				svc.ResolveErr = fmt.Errorf("scan: no process found for socket inode %d (permission denied or race)", e.Inode)
			}
			services = append(services, svc)
		}
	}
	return Enrich(services, l.detector), nil
}

// SocketsForPID implements scan.SocketQuerier: every socket a process
// holds, not just LISTEN-state/bound ones, resolved by re-parsing all four
// /proc/net tables and keeping only rows whose inode belongs to this PID's
// open file descriptors (rather than the whole-system walk buildInodeToPIDMap
// does for a regular scan).
func (l linuxLister) SocketsForPID(ctx context.Context, pid int) ([]Service, error) {
	inodes, err := socketInodesForPID(pid)
	if err != nil {
		return nil, err
	}

	var out []Service
	tables := []struct {
		path string
		kind Proto
	}{
		{"/proc/net/tcp", ProtoTCP},
		{"/proc/net/tcp6", ProtoTCP6},
		{"/proc/net/udp", ProtoUDP},
		{"/proc/net/udp6", ProtoUDP6},
	}
	for _, table := range tables {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		default:
		}

		entries, err := readTCPTable(table.path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return out, err
		}
		for _, e := range entries {
			if !inodes[e.Inode] {
				continue
			}
			out = append(out, Service{
				Port:  int(e.LocalPort),
				Proto: table.kind,
				Addr:  e.LocalAddr.String(),
				PID:   pid,
			})
		}
	}
	return out, nil
}

func socketInodesForPID(pid int) (map[uint64]bool, error) {
	fds, err := os.ReadDir(filepath.Join("/proc", strconv.Itoa(pid), "fd"))
	if err != nil {
		return nil, fmt.Errorf("scan: reading fds for pid %d: %w", pid, err)
	}
	m := make(map[uint64]bool)
	for _, fd := range fds {
		link, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "fd", fd.Name()))
		if err != nil {
			continue
		}
		if inode, ok := parseSocketInode(link); ok {
			m[inode] = true
		}
	}
	return m, nil
}

func readTCPTable(path string) ([]procfmt.TCPEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return procfmt.ParseTCPTable(f)
}

// buildInodeToPIDMap walks /proc/[pid]/fd/* once, mapping each open
// "socket:[N]" descriptor back to the PID that holds it. A PID whose fd
// directory can't be read (another user's process) is silently skipped —
// its sockets simply won't resolve, and the caller records why per PRD §8.2.
func buildInodeToPIDMap() (map[uint64]int, error) {
	procEntries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("scan: reading /proc: %w", err)
	}

	m := make(map[uint64]int)
	for _, entry := range procEntries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // not a PID directory
		}

		fds, err := os.ReadDir(filepath.Join("/proc", entry.Name(), "fd"))
		if err != nil {
			continue // permission denied, or process exited mid-scan
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join("/proc", entry.Name(), "fd", fd.Name()))
			if err != nil {
				continue
			}
			if inode, ok := parseSocketInode(link); ok {
				m[inode] = pid
			}
		}
	}
	return m, nil
}

func parseSocketInode(link string) (uint64, bool) {
	if !strings.HasPrefix(link, "socket:[") || !strings.HasSuffix(link, "]") {
		return 0, false
	}
	n, err := strconv.ParseUint(link[len("socket:["):len(link)-1], 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
