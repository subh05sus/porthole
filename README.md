# porthole

**A port kill switch and service viewer for your terminal.**

See what's listening on your machine, know which project owns it, and kill it in one keystroke — from an animated TUI or a scriptable CLI, backed by the same engine. Cross-platform: Linux, macOS, Windows.

[![CI](https://github.com/subh05sus/porthole/actions/workflows/ci.yml/badge.svg)](https://github.com/subh05sus/porthole/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/subh05sus/porthole.svg)](https://pkg.go.dev/github.com/subh05sus/porthole)
[![Go Report Card](https://goreportcard.com/badge/github.com/subh05sus/porthole)](https://goreportcard.com/report/github.com/subh05sus/porthole)
[![Latest release](https://img.shields.io/github/v/release/subh05sus/porthole)](https://github.com/subh05sus/porthole/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

---

## Why porthole

`lsof -i :3000 | grep LISTEN` and `kill -9 $(lsof -t -i:3000)` work, but they don't answer the question you actually have:

- **"Which of my projects owns this port?"** `node` on port 3000 is useless information. `zapmail-web` on port 3000 is the answer. Porthole walks up from the listening process's working directory to find the nearest `package.json`/`go.mod`/`Cargo.toml`/`.git` and shows you the project name, not just the process name.
- **"Is it safe to kill?"** Porthole verifies the process is still the one it scanned — by PID *and* start time — immediately before signaling it, so a recycled PID never gets the wrong process killed.
- **"What actually happens when I hit kill?"** SIGTERM, wait, escalate to SIGKILL only if it's ignored — the same ladder every time, whether you're in the TUI, the CLI, the daemon, or the web dashboard.

```
┌─ porthole ──────────────────────────────────────────────────────┐
│ PORT   PID    PROCESS   PROJECT       CONTAINER   UPTIME         │
│ 3000   48211  node      zapmail-web   -           2h 14m         │
│ 5432   1092   postgres  -             pg-dev       6d            │
│ 6379   2288   redis     -             -            18m           │
└────────────────────────────────────────────────────────────────┘
  ↑↓ nav · space select · k kill · K force · / filter · w watch · ? help
```

## Install

```bash
# Homebrew (macOS + Linux)
brew install subh05sus/tap/porthole

# Scoop (Windows)
scoop bucket add porthole https://github.com/subh05sus/scoop-bucket
scoop install porthole

# curl | bash (Linux/macOS)
curl -fsSL https://raw.githubusercontent.com/subh05sus/porthole/main/install.sh | bash

# go install (any platform with Go)
go install github.com/subh05sus/porthole/cmd/porthole@latest
```

Prebuilt binaries for Linux/macOS/Windows × amd64/arm64 are on the [releases page](https://github.com/subh05sus/porthole/releases).

## Quick start

```bash
porthole                 # launch the interactive TUI
porthole list             # table to stdout
porthole list --json      # for scripts/jq
porthole kill 3000        # SIGTERM, escalate after 2s if ignored
porthole kill 3000 8080   # multiple ports at once
porthole watch             # headless live tail, no TUI
```

## Commands

| Command | What it does |
|---|---|
| `porthole` | Launch the interactive TUI |
| `porthole list [--json\|--oneline] [--port N] [--project NAME] [--since 5m] [--containers]` | List listening services |
| `porthole kill <ports...> [--force] [--dry-run] [--yes] [--project NAME] [--dev]` | Kill whatever's listening on one or more ports |
| `porthole watch [--interval 2s] [--notify]` | Headless live tail — prints what changed, no TUI |
| `porthole restart <port>` | Kill a process, then respawn it with the same command/cwd/env |
| `porthole doctor` | Diagnose common permission/environment issues |
| `porthole history` | Show recent kills (who, when, what happened) |
| `porthole serve [--port 9191]` | Local-only web dashboard (binds `127.0.0.1`, never the network) |
| `porthole firewall block\|allow <port> [--proto tcp\|udp] [--out]` | Add an isolated firewall rule for a port porthole discovered |
| `porthole firewall list` / `porthole firewall clean` | List / remove porthole-owned firewall rules |
| `porthole daemon [--live] [--interval 5s]` | Auto-kill daemon — see [Auto-kill daemon](#auto-kill-daemon-opt-in) below |

Every command supports `--help` for the full flag list.

## TUI keybindings

| Key | Action |
|---|---|
| `↑` `↓` / `j` | Navigate |
| `enter` | Detail pane (full cmdline, cwd, env, sockets, CPU%/RSS) |
| `space` | Multi-select |
| `k` | Kill (graceful) |
| `K` | Force kill |
| `R` | Restart |
| `/` | Filter by port, process, or project |
| `w` | Toggle watch mode |
| `r` | Refresh |
| `?` | Help |
| `q` / `esc` | Quit |

## Configuration

Optional `~/.porthole.yaml` — porthole works fine with no config file at all.

```yaml
protected:
  - 5432
  - port: 3306
    reason: "prod tunnel, do not touch"

auto_kill:
  enabled: false          # opt-in, see below
  allow:
    - port: 3000
      process: node
```

**Protected ports** need a typed confirmation (the exact port number) instead of a plain y/n — and `--yes` does not bypass it, in either the CLI or the TUI.

### Auto-kill daemon (opt-in)

`porthole daemon` watches for services matching an exact `(port, process-name)` allow-list and kills them automatically. This is the most dangerous thing porthole can do, so it's built with rails on every side:

- **Disabled by default** — `auto_kill.enabled: true` is required even with allow entries present.
- **Dry-run by default**, even when enabled — real kills need the explicit `--live` flag, every time you start it. There's no config-file toggle for that, and nothing reachable from the TUI.
- **Exact matches only** — no wildcards, ranges, or patterns.
- **Re-verified immediately before every kill** against a fresh scan, not just trusted from when it first matched.
- **Rate-limited** — a cooldown per port so a crash-looping process can't be killed in a tight loop.
- Every kill goes through the exact same PID-reuse-guarded kill ladder as everything else in porthole.

```bash
porthole daemon                 # dry-run: logs what it would kill
porthole daemon --live          # actually kills matches
```

### Firewall rules

`porthole firewall` manages a small set of OS firewall rules scoped to exactly what porthole already showed you — one port, one protocol at a time, never a CIDR or a wildcard. Every rule lives in an isolated group porthole exclusively owns (a dedicated `netsh` rule name prefix on Windows, a dedicated `PORTHOLE` chain on Linux iptables, a dedicated `pf` anchor on macOS) and porthole never touches anything it didn't create itself. There's no `--yes` flag anywhere in this command tree — every change requires typing the exact port number to confirm.

## How it works

- **Kill ladder**: verify the target is still the process that was scanned (PID + start time) → SIGTERM → wait → SIGKILL only if still alive. The same ladder runs behind the CLI, the TUI, the web dashboard, and the daemon — one code path, everywhere.
- **Project detection**: walks up from the process's working directory looking for `package.json`, `go.mod`, `Cargo.toml`, `pyproject.toml`, or `.git`, with the result cached per-session and persisted across runs.
- **Container awareness**: cross-references listening ports against the Docker Engine API's published-port list, so a container-forwarded port shows the real container name/image, and killing it routes to `docker stop` instead of signaling the host-side proxy process.
- **Cross-platform by construction**: every OS-specific piece (socket scanning, process metadata, signaling) sits behind a small interface, with the OS-agnostic logic (the kill ladder, project detection, config, filtering) fully portable and unit-tested independent of any real OS.

## Building from source

```bash
git clone https://github.com/subh05sus/porthole.git
cd porthole
go build -o porthole ./cmd/porthole
go test ./...
```

Requires Go 1.26+. No cgo, no external dependencies beyond the Go module graph — porthole ships as a single static binary.

## Contributing

Bug reports, feature requests, and PRs are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

Porthole kills processes, manages firewall rules, and can auto-kill things unsupervised — if you find a security issue, please report it responsibly. See [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
