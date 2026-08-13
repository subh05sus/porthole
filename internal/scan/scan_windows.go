//go:build windows

package scan

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/subh05sus/porthole/internal/proc"
)

type windowsLister struct {
	lookup proc.Lookup
}

// NewDefaultLister returns the Windows scanner: GetExtendedTcpTable from
// iphlpapi.dll gives port->PID directly, no /proc-style inode walking
// needed — the cleanest of the three platforms per PRD §7.2. Process
// metadata beyond PID comes from internal/proc's Win32-based resolver.
func NewDefaultLister() Lister { return windowsLister{lookup: proc.NewDefaultLookup()} }

var (
	modIPHelper             = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTCPTable = modIPHelper.NewProc("GetExtendedTcpTable")
)

const (
	afINET  = 2  // AF_INET
	afINET6 = 23 // AF_INET6

	tcpTableOwnerPIDListener = 3 // TCP_TABLE_OWNER_PID_LISTENER
)

// mibTCPRowOwnerPID mirrors MIB_TCPROW_OWNER_PID.
type mibTCPRowOwnerPID struct {
	State      uint32
	LocalAddr  uint32
	LocalPort  uint32
	RemoteAddr uint32
	RemotePort uint32
	OwningPid  uint32
}

// mibTCP6RowOwnerPID mirrors MIB_TCP6ROW_OWNER_PID. Field order differs
// from the v4 row (address/port pairs first, state and PID last).
type mibTCP6RowOwnerPID struct {
	LocalAddr     [16]byte
	LocalScopeID  uint32
	LocalPort     uint32
	RemoteAddr    [16]byte
	RemoteScopeID uint32
	RemotePort    uint32
	State         uint32
	OwningPid     uint32
}

func (l windowsLister) List(ctx context.Context) ([]Service, error) {
	var services []Service

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	v4, err := listTCP4()
	if err != nil {
		return nil, err
	}
	services = append(services, v4...)

	select {
	case <-ctx.Done():
		return services, ctx.Err()
	default:
	}
	v6, err := listTCP6()
	if err != nil {
		return services, err
	}
	services = append(services, v6...)

	for i := range services {
		info, err := l.lookup.Lookup(services[i].PID)
		if err != nil {
			services[i].ResolveErr = err
			continue
		}
		services[i].Process = info.Process
		services[i].Cmdline = info.Cmdline
		services[i].User = info.User
		services[i].CWD = info.CWD
		services[i].StartTime = info.StartTime
		services[i].Uptime = info.Uptime
	}

	return services, nil
}

func listTCP4() ([]Service, error) {
	buf, numEntries, err := fetchTCPTable(afINET)
	if err != nil {
		return nil, err
	}

	rowSize := unsafe.Sizeof(mibTCPRowOwnerPID{})
	services := make([]Service, 0, numEntries)
	for i := uint32(0); i < numEntries; i++ {
		row := (*mibTCPRowOwnerPID)(unsafe.Pointer(&buf[4+uintptr(i)*rowSize]))
		services = append(services, Service{
			Port:  int(swapPort(row.LocalPort)),
			Proto: ProtoTCP,
			Addr:  ipv4FromUint32(row.LocalAddr).String(),
			PID:   int(row.OwningPid),
		})
	}
	return services, nil
}

func listTCP6() ([]Service, error) {
	buf, numEntries, err := fetchTCPTable(afINET6)
	if err != nil {
		return nil, err
	}

	rowSize := unsafe.Sizeof(mibTCP6RowOwnerPID{})
	services := make([]Service, 0, numEntries)
	for i := uint32(0); i < numEntries; i++ {
		row := (*mibTCP6RowOwnerPID)(unsafe.Pointer(&buf[4+uintptr(i)*rowSize]))
		services = append(services, Service{
			Port:  int(swapPort(row.LocalPort)),
			Proto: ProtoTCP6,
			Addr:  net.IP(row.LocalAddr[:]).String(),
			PID:   int(row.OwningPid),
		})
	}
	return services, nil
}

// fetchTCPTable calls GetExtendedTcpTable twice: once to learn the required
// buffer size, once to fill it. Returns the raw buffer (numEntries as its
// first 4 bytes, per MIB_TCPTABLE_OWNER_PID/MIB_TCP6TABLE_OWNER_PID) and the
// entry count.
func fetchTCPTable(af uintptr) ([]byte, uint32, error) {
	var size uint32
	r, _, _ := procGetExtendedTCPTable.Call(
		0, uintptr(unsafe.Pointer(&size)), 0, af, tcpTableOwnerPIDListener, 0,
	)
	if windows.Errno(r) != windows.ERROR_INSUFFICIENT_BUFFER {
		return nil, 0, fmt.Errorf("scan: GetExtendedTcpTable sizing call failed: %w", windows.Errno(r))
	}
	if size < 4 {
		size = 4
	}

	buf := make([]byte, size)
	r, _, _ = procGetExtendedTCPTable.Call(
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, af, tcpTableOwnerPIDListener, 0,
	)
	if r != 0 {
		return nil, 0, fmt.Errorf("scan: GetExtendedTcpTable failed: %w", windows.Errno(r))
	}

	numEntries := binary.LittleEndian.Uint32(buf[:4])
	return buf, numEntries, nil
}

// swapPort undoes the network-byte-order packing Windows stores the port
// number in within the low 16 bits of the row's DWORD port field.
func swapPort(p uint32) uint16 {
	b := uint16(p)
	return (b >> 8) | (b << 8)
}

func ipv4FromUint32(v uint32) net.IP {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return net.IPv4(b[0], b[1], b[2], b[3])
}
