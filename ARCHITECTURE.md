# Architecture

How porthole is put together, and why. Aimed at contributors and anyone evaluating whether to trust this codebase with something as consequential as killing processes and writing firewall rules.

## The core idea: portable logic, thin OS glue

Almost every OS-specific concern in porthole — reading `/proc`, shelling out to `lsof`, calling Win32 APIs, running `netsh`/`iptables`/`pfctl` — is split into two halves:

1. A **pure function or algorithm**, with no build tags and no I/O of its own. It takes bytes or an `io.Reader` in, and returns parsed structs out.
2. A thin **OS glue layer**, gated behind a `//go:build linux`/`darwin`/`windows` tag, that does the actual file read / `exec.Command` / syscall and hands the raw result to the pure half.

The payoff: the pure half gets real `go test` coverage on *any* host OS, including CI runners and a contributor's own machine regardless of what they're running — not just a `GOOS=linux go build` compile-check. The glue half is small enough that when it can't be run locally, cross-compiling it at least catches type errors, and the project is explicit in commit messages and `CONTRIBUTING.md` about which is which for any given piece of code.

Examples of this split:

| Pure, portable | OS glue |
|---|---|
| `scan/procfmt.ParseTCPTable` | `scan_linux.go` (reads `/proc/net/tcp`) |
| `scan/lsoffmt.ParseLsofFields` | `scan_darwin.go` (shells out to `lsof`) |
| `firewall/firewallfmt.ParseNetshRules` | `firewall_windows.go` (shells out to `netsh`) |
| `kill.Ladder` (the whole algorithm) | `kill_unix.go`/`kill_windows.go` (implement `Signaler`) |

## The kill ladder

`internal/kill.Ladder` is the one place in the codebase that's actually allowed to send a signal to a process, and every kill — from the CLI, the TUI, the web dashboard, or the auto-kill daemon — goes through it.

```
Execute(target, opts):
  verifyAndSignal(target, force=opts.Force)
    → StillAlive(target.PID)?
        no  → return AlreadyDead
        yes → does its start-time match target.StartTime?
                no  → return ErrPIDReused (abort — do NOT signal)
                yes → send Terminate (or Kill if force)
  pollUntilDead(interval, timeout)
    dead?     → return Killed
    timeout?  → NeedsEscalation (caller decides, or AutoEscalate retries with Kill)
```

The PID-reuse guard is the load-bearing part: `target.StartTime` is captured at scan time, and re-checked against the *current* process's start time immediately before every single signal — not just once at the start of `Execute`. If a PID gets recycled by an unrelated process between the scan and the kill (rare, but a real race on a busy machine), porthole aborts rather than signaling whatever now happens to hold that PID.

`Ladder` itself has zero OS-specific code — it's driven entirely by a small `Signaler` interface (`StillAlive`/`Terminate`/`Kill`), which is what makes the ladder, PID-reuse guard included, fully unit-testable against a fake `Signaler` without ever touching a real OS process.

## Data flow

```
scan.Lister.List(ctx)          — one call, one platform-specific implementation
        │
        ▼
[]scan.Service                 — one struct per listening socket
        │
        ├─→ project.Detector.Detect(cwd)     — walks up to nearest package.json/go.mod/.../.git
        ├─→ container.EnrichServices(...)     — cross-references Docker's published-port list
        │
        ▼
[]scan.Service (enriched)
        │
   ┌────┼────────────┬──────────────┬───────────────┐
   ▼    ▼             ▼              ▼               ▼
 CLI   TUI      web dashboard   auto-kill daemon   watch mode
```

`container.AwareLister` and `container.AwareKiller` are decorators wrapping the plain `scan.Lister`/`kill.Killer` — the same shape `history.LoggingKiller` uses to add kill-history logging. Every entry point receives the same fully-decorated `Lister`/`Killer` from `cmd/porthole/main.go`, so there's exactly one place that assembles "what a scan/kill actually does," not one implementation per entry point.

## Package map

- **`internal/scan`** — socket discovery. `Lister` interface, one implementation per OS, plus the `Service` struct every other package operates on. `scan/procfmt` and `scan/lsoffmt` hold the pure Linux/macOS parsers.
- **`internal/kill`** — the ladder described above.
- **`internal/proc`** — per-process metadata (cmdline, cwd, user, start time, CPU%/RSS) beyond what a socket scan reveals.
- **`internal/project`** — project-name detection, with a session-and-disk-persisted cache keyed by `(cwd, marker-file mtime)`.
- **`internal/container`** — a minimal Docker Engine API client (no `docker/docker` SDK dependency — it's plain HTTP over a Unix socket/named pipe) plus the `AwareLister`/`AwareKiller` decorators.
- **`internal/firewall`** — isolated firewall rule management, one `Manager` implementation per OS, sharing a `RulePrefix`-based ownership convention so no implementation ever touches a rule it didn't create.
- **`internal/daemon`** — the auto-kill daemon's polling loop; see its package doc comment for the full safety-rail list.
- **`internal/config`** — optional `~/.porthole.yaml` parsing.
- **`internal/history`** — a `kill.Killer` decorator that appends every kill (any entry point) to a JSONL log.
- **`internal/notify`** — desktop notifications for watch mode, shelling out to whatever native notifier the OS ships (`notify-send`/`osascript`/a PowerShell balloon tip) rather than adding a notification library dependency.
- **`internal/restart`** — captures a process's cmdline/cwd/env before killing it, then respawns it.
- **`internal/webui`** — the `porthole serve` dashboard: a small JSON API plus one `go:embed`-ed HTML/JS page, no build step.
- **`internal/cli`** — the Cobra command tree. Every subcommand takes its dependencies via an `App` struct rather than reaching for globals, which is what makes CLI-layer tests possible without a real OS (see Testing below).
- **`internal/tui`** — the interactive terminal UI (Bubble Tea).

## The TUI's concurrency model

The TUI follows Bubble Tea's Elm-style architecture strictly: `Update` never calls the scanner, killer, or any other OS-facing dependency directly — every such interaction is dispatched as a `tea.Cmd`, a function that runs off the UI goroutine and returns a `tea.Msg` that `Update` handles on the next event-loop pass. Concretely: one recurring 60ms `tea.Tick` drives every animation (staggered row reveal, fade-out on kill, the spinner, the watch-mode pulse) — there are no per-row goroutines and no independently-scheduled timers stacking up. This is also what makes the TUI deterministically testable: `model_test.go` drives the whole thing with an injectable clock instead of racing real wall-clock time, and a `runCmd` test helper executes a `tea.Cmd` synchronously and feeds its result back into `Update`, exactly mirroring what Bubble Tea's real runtime does.

## Build-tag-narrowing factory pattern

Several packages (`scan`, `kill`, `firewall`, `proc`) follow the same pattern for their `NewDefaultX()` constructor: an untagged (or negative-build-tag) fallback file provides a "not implemented on this platform" stub, and as each OS's real implementation lands, that fallback file's build tag narrows (e.g. `//go:build !linux && !darwin && !windows`) so exactly one implementation of `NewDefaultX` is visible to the compiler for any given `GOOS` — including exotic ones nobody's written a real backend for yet, which still get a working (if inert) build rather than a compile error.

## Testing philosophy

- **Fakes live in sibling `*test` packages** (`scan/scantest`, `kill/killtest`, `proc/proctest`, `firewall/firewalltest`, `restart/restarttest`, `container` has its own inline fakes) — reused across every package that needs them, rather than one-off mocks per test file.
- **"Tested" vs. "compiles"** is a real, load-bearing distinction in this codebase's commit history and `CONTRIBUTING.md`: a pure parser gets fixture-based `go test` coverage regardless of host OS; OS glue that can't be exercised locally is at minimum cross-compiled (`GOOS=X go build ./...`), and PRs are expected to say explicitly which is which rather than let a commit message imply more verification happened than actually did.
- **Safety-critical logic gets extra scrutiny**: the kill ladder's PID-reuse guard, firewall rule isolation, and the auto-kill daemon's safety rails all have dedicated tests proving the specific property that matters (not just "doesn't crash") — e.g. the daemon has separate tests for "the process is gone by re-verify time" and "a *different* process now owns the same port," not just one generic "re-verify failed" case.
