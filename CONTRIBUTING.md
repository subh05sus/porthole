# Contributing to porthole

Thanks for considering a contribution. This project is a Go CLI+TUI, cross-platform (Linux/macOS/Windows), with a strict split between portable logic and OS-specific glue — that split is the thing to understand before making non-trivial changes.

## Development setup

```bash
git clone https://github.com/subh05sus/porthole.git
cd porthole
go build -o porthole ./cmd/porthole
go test ./...
```

Requires Go 1.26+. No cgo, no external system dependencies.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full picture: the package map, the portable/OS-glue split (the thing to understand before making any non-trivial change), the kill ladder's algorithm, and the TUI's concurrency model. The short version: every OS-specific concern (parsing `/proc/net/tcp`, shelling out to `lsof`, calling Win32 APIs) is split into a pure, build-tag-free parser (real `go test` coverage on any host OS) and a thin OS-glue layer that does the actual file read/exec/syscall. If you're adding a new OS-specific data source, follow that split rather than inlining parsing logic into the glue file — it's the difference between "testable everywhere" and "only compile-checked outside of CI."

## Testing expectations

- Every PR should include tests for new logic. Look at the sibling `*_test.go` file for the pattern already in use in that package.
- OS-specific glue that can't be tested on your machine (e.g. you're on Linux and touching `proc_windows.go`) should at minimum cross-compile: `GOOS=windows go build ./...`. Say so explicitly in the PR description — this project has a strict "label what's actually tested vs. only compile-checked" discipline, and CI (`.github/workflows/ci.yml`) builds and tests on all three platforms.
- Fakes live in `<pkg>test` sibling packages (`scan/scantest`, `kill/killtest`, `proc/proctest`, `firewall/firewalltest`, `restart/restarttest`) — reuse the existing fake rather than writing a one-off mock inline.

## Safety-sensitive areas

`internal/kill` (the PID-reuse guard specifically), `internal/firewall`, and `internal/daemon` (the auto-kill daemon) get held to a higher bar:

- Any change to the kill ladder's re-verification logic needs a test proving the PID-reuse guard still holds.
- Firewall rule changes must stay scoped to porthole's own isolated rule group/chain/anchor — never touch a pre-existing rule.
- Daemon changes must preserve every existing safety rail (disabled-by-default, dry-run-by-default, exact-match-only, re-verify-before-acting, rate-limiting) — if you're relaxing one of these, explain why in the PR description, don't just remove it.

## Commit style

Look at `git log` for the existing convention: `<area>: <what changed>`, imperative mood, body explaining *why* when it's not obvious from the diff. Small, focused commits over one large one.

## Reporting bugs / requesting features

Use the issue templates — they ask for the information that's actually needed to act on a report (OS, porthole version, exact steps). See [SECURITY.md](SECURITY.md) instead for anything security-sensitive.
