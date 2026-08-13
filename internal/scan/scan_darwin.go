//go:build darwin

package scan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/subh05sus/porthole/internal/scan/lsoffmt"
)

type darwinLister struct{}

// NewDefaultLister returns the macOS scanner: shells out to lsof in field
// output mode (machine-parseable, per PRD §7.2) rather than parsing the
// human-readable table.
func NewDefaultLister() Lister { return darwinLister{} }

func (darwinLister) List(ctx context.Context) ([]Service, error) {
	cmd := exec.CommandContext(ctx, "lsof", "-iTCP", "-sTCP:LISTEN", "-P", "-n", "-F", "pcnPu")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && stdout.Len() == 0 {
			// lsof exits 1 when it finds nothing matching the filter —
			// that's zero services, not a failure.
			return nil, nil
		}
		return nil, fmt.Errorf("scan: running lsof: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	sockets, err := lsoffmt.ParseLsofFields(&stdout)
	if err != nil {
		return nil, err
	}

	services := make([]Service, 0, len(sockets))
	for _, s := range sockets {
		services = append(services, Service{
			Port:    s.Port,
			Proto:   ProtoTCP,
			Addr:    s.Host,
			PID:     s.PID,
			Process: s.Command,
		})
	}
	return services, nil
}
