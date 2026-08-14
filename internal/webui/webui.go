// Package webui implements `porthole serve`'s optional local web
// dashboard: a small stdlib net/http server exposing the scan/kill loop
// as a JSON API plus one embedded HTML/JS page. Binding is the caller's
// responsibility (see cli/serve.go) — this package only builds the
// handler, it never opens a listener itself.
package webui

import (
	"embed"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/scan"
)

//go:embed index.html
var indexFS embed.FS

// Handler serves the dashboard page and its JSON API. Construct via
// NewHandler; it implements http.Handler directly so callers can wrap it
// in their own middleware/mux if ever needed.
type Handler struct {
	lister scan.Lister
	killer kill.Killer
	mux    *http.ServeMux
}

func NewHandler(lister scan.Lister, killer kill.Killer) *Handler {
	h := &Handler{lister: lister, killer: killer}
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/api/services", h.handleServices)
	mux.HandleFunc("/api/kill", h.handleKill)
	h.mux = mux
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := indexFS.ReadFile("index.html")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// wireService is the dashboard API's own JSON shape — distinct from
// output.JSON's `porthole list --json` shape, which deliberately omits
// StartTime as a meaningless opaque token for a read-only script consumer.
// The dashboard needs StartTime back on the kill request (see handleKill)
// so the ladder can still re-verify PID reuse before signaling, the same
// safety property every other kill entry point has.
type wireService struct {
	Port           int     `json:"port"`
	Proto          string  `json:"proto"`
	Addr           string  `json:"addr"`
	PID            int     `json:"pid"`
	StartTime      uint64  `json:"start_time"`
	Process        string  `json:"process"`
	Cmdline        string  `json:"cmdline"`
	User           string  `json:"user"`
	CWD            string  `json:"cwd"`
	Project        string  `json:"project"`
	Container      string  `json:"container,omitempty"`
	ContainerImage string  `json:"container_image,omitempty"`
	ContainerID    string  `json:"container_id,omitempty"`
	UptimeSeconds  float64 `json:"uptime_seconds"`
	Owned          bool    `json:"owned"`
	ResolveErr     string  `json:"resolve_error,omitempty"`
}

func (h *Handler) handleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	services, err := h.lister.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out := make([]wireService, len(services))
	for i, s := range services {
		ws := wireService{
			Port: s.Port, Proto: string(s.Proto), Addr: s.Addr,
			PID: s.PID, StartTime: s.StartTime,
			Process: s.Process, Cmdline: s.Cmdline, User: s.User, CWD: s.CWD,
			Project: s.Project, Container: s.Container, ContainerImage: s.ContainerImage, ContainerID: s.ContainerID,
			UptimeSeconds: s.Uptime.Seconds(), Owned: s.Owned,
		}
		if s.ResolveErr != nil {
			ws.ResolveErr = s.ResolveErr.Error()
		}
		out[i] = ws
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

type killRequest struct {
	PID         int    `json:"pid"`
	StartTime   uint64 `json:"start_time"`
	Owned       bool   `json:"owned"`
	ContainerID string `json:"container_id,omitempty"`
}

type killResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (h *Handler) handleKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !sameOrigin(r) {
		// See sameOrigin's comment: this is a browser-CSRF mitigation, not
		// an auth boundary — anything hitting this port directly already
		// has the same access running the CLI locally would.
		http.Error(w, "cross-origin requests are not allowed", http.StatusForbidden)
		return
	}

	var req killRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	target := kill.Target{PID: req.PID, StartTime: req.StartTime, Owned: req.Owned, ContainerID: req.ContainerID}
	res, err := h.killer.Execute(r.Context(), target, kill.Options{AutoEscalate: true})

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		json.NewEncoder(w).Encode(killResponse{Status: "error", Error: err.Error()})
		return
	}
	json.NewEncoder(w).Encode(killResponse{Status: statusString(res.Status)})
}

func statusString(s kill.Status) string {
	switch s {
	case kill.StatusKilled:
		return "killed"
	case kill.StatusAlreadyDead:
		return "already_dead"
	case kill.StatusNeedsEscalation:
		return "needs_escalation"
	default:
		return "unknown"
	}
}

// sameOrigin implements a minimal CSRF mitigation appropriate for a
// localhost-only server with a kill switch: browser CORS only blocks a
// page from *reading* a cross-origin response, not from *sending* a
// state-changing fetch/form POST in the first place — so without this
// check, any website a porthole-serve user happens to visit could kill
// processes on their machine just by shipping a POST to
// http://127.0.0.1:<port>/api/kill in its JS. Requests with no Origin
// header (curl, scripts, server-to-server) are let through — this guards
// against a *browser* silently carrying a malicious page's request, not
// against something with direct access to the port, which already has
// the same access running the CLI locally would.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return u.Host == r.Host
}
