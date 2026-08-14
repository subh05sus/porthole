# Roadmap

Porthole is under active development. This is a snapshot of what's shipped and what's next — not a promise of dates, and open to reprioritization based on issues/feedback.

## Shipped (v1.0.0)

- Cross-platform TCP/UDP socket discovery (Linux, macOS, Windows; IPv4 + IPv6)
- Interactive TUI: navigate, filter, kill/force-kill, multi-select bulk kill, live watch mode, a detail pane (full process info, every socket held, on-demand CPU%/RSS)
- Scriptable CLI: `list`, `kill`, `watch`, `restart`, `doctor`, `history`
- Project detection (walks up from a process's cwd to the nearest `package.json`/`go.mod`/`Cargo.toml`/`.git`)
- Protected ports with typed confirmation, no bypass
- Container awareness (Docker Engine API — real container name/image, `docker stop` kill routing)
- Local-only web dashboard (`porthole serve`)
- Isolated firewall rule management (`porthole firewall`)
- Opt-in, safety-railed auto-kill daemon (`porthole daemon`)
- Distribution: Homebrew tap, Scoop bucket, `go install`, prebuilt binaries for 6 platform/arch combos
- Configurable defaults (`~/.porthole.yaml`): display filters (`hide_system_processes`/`hide_privileged_ports`, with `--all`/`a` per-run overrides), theme (`auto`/`color`/`plain`), animations on/off, the `--dev` port range, the kill ladder's escalation timeout, and `watch`/`auto_kill` poll intervals

## Known gaps

Being upfront about what's not fully verified yet:

- **Linux/macOS real-world testing.** A meaningful amount of the OS-specific code (the `/proc`-parsing and `lsof`-shelling glue especially) has been built and cross-compiled carefully but not run on real Linux/macOS hardware yet. If you hit something that only shows up there, that's exactly the kind of report that's most valuable right now.
- **Traffic inspection** (`porthole sniff`) is not implemented. It needs `gopacket`/libpcap, which needs cgo and, on Windows, the separately-installed Npcap driver — a real tension with shipping a single static binary. Open to revisiting this if there's real demand.
- **AUR packaging** isn't published yet (the PKGBUILD template exists in `packaging/aur/`, just not submitted).
- `~/.porthole.yaml`'s `default_signal` field is still parsed but not wired to any behavior. (`theme` and `animations` — previously listed here too — are now fully wired; see the README's Configuration section. `default_signal` needs its own design pass, since there's no cross-platform equivalent of "pick a different initial signal" the way Unix's SIGTERM/SIGKILL split has — Windows has no distinct graceful signal at all.)

## Under consideration

No commitments here, just things that have come up:

- Native macOS process inspection via `libproc` instead of shelling out to `ps`/`lsof` (perf, one fewer external dependency)
- Remote/SSH host inspection
- A rendered demo GIF for the README (the VHS script already exists at `demo.tape`, just needs someone with `vhs`/`ttyd`/`ffmpeg` installed to run it)

## How to influence this

Open an issue (see [SUPPORT.md](SUPPORT.md)) — real usage and real friction move this list more than speculation does. If you're picking up a "known gap" or "under consideration" item, check [CONTRIBUTING.md](CONTRIBUTING.md) first, especially the section on safety-sensitive areas.
