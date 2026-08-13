package output

import (
	"encoding/json"
	"io"

	"github.com/subh05sus/porthole/internal/scan"
)

// jsonService is the wire shape for `porthole list --json`. StartTime is
// deliberately excluded — it's an opaque internal reuse-detection token,
// not meaningful to a script consuming this output.
type jsonService struct {
	Port          int     `json:"port"`
	Proto         string  `json:"proto"`
	Addr          string  `json:"addr"`
	PID           int     `json:"pid"`
	Process       string  `json:"process"`
	Cmdline       string  `json:"cmdline"`
	User          string  `json:"user"`
	CWD           string  `json:"cwd"`
	Project       string  `json:"project"`
	UptimeSeconds float64 `json:"uptime_seconds"`
	Owned         bool    `json:"owned"`
	ResolveErr    string  `json:"resolve_error,omitempty"`
}

// JSON writes services to w as an indented JSON array. Always emits an
// array, even for zero services ("[]"), so consumers never need to special
// case a missing field.
func JSON(w io.Writer, services []scan.Service) error {
	out := make([]jsonService, len(services))
	for i, s := range services {
		js := jsonService{
			Port:          s.Port,
			Proto:         string(s.Proto),
			Addr:          s.Addr,
			PID:           s.PID,
			Process:       s.Process,
			Cmdline:       s.Cmdline,
			User:          s.User,
			CWD:           s.CWD,
			Project:       s.Project,
			UptimeSeconds: s.Uptime.Seconds(),
			Owned:         s.Owned,
		}
		if s.ResolveErr != nil {
			js.ResolveErr = s.ResolveErr.Error()
		}
		out[i] = js
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
