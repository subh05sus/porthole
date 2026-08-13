package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/subh05sus/porthole/internal/scan"
)

func TestJSONRoundTripsFields(t *testing.T) {
	services := []scan.Service{
		{
			Port: 3000, Proto: scan.ProtoTCP, Addr: "0.0.0.0", PID: 48211,
			Process: "node", Cmdline: "node server.js", User: "sub", CWD: "/home/sub/app",
			Project: "zapmail-web", Owned: true, Uptime: 90 * time.Second,
		},
	}

	var buf bytes.Buffer
	if err := JSON(&buf, services); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []jsonService
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	want := jsonService{
		Port: 3000, Proto: "tcp", Addr: "0.0.0.0", PID: 48211,
		Process: "node", Cmdline: "node server.js", User: "sub", CWD: "/home/sub/app",
		Project: "zapmail-web", UptimeSeconds: 90, Owned: true,
	}
	if got[0] != want {
		t.Fatalf("got %+v, want %+v", got[0], want)
	}
}

func TestJSONIncludesResolveErrAsString(t *testing.T) {
	services := []scan.Service{{Port: 631, ResolveErr: errors.New("permission denied")}}

	var buf bytes.Buffer
	if err := JSON(&buf, services); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []jsonService
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got[0].ResolveErr != "permission denied" {
		t.Fatalf("got ResolveErr %q, want %q", got[0].ResolveErr, "permission denied")
	}
}

func TestJSONEmptyServicesRendersEmptyArrayNotNull(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	trimmed := bytes.TrimSpace(buf.Bytes())
	if string(trimmed) != "[]" {
		t.Fatalf("got %q, want %q", trimmed, "[]")
	}
}
