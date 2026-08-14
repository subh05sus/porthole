package webui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/kill/killtest"
	"github.com/subh05sus/porthole/internal/scan"
	"github.com/subh05sus/porthole/internal/scan/scantest"
)

func TestIndexServesEmbeddedPage(t *testing.T) {
	h := NewHandler(&scantest.FakeLister{}, &killtest.FakeKiller{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>porthole</title>") {
		t.Fatalf("expected the dashboard page, got: %s", rec.Body.String())
	}
}

func TestIndexNotFoundForUnknownPath(t *testing.T) {
	h := NewHandler(&scantest.FakeLister{}, &killtest.FakeKiller{})
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", rec.Code)
	}
}

func TestServicesReturnsWireShapeWithStartTime(t *testing.T) {
	services := []scan.Service{
		{Port: 3000, PID: 111, StartTime: 42, Process: "node", Owned: true, Uptime: 90 * time.Second},
	}
	h := NewHandler(&scantest.FakeLister{Services: services}, &killtest.FakeKiller{})
	req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
	var got []wireService
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].StartTime != 42 || got[0].Port != 3000 {
		t.Fatalf("got %+v", got)
	}
}

func TestServicesPropagatesScanError(t *testing.T) {
	h := NewHandler(&scantest.FakeLister{Err: errBoom}, &killtest.FakeKiller{})
	req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want 500", rec.Code)
	}
}

func TestKillRejectsGET(t *testing.T) {
	h := NewHandler(&scantest.FakeLister{}, &killtest.FakeKiller{})
	req := httptest.NewRequest(http.MethodGet, "/api/kill", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got status %d, want 405", rec.Code)
	}
}

func TestKillSucceedsWithMatchingOrigin(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	h := NewHandler(&scantest.FakeLister{}, killer)

	body, _ := json.Marshal(killRequest{PID: 111, StartTime: 42, Owned: true})
	req := httptest.NewRequest(http.MethodPost, "/api/kill", bytes.NewReader(body))
	req.Header.Set("Origin", "http://127.0.0.1:9191")
	req.Host = "127.0.0.1:9191"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var resp killResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Status != "killed" {
		t.Fatalf("got status %q, want killed", resp.Status)
	}
	if len(killer.ExecuteCalls) != 1 || killer.ExecuteCalls[0].Target.PID != 111 || killer.ExecuteCalls[0].Target.StartTime != 42 {
		t.Fatalf("expected Execute called with the decoded target, got %+v", killer.ExecuteCalls)
	}
}

func TestKillRejectsCrossOrigin(t *testing.T) {
	killer := &killtest.FakeKiller{}
	h := NewHandler(&scantest.FakeLister{}, killer)

	body, _ := json.Marshal(killRequest{PID: 111})
	req := httptest.NewRequest(http.MethodPost, "/api/kill", bytes.NewReader(body))
	req.Header.Set("Origin", "http://evil.example.com")
	req.Host = "127.0.0.1:9191"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got status %d, want 403", rec.Code)
	}
	if len(killer.ExecuteCalls) != 0 {
		t.Fatalf("must never call Execute for a cross-origin request, got %d calls", len(killer.ExecuteCalls))
	}
}

func TestKillAllowsRequestsWithNoOriginHeader(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteResult: kill.Result{Status: kill.StatusKilled}}
	h := NewHandler(&scantest.FakeLister{}, killer)

	body, _ := json.Marshal(killRequest{PID: 111})
	req := httptest.NewRequest(http.MethodPost, "/api/kill", bytes.NewReader(body))
	// No Origin header — e.g. curl or a non-browser script.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200", rec.Code)
	}
}

func TestKillReturnsErrorStatusOnKillerFailure(t *testing.T) {
	killer := &killtest.FakeKiller{ExecuteErr: errBoom}
	h := NewHandler(&scantest.FakeLister{}, killer)

	body, _ := json.Marshal(killRequest{PID: 111})
	req := httptest.NewRequest(http.MethodPost, "/api/kill", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		// Kill errors report as a JSON body, not an HTTP error status —
		// mirrors how the CLI/TUI surface kill failures as messages, not
		// process crashes.
		t.Fatalf("got status %d, want 200 with an error body", rec.Code)
	}
	var resp killResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Status != "error" || resp.Error == "" {
		t.Fatalf("got %+v, want an error status with a message", resp)
	}
}

var errBoom = &boomError{}

type boomError struct{}

func (*boomError) Error() string { return "boom" }
