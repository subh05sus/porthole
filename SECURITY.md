# Security Policy

Porthole terminates processes, modifies OS firewall rules, and — if you opt in — can kill processes unsupervised via `porthole daemon`. Please take security reports about it seriously, and we will too.

## Reporting a vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Instead, use GitHub's private vulnerability reporting: go to the [Security tab](https://github.com/subh05sus/porthole/security/advisories/new) on this repository and open a new draft advisory. If that's not available for you, open a regular issue asking for a private contact channel without describing the vulnerability itself.

Please include:

- Porthole version (`porthole --version`) and OS/architecture
- Steps to reproduce
- What you expected vs. what happened
- Impact, as you understand it (e.g. "kills a process the user didn't select," "escapes the firewall rule isolation group," "auto-kill daemon acts without the allow-list matching")

We'll acknowledge reports as promptly as we can and keep you updated as a fix is worked on.

## Scope

In scope:

- The kill ladder incorrectly signaling a process other than the one the user selected (in particular, any PID-reuse bypass)
- `porthole firewall` rules affecting anything outside porthole's own isolated rule group/chain/anchor
- `porthole daemon` acting outside its documented safety rails (disabled-by-default, dry-run-by-default without `--live`, matching outside the exact allow-list, skipping the pre-kill re-verification, ignoring the rate limit)
- `porthole serve`'s web dashboard: anything reachable from a non-localhost origin, or a cross-origin request able to trigger a kill
- Privilege escalation of any kind
- Memory-safety issues from the Windows PEB-reading code (`internal/proc/peb_windows.go`, `peb_wow64_windows.go`) — this code reads undocumented Windows internals and fails closed by design, but a bug there is still worth reporting

Out of scope:

- A killed process the user explicitly targeted behaving as intended
- Denial-of-service via scanning/killing ports you already have local-user permission to see and kill (porthole has no privilege beyond what the invoking user already has)
- Issues requiring physical access or an already-compromised machine

## Design notes for reviewers

If you're auditing this codebase, the places worth reading most carefully:

- `internal/kill/kill.go` — the ladder's `verifyAndSignal`, the single choke point every signal passes through
- `internal/daemon/daemon.go` — package doc comment lists every safety rail and where it's enforced
- `internal/firewall/*.go` — `RulePrefix`-based ownership check, present in every `List`/`Remove`/`RemoveAll` implementation
- `internal/webui/webui.go` — `sameOrigin`, the CSRF mitigation on the kill endpoint

## Supported versions

Only the latest released version is supported with security fixes.
