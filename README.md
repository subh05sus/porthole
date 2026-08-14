# porthole

A port kill switch and service viewer for your terminal.

See what is listening, know which project owns it, kill it in one keystroke — from an animated TUI or a scriptable CLI, backed by the same engine.

```bash
porthole                 # launch the interactive TUI
porthole list             # table to stdout
porthole kill 3000        # SIGTERM, escalate after 2s
```

## Install

```bash
go install github.com/subh05sus/porthole/cmd/porthole@latest
```

Prebuilt binaries, a Homebrew tap, and a Scoop bucket are planned (see `.goreleaser.yaml` and `install.sh`) but not published yet — `go install` is the only way to get porthole today.

Status: pre-release, under active development.
