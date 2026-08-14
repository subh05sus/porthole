# Changelog

All notable changes to this project are documented here. Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project uses [Semantic Versioning](https://semver.org/).

## [1.3.0] — 2026-08-14

### Added

- **Editable TUI settings** (`S`): the settings screen is now a full editor, not just a viewer — toggle bools, cycle the theme, edit the dev port range/timeouts/intervals inline, and add or delete protected-port and auto-kill allow-list entries, all from the TUI. Edits happen on an in-memory draft; nothing touches disk until an explicit `s` save, and an invalid value is rejected inline rather than silently written.

## [1.0.0] — 2026-08-14

Initial public release.

### Added

- **Core engine**: cross-platform (Linux/macOS/Windows) listening-socket discovery, TCP and UDP, IPv4 and IPv6.
- **Kill ladder**: SIGTERM → wait → SIGKILL escalation, with a PID-reuse guard that re-verifies a process's identity (PID + start time) immediately before every signal.
- **Interactive TUI**: animated Bubble Tea interface — navigate, filter, kill, force-kill, multi-select bulk kill, a detail pane (full cmdline/cwd/env, every socket a process holds, on-demand CPU%/RSS), and a live watch mode with a pulsing status indicator.
- **Scriptable CLI**: `list` (table/JSON/one-line, filterable by port/project/uptime/container), `kill` (single/multiple/range, `--dev` preset, `--dry-run`), `watch` (headless live tail with optional desktop notifications), `restart` (kill-and-respawn with the same cmdline/cwd/env), `doctor` (environment diagnostics), `history` (kill audit log).
- **Project detection**: resolves the project name (not just the process name) owning a port by walking up from its working directory to the nearest `package.json`/`go.mod`/`Cargo.toml`/`.git`, cached across runs.
- **Protected ports**: an optional `~/.porthole.yaml` allow-list requiring typed confirmation (not just y/n) before killing specific ports — no `--yes` bypass.
- **Container awareness**: enriches listening services with the real container name/image when a port is published by a running Docker (or Docker-Engine-API-compatible) container, and routes kills through `docker stop` instead of signaling the host-side forwarding process.
- **Web dashboard** (`porthole serve`): a localhost-only (never network-exposed) live dashboard with the same list/kill capability as the CLI, with CSRF protection on the kill endpoint.
- **Firewall rule management** (`porthole firewall`): block/allow specific ports via an isolated, porthole-owned rule group — never touching pre-existing firewall rules — with typed confirmation and no bypass flag.
- **Auto-kill daemon** (`porthole daemon`): disabled by default, dry-run by default, exact-match allow-list only, re-verifies every match against a fresh scan immediately before acting, rate-limited per port.
- Shell completions (bash/zsh/fish/PowerShell) via Cobra.

[1.3.0]: https://github.com/subh05sus/porthole/releases/tag/v1.3.0
[1.0.0]: https://github.com/subh05sus/porthole/releases/tag/v1.0.0
