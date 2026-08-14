package cli

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/subh05sus/porthole/internal/kill/killtest"
	"github.com/subh05sus/porthole/internal/scan"
	"github.com/subh05sus/porthole/internal/scan/scantest"
)

func TestServeBindsLoopbackOnlyAndServesDashboard(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: &scantest.FakeLister{Services: []scan.Service{{Port: 3000, Process: "node", Owned: true}}},
		Killer: &killtest.FakeKiller{},
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- serveUntilDone(ctx, app, 0, ready) }()

	var addr string
	select {
	case addr = <-ready:
	case <-time.After(2 * time.Second):
		t.Fatalf("server never reported ready")
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("expected a loopback-only bind, got %q", addr)
	}

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want 200", resp.StatusCode)
	}

	resp, err = http.Get("http://" + addr + "/api/services")
	if err != nil {
		t.Fatalf("GET /api/services failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want 200", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("serveUntilDone did not return after context cancellation")
	}

	if !strings.Contains(stdout.String(), "porthole dashboard: http://127.0.0.1:") {
		t.Fatalf("expected the listening-address banner, got %q", stdout.String())
	}
}

func TestServeReportsBindFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	app := &App{
		Lister: &scantest.FakeLister{},
		Killer: &killtest.FakeKiller{},
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// Occupy a port first so the real bind attempt below fails.
	holdCtx, holdCancel := context.WithCancel(context.Background())
	defer holdCancel()
	holdReady := make(chan string, 1)
	go func() { _ = serveUntilDone(holdCtx, app, 0, holdReady) }()
	addrStr := <-holdReady
	port := addrStr[strings.LastIndex(addrStr, ":")+1:]

	var portInt int
	for _, c := range port {
		portInt = portInt*10 + int(c-'0')
	}

	ready := make(chan string, 1)
	err := serveUntilDone(context.Background(), app, portInt, ready)
	if err == nil {
		t.Fatalf("expected an error binding an already-occupied port")
	}
}
