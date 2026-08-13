//go:build darwin

package scan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/subh05sus/porthole/internal/proc"
	"github.com/subh05sus/porthole/internal/project"
	"github.com/subh05sus/porthole/internal/scan/lsoffmt"
)

type darwinLister struct {
	lookup   proc.Lookup
	detector *project.Detector
}

// NewDefaultLister returns the macOS scanner: shells out to lsof in field
// output mode (machine-parseable, per PRD §7.2) rather than parsing the
// human-readable table. Process metadata beyond name/PID comes from
// internal/proc's ps/lsof-based resolver.
func NewDefaultLister() Lister {
	return darwinLister{lookup: proc.NewDefaultLookup(), detector: project.NewDetector()}
}

func (l darwinLister) List(ctx context.Context) ([]Service, error) {
	tcpSockets, err := runLsof(ctx, "-iTCP", "-sTCP:LISTEN", "-P", "-n", "-F", "pcnPu")
	if err != nil {
		return nil, err
	}
	// UDP has no LISTEN state, so the -sTCP:LISTEN filter doesn't apply —
	// every socket lsof reports for -iUDP is already locally bound.
	udpSockets, err := runLsof(ctx, "-iUDP", "-P", "-n", "-F", "pcnPu")
	if err != nil {
		return nil, err
	}

	services := make([]Service, 0, len(tcpSockets)+len(udpSockets))
	services = appendLsofServices(services, tcpSockets, ProtoTCP)
	services = appendLsofServices(services, udpSockets, ProtoUDP)

	for i := range services {
		owned, permErr := checkOwned(services[i].PID)
		services[i].Owned = owned

		if info, err := l.lookup.Lookup(services[i].PID); err == nil {
			services[i].Cmdline = info.Cmdline
			services[i].User = info.User
			services[i].CWD = info.CWD
			services[i].StartTime = info.StartTime
			services[i].Uptime = info.Uptime
		} else {
			services[i].ResolveErr = err
		}
		if !owned && services[i].ResolveErr == nil {
			services[i].ResolveErr = permErr
		}
	}
	return Enrich(services, l.detector), nil
}

func runLsof(ctx context.Context, args ...string) ([]lsoffmt.Socket, error) {
	cmd := exec.CommandContext(ctx, "lsof", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && stdout.Len() == 0 {
			// lsof exits 1 when it finds nothing matching the filter —
			// that's zero services, not a failure.
			return nil, nil
		}
		return nil, fmt.Errorf("scan: running lsof: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	return lsoffmt.ParseLsofFields(&stdout)
}

func appendLsofServices(services []Service, sockets []lsoffmt.Socket, proto Proto) []Service {
	for _, s := range sockets {
		services = append(services, Service{
			Port:    s.Port,
			Proto:   proto,
			Addr:    s.Host,
			PID:     s.PID,
			Process: s.Command,
		})
	}
	return services
}
