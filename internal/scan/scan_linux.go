//go:build linux

package scan

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/subh05sus/porthole/internal/scan/procfmt"
)

type linuxLister struct{}

// NewDefaultLister returns the Linux scanner: parses /proc/net/tcp[6] for
// listening sockets, then walks /proc/[pid]/fd/* to map socket inodes back
// to PIDs, in one pass per PRD §7.2.
func NewDefaultLister() Lister { return linuxLister{} }

func (linuxLister) List(ctx context.Context) ([]Service, error) {
	inodeToPID, err := buildInodeToPIDMap()
	if err != nil {
		return nil, err
	}

	var services []Service
	tables := []struct {
		path string
		kind Proto
	}{
		{"/proc/net/tcp", ProtoTCP},
		{"/proc/net/tcp6", ProtoTCP6},
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

		for _, e := range procfmt.FilterListening(entries) {
			svc := Service{
				Port:  int(e.LocalPort),
				Proto: table.kind,
				Addr:  e.LocalAddr.String(),
			}
			if pid, ok := inodeToPID[e.Inode]; ok {
				svc.PID = pid
			} else {
				svc.ResolveErr = fmt.Errorf("scan: no process found for socket inode %d (permission denied or race)", e.Inode)
			}
			services = append(services, svc)
		}
	}
	return services, nil
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
