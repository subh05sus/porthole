// Package tui implements porthole's interactive Bubble Tea interface.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/subh05sus/porthole/internal/config"
	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/proc"
	"github.com/subh05sus/porthole/internal/restart"
	"github.com/subh05sus/porthole/internal/scan"
	"github.com/subh05sus/porthole/internal/tui/anim"
	"github.com/subh05sus/porthole/internal/tui/theme"
)

// scanTimeout bounds every scan per PRD §8.3 — a slow scan renders partial
// results plus a warning instead of freezing the UI.
const scanTimeout = 3 * time.Second

// tickInterval drives every animation in the TUI from one recurring
// tea.Tick, per PRD §5.3's "everything drives off one tea.Tick at 60ms. No
// per-row goroutines. No timers stacking."
const tickInterval = 60 * time.Millisecond

// killDissolveDuration is how long a just-killed row stays visible,
// dissolving, before the list reflows (PRD §5.2: "~400ms").
const killDissolveDuration = 400 * time.Millisecond

type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeConfirmKill
	modeConfirmEscalate
	modeKilling
	modeHelp
	modeDetail
	modeConfirmRestart
	modeRestarting
	modeConfirmProtected
)

// watchInterval is how often watch mode rescans, per PRD §6's "watch" verb.
const watchInterval = 2 * time.Second

// rowKey identifies a row for highlight/fade tracking — a lighter-weight
// local twin of scan's own unexported identity type.
type rowKey struct {
	port, pid int
}

func keyOf(s scan.Service) rowKey { return rowKey{port: s.Port, pid: s.PID} }

// fadeEntry is a row still visible while dissolving — either a row the
// user just killed, or one watch mode reported as removed. triggersRescan
// is true only for kill-flow entries: watch mode already drives its own
// scan cadence, so a watch-removal's fade completing must not also queue
// an extra scan.
type fadeEntry struct {
	service        scan.Service
	startedAt      time.Time
	triggersRescan bool
}

// Model is porthole's Bubble Tea Model. Init/Update/View never call the
// scanner or killer directly on this goroutine — every OS interaction is
// dispatched as a tea.Cmd so the UI thread never blocks (PRD §8.3).
type Model struct {
	lister  scan.Lister
	killer  kill.Killer
	lookup  proc.Lookup
	spawner restart.Spawner
	cfg     config.Config
	th      theme.Theme

	// clock is injectable so animation timing is deterministic in tests
	// (see model_test.go) instead of racing real wall-clock time.
	clock func() time.Time

	services []scan.Service
	filtered []scan.Service
	cursor   int

	filterInput  textinput.Model
	confirmInput textinput.Model // typed-port confirmation for protected ports

	mode          mode
	pending       *scan.Service   // single row awaiting the interactive escalate-after-ignored-SIGTERM prompt
	pendingBulk   []scan.Service  // row(s) awaiting the initial kill/force-kill y/n confirmation (1+ rows)
	multiSelected map[rowKey]bool // multi-selected rows (space), independent of cursor position
	force         bool            // whether the pending kill is the force (K) path
	killStart     time.Time       // when the in-flight kill/escalate began, for the spinner
	killCount     int             // how many targets the in-flight kill covers, for the status line

	fadingOut     []fadeEntry          // rows dissolving (kill success and/or watch removals)
	recentlyAdded map[rowKey]time.Time // rows briefly highlighted after appearing in watch mode

	// detailTarget is a snapshot copy (not a pointer into m.filtered) of
	// the row the detail pane (enter) is showing, so it stays stable even
	// if the underlying list refreshes while the pane is open.
	detailTarget *scan.Service

	// querier is the optional scan.SocketQuerier capability (nil if the
	// injected Lister doesn't implement it), used for the detail pane's
	// on-demand full socket list. detailSockets/detailSocketsLoading track
	// that in-flight/completed query; detailSockets falls back to
	// relatedSockets (listening-only, from the last regular scan) when
	// querier is nil or the query fails.
	querier              scan.SocketQuerier
	detailSockets        []scan.Service
	detailSocketsLoading bool

	pendingRestart *scan.Service // row awaiting the R confirmation prompt
	restartStart   time.Time     // when the in-flight restart began, for the spinner

	watching     bool
	watchCancel  context.CancelFunc
	watchEvents  <-chan scan.Event
	watchPulseOn bool

	status       string
	statusExpiry time.Time // while non-zero and in the future, status overrides computeNormalStatus
	scanErr      error
	scanning     bool
	scanStart    time.Time
	revealStart  time.Time

	// firstScanDone/sudoBanner implement PRD §8.2's startup check: if more
	// than half of the sockets found on the very first scan are
	// unresolvable, show a one-line banner suggesting elevated
	// permissions. Computed once, not re-evaluated on every refresh.
	firstScanDone bool
	sudoBanner    string

	width, height int
	quitting      bool
}

// New builds a Model. Dependencies are injected so this whole package can
// be tested without a real OS (see model_test.go).
func New(lister scan.Lister, killer kill.Killer, lookup proc.Lookup, spawner restart.Spawner, cfg config.Config, th theme.Theme) Model {
	ti := textinput.New()
	ti.Placeholder = "filter by port, process, or project"
	ti.Prompt = "/ "

	clock := time.Now
	querier, _ := lister.(scan.SocketQuerier)
	return Model{
		lister:      lister,
		killer:      killer,
		lookup:      lookup,
		spawner:     spawner,
		cfg:         cfg,
		th:          th,
		clock:       clock,
		filterInput: ti,
		status:      "scanning sockets",
		scanning:    true,
		scanStart:   clock(),
		querier:     querier,
	}
}

// Init cannot mutate the Model — bubbletea's Init() returns only a Cmd, and
// any receiver mutation here would be discarded — so the initial
// scanning/scanStart state is set directly in New() instead.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.scanCmd(), m.tickCmd())
}

type scanCompleteMsg struct {
	services []scan.Service
	err      error
}

func (m Model) scanCmd() tea.Cmd {
	lister := m.lister
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
		defer cancel()
		services, err := lister.List(ctx)
		return scanCompleteMsg{services: services, err: err}
	}
}

type killResultMsg struct {
	target scan.Service
	force  bool
	result kill.Result
	err    error
}

func (m Model) killCmd(target scan.Service, force bool) tea.Cmd {
	killer := m.killer
	t := kill.Target{PID: target.PID, StartTime: target.StartTime, Owned: target.Owned, ContainerID: target.ContainerID}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := killer.Execute(ctx, t, kill.Options{Force: force})
		return killResultMsg{target: target, force: force, result: res, err: err}
	}
}

func (m Model) escalateCmd(target scan.Service) tea.Cmd {
	killer := m.killer
	t := kill.Target{PID: target.PID, StartTime: target.StartTime, Owned: target.Owned, ContainerID: target.ContainerID}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := killer.Escalate(ctx, t)
		return killResultMsg{target: target, force: true, result: res, err: err}
	}
}

type restartResultMsg struct {
	target scan.Service
	err    error
}

// restartCmd captures a respawn plan, kills the target, and respawns it —
// all inside one tea.Cmd closure (off the UI thread), same discipline as
// every other OS interaction in this model. Mirrors internal/cli/restart.go's
// runRestart: capture before kill (nothing left to capture from a dead
// process), only spawn after confirming the kill actually succeeded.
func (m Model) restartCmd(target scan.Service) tea.Cmd {
	lookup := m.lookup
	killer := m.killer
	spawner := m.spawner
	return func() tea.Msg {
		plan, err := restart.Capture(lookup, target)
		if err != nil {
			return restartResultMsg{target: target, err: err}
		}

		t := kill.Target{PID: target.PID, StartTime: target.StartTime, Owned: target.Owned, ContainerID: target.ContainerID}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := killer.Execute(ctx, t, kill.Options{AutoEscalate: true})
		if err != nil {
			return restartResultMsg{target: target, err: err}
		}
		if res.Status != kill.StatusKilled && res.Status != kill.StatusAlreadyDead {
			return restartResultMsg{target: target, err: fmt.Errorf("process ignored kill signal, not restarting")}
		}

		if err := spawner.Spawn(plan); err != nil {
			return restartResultMsg{target: target, err: fmt.Errorf("killed but failed to respawn: %w", err)}
		}
		return restartResultMsg{target: target}
	}
}

func (m Model) handleRestartResult(msg restartResultMsg) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	m.pendingRestart = nil

	if msg.err != nil {
		m.setEphemeralStatus(fmt.Sprintf("restart failed for %s on :%d: %v", msg.target.Process, msg.target.Port, msg.err), ephemeralStatusDuration)
		return m, nil
	}

	m.fadingOut = append(m.fadingOut, fadeEntry{service: msg.target, startedAt: m.clock(), triggersRescan: true})
	m.setEphemeralStatus(fmt.Sprintf("restarted %s on :%d", msg.target.Process, msg.target.Port), killDissolveDuration)
	return m, nil
}

type bulkKillResult struct {
	target scan.Service
	result kill.Result
	err    error
}

type bulkKillMsg struct {
	results []bulkKillResult
}

// bulkKillCmd kills multiple targets sequentially, auto-escalating past an
// ignored SIGTERM for each (kill.Options.AutoEscalate) rather than prompting
// interactively per-target — with N selected rows, a per-target y/n
// escalation prompt doesn't scale, so bulk kill behaves like the CLI's
// multi-port kill: keep going, report the aggregate outcome at the end.
func (m Model) bulkKillCmd(targets []scan.Service, force bool) tea.Cmd {
	killer := m.killer
	return func() tea.Msg {
		results := make([]bulkKillResult, 0, len(targets))
		for _, t := range targets {
			target := kill.Target{PID: t.PID, StartTime: t.StartTime, Owned: t.Owned, ContainerID: t.ContainerID}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			res, err := killer.Execute(ctx, target, kill.Options{Force: force, AutoEscalate: true})
			cancel()
			results = append(results, bulkKillResult{target: t, result: res, err: err})
		}
		return bulkKillMsg{results: results}
	}
}

type detailSocketsMsg struct {
	pid     int
	sockets []scan.Service
	err     error
}

// detailSocketsCmd fetches the full socket list for pid via the injected
// SocketQuerier (off the UI thread, same discipline as every other OS
// interaction here). Only called when m.querier is non-nil — callers fall
// back to relatedSockets otherwise.
func (m Model) detailSocketsCmd(pid int) tea.Cmd {
	querier := m.querier
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), scanTimeout)
		defer cancel()
		sockets, err := querier.SocketsForPID(ctx, pid)
		return detailSocketsMsg{pid: pid, sockets: sockets, err: err}
	}
}

func (m Model) handleDetailSockets(msg detailSocketsMsg) (tea.Model, tea.Cmd) {
	m.detailSocketsLoading = false
	if m.detailTarget == nil || msg.pid != m.detailTarget.PID {
		// Pane was closed or reopened on a different row before this
		// landed — drop it rather than showing stale sockets.
		return m, nil
	}
	if msg.err != nil {
		// Fall back to what the last regular scan already knows rather
		// than showing nothing.
		m.detailSockets = m.relatedSockets(*m.detailTarget)
		return m, nil
	}
	m.detailSockets = msg.sockets
	return m, nil
}

type tickMsg time.Time

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case scanCompleteMsg:
		m.scanning = false
		m.scanErr = msg.err
		m.services = msg.services
		m.revealStart = m.clock()
		m.applyFilter()
		if !m.firstScanDone {
			m.firstScanDone = true
			m.sudoBanner = sudoBannerFor(msg.services)
		}
		return m, nil

	case killResultMsg:
		return m.handleKillResult(msg)

	case bulkKillMsg:
		return m.handleBulkKillResult(msg)

	case restartResultMsg:
		return m.handleRestartResult(msg)

	case detailSocketsMsg:
		return m.handleDetailSockets(msg)

	case watchEventMsg:
		return m.handleWatchEvent(msg)

	case tickMsg:
		return m.handleTick()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// watchEventMsg wraps a scan.Event so the "listen on channel, requeue Cmd"
// pattern below can distinguish it from other tea.Msg types.
type watchEventMsg struct {
	event scan.Event
	ok    bool // false means the channel closed
}

// waitForWatchEvent reads exactly one event from m.watchEvents. Called
// again after every event is processed (see handleWatchEvent), mirroring
// the tick loop's self-rescheduling — never more than one of these is ever
// in flight, so this doesn't violate the "no timers/goroutines stacking"
// budget even though it's conceptually a separate stream from the 60ms tick.
func (m Model) waitForWatchEvent() tea.Cmd {
	ch := m.watchEvents
	return func() tea.Msg {
		ev, ok := <-ch
		return watchEventMsg{event: ev, ok: ok}
	}
}

func (m Model) handleWatchEvent(msg watchEventMsg) (tea.Model, tea.Cmd) {
	if !msg.ok {
		// The channel closed — e.g. someone else cancelled our context.
		// Fall back to normal (non-watching) state rather than spin on a
		// closed channel.
		m.watching = false
		return m, nil
	}

	m.watchPulseOn = !m.watchPulseOn

	if msg.event.Err == nil {
		m.services = msg.event.Services
		m.revealStart = m.clock()
		m.applyFilter()

		now := m.clock()
		if m.recentlyAdded == nil {
			m.recentlyAdded = make(map[rowKey]time.Time)
		}
		for _, s := range msg.event.Diff.Added {
			m.recentlyAdded[keyOf(s)] = now
		}
		for _, s := range msg.event.Diff.Removed {
			m.fadingOut = append(m.fadingOut, fadeEntry{service: s, startedAt: now, triggersRescan: false})
		}
	}

	return m, m.waitForWatchEvent()
}

func (m Model) handleTick() (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if len(m.fadingOut) > 0 {
		remaining := m.fadingOut[:0:0]
		needsRescan := false
		for _, f := range m.fadingOut {
			if anim.FadeOutComplete(m.clock().Sub(f.startedAt), killDissolveDuration) {
				if f.triggersRescan {
					needsRescan = true
				}
				continue
			}
			remaining = append(remaining, f)
		}
		m.fadingOut = remaining
		if needsRescan {
			cmd = m.scanCmd()
		}
	}

	for key, at := range m.recentlyAdded {
		if m.clock().Sub(at) >= killDissolveDuration {
			delete(m.recentlyAdded, key)
		}
	}

	switch {
	case m.mode == modeNormal && (m.statusExpiry.IsZero() || !m.clock().Before(m.statusExpiry)):
		m.status = m.computeNormalStatus()
	case m.mode == modeKilling:
		verb := "sending kill signal"
		if m.force {
			verb = "sending force kill"
		}
		if m.killCount > 1 {
			verb = fmt.Sprintf("%s to %d services", verb, m.killCount)
		}
		m.status = fmt.Sprintf("%s… %c", verb, anim.SpinnerFrame(m.clock().Sub(m.killStart)))
	case m.mode == modeRestarting:
		m.status = fmt.Sprintf("restarting… %c", anim.SpinnerFrame(m.clock().Sub(m.restartStart)))
	}

	return m, tea.Batch(cmd, m.tickCmd())
}

// ephemeralStatusDuration is how long a one-off message (kill
// cancelled/succeeded/failed) stays on screen in modeNormal before
// computeNormalStatus resumes driving the header text.
const ephemeralStatusDuration = 2 * time.Second

func (m *Model) setEphemeralStatus(text string, dur time.Duration) {
	m.status = text
	m.statusExpiry = m.clock().Add(dur)
}

// computeNormalStatus implements PRD §5.3's "talking status line": the
// header text progresses "scanning sockets" -> "resolving N processes" ->
// "N services listening" as time passes since the last scan, rather than
// tracking literal per-service resolution (Lister.List returns everything
// in one call, so this is a scripted animation beat keyed off elapsed time
// and the final count, not real streaming progress).
func (m Model) computeNormalStatus() string {
	if m.scanErr != nil {
		return fmt.Sprintf("scan timed out or failed: %v", m.scanErr)
	}
	if m.scanning {
		return fmt.Sprintf("scanning sockets %c", anim.SpinnerFrame(m.clock().Sub(m.scanStart)))
	}
	elapsed := m.clock().Sub(m.revealStart)
	if elapsed < anim.RevealCap {
		return fmt.Sprintf("resolving %d processes %c", len(m.services), anim.SpinnerFrame(elapsed))
	}
	return fmt.Sprintf("%d services listening", len(m.services))
}

func (m *Model) applyFilter() {
	query := strings.TrimSpace(m.filterInput.Value())
	if query == "" {
		m.filtered = m.services
	} else {
		m.filtered = filterServices(m.services, query)
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// filterServices does a case-insensitive substring match across port,
// process, and project — the v1 reading of PRD §5.4's "fuzzy filter" (see
// TODO.md for the deferred true-fuzzy-matching upgrade).
func filterServices(services []scan.Service, query string) []scan.Service {
	q := strings.ToLower(query)
	out := make([]scan.Service, 0, len(services))
	for _, s := range services {
		port := fmt.Sprintf("%d", s.Port)
		if strings.Contains(port, q) ||
			strings.Contains(strings.ToLower(s.Process), q) ||
			strings.Contains(strings.ToLower(s.Project), q) {
			out = append(out, s)
		}
	}
	return out
}

// sudoBannerFor implements PRD §8.2: "if more than half of discovered
// sockets are unresolvable, show a one-line banner suggesting sudo."
// Returns "" when the check doesn't trigger (including the len==0 case, so
// an empty scan never falsely divides by zero into "100% unresolvable").
func sudoBannerFor(services []scan.Service) string {
	if len(services) == 0 {
		return ""
	}
	unresolved := 0
	for _, s := range services {
		if s.ResolveErr != nil {
			unresolved++
		}
	}
	if unresolved*2 <= len(services) {
		return ""
	}
	return fmt.Sprintf("%d of %d services need elevated permissions to inspect — try running as Administrator/root", unresolved, len(services))
}

func (m Model) handleKillResult(msg killResultMsg) (tea.Model, tea.Cmd) {
	m.pending = nil

	switch {
	case msg.err != nil:
		m.mode = modeNormal
		m.setEphemeralStatus(fmt.Sprintf("port :%d: %v", msg.target.Port, msg.err), ephemeralStatusDuration)
		return m, nil

	case msg.result.Status == kill.StatusKilled || msg.result.Status == kill.StatusAlreadyDead:
		m.mode = modeNormal
		m.fadingOut = append(m.fadingOut, fadeEntry{service: msg.target, startedAt: m.clock(), triggersRescan: true})
		m.setEphemeralStatus(fmt.Sprintf("terminated %s on :%d", msg.target.Process, msg.target.Port), killDissolveDuration)
		return m, nil

	case msg.result.Status == kill.StatusNeedsEscalation && !msg.force:
		m.mode = modeConfirmEscalate
		m.pending = &msg.target
		m.status = fmt.Sprintf("process ignored SIGTERM, force kill %s on :%d? (y/n)", msg.target.Process, msg.target.Port)
		return m, nil

	default:
		m.mode = modeNormal
		m.setEphemeralStatus(fmt.Sprintf("port :%d: process ignored kill signal", msg.target.Port), ephemeralStatusDuration)
		return m, nil
	}
}

// handleBulkKillResult processes the aggregate outcome of a multi-select
// bulk kill. Every succeeded target gets its own fadeEntry (so each row
// dissolves individually), but only the last one triggers the follow-up
// rescan — they all started dissolving at the same instant, so they finish
// together, and one rescan is enough to refresh the whole list.
func (m Model) handleBulkKillResult(msg bulkKillMsg) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	m.multiSelected = nil

	now := m.clock()
	killedCount := 0
	failedCount := 0
	for i, r := range msg.results {
		if r.err == nil && (r.result.Status == kill.StatusKilled || r.result.Status == kill.StatusAlreadyDead) {
			killedCount++
			m.fadingOut = append(m.fadingOut, fadeEntry{
				service:        r.target,
				startedAt:      now,
				triggersRescan: i == len(msg.results)-1,
			})
			continue
		}
		failedCount++
	}

	switch {
	case failedCount == 0:
		m.setEphemeralStatus(fmt.Sprintf("terminated %d services", killedCount), killDissolveDuration)
	case killedCount == 0:
		m.setEphemeralStatus(fmt.Sprintf("failed to terminate %d services", failedCount), ephemeralStatusDuration)
	default:
		m.setEphemeralStatus(fmt.Sprintf("terminated %d, failed %d", killedCount, failedCount), ephemeralStatusDuration)
	}

	if killedCount == 0 {
		// Nothing is dissolving, so nothing would otherwise trigger a
		// refresh — force one so a fully-failed bulk kill doesn't leave a
		// stale list.
		return m, m.scanCmd()
	}
	return m, nil
}

func (m Model) selected() *scan.Service {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	return &m.filtered[m.cursor]
}
