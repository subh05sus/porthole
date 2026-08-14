# FAQ

## Is it safe to use? What if it kills the wrong process?

Every kill goes through the same ladder: porthole re-verifies the target is still the exact process it scanned — by PID **and** start time — immediately before sending any signal. If the PID has been recycled by a different process since the scan (rare, but it happens), porthole refuses and tells you the process changed, instead of signaling whatever now happens to hold that PID. This check runs on every kill, from every entry point (CLI, TUI, web dashboard, daemon) — there's exactly one code path that actually signals a process, and every caller goes through it.

## Does porthole need admin/root/sudo?

Only to kill or inspect a process you don't own (e.g. a system service running as another user), or to manage firewall rules on Windows (`netsh` itself requires elevation for writes, not just porthole). Listing and killing your own processes needs no elevated privileges. Porthole never silently attempts a privileged action — if something needs elevation it doesn't have, it tells you, rather than failing silently or half-succeeding.

## How is this different from `lsof -i :PORT | xargs kill`, `fkill-cli`, or `npx kill-port`?

Those tools answer "what's the PID." Porthole answers "what's the PID, whose project is it, is it safe to kill right now, and what happens if I get it wrong." Specifically:

- **Project detection**: `node` on port 3000 doesn't tell you which of your five Node projects it is. Porthole walks up from the process's working directory to the nearest `package.json`/`go.mod`/`Cargo.toml`/`.git` and shows you the actual project name.
- **PID-reuse safety**: a plain `kill $(lsof -t -i:PORT)` has a real, if narrow, race — the PID it read could already belong to a different process by the time the signal lands. Porthole's kill ladder closes that gap.
- **It's not just a one-shot script**: a real interactive TUI (live watch mode, multi-select bulk kill, a detail pane with sockets/CPU/RSS), plus a scriptable CLI, a local web dashboard, and an opt-in auto-kill daemon — all backed by the same engine, not three different ad-hoc implementations.
- **Container awareness**: if a port is actually published by a Docker container, porthole shows you the container name/image and routes the kill through `docker stop` instead of signaling the host-side proxy process (which usually wouldn't even stop the container).

## Does porthole work with Docker/containers?

Yes. If a listening port is published by a running container (Docker or another Docker-Engine-API-compatible daemon), porthole shows the real container name and image instead of the generic `com.docker.backend`/`docker-proxy` process, and killing it runs `docker stop <container>` instead of signaling the forwarding process directly. If no container runtime is reachable, this degrades silently — porthole works exactly the same without Docker running.

## What's the auto-kill daemon, and is it safe to enable?

`porthole daemon` watches for an exact, user-authored `(port, process-name)` allow-list and kills matches automatically. It's the most dangerous feature porthole has, so:

- It's **disabled by default**, even if you've written allow-list entries.
- It's **dry-run by default** even when enabled — real kills need the `--live` flag every single time you start it, not a config toggle you can forget is on.
- It only ever matches **exact** `(port, process)` pairs — no wildcards or ranges.
- Every match is **re-verified against a fresh scan** immediately before acting, closing the gap where a port briefly matched an allow-listed name but the process behind it changed in between.
- It's **rate-limited** per port, so a crash-looping process can't be killed in a tight loop.

Read the daemon section of the README before enabling `--live` on anything you care about.

## What does `porthole firewall` actually touch?

Only ports porthole itself already showed you in a scan — never an arbitrary CIDR, range, or protocol. Every rule it creates lives in an isolated group it exclusively owns (a dedicated rule-name prefix on Windows, a dedicated iptables chain on Linux, a dedicated pf anchor on macOS), and it never lists, modifies, or deletes anything outside that group. There's no `--yes` flag anywhere in the firewall command tree — every change requires typing the exact port number to confirm.

## Does the web dashboard expose my machine to the network?

No. `porthole serve` binds `127.0.0.1` only, always — there's no flag to change that. It also checks the request `Origin` on the kill endpoint, so a malicious website you happen to have open in a browser tab can't trigger a kill just by shipping a background request to it.

## Which platforms are supported?

Linux, macOS, and Windows, all first-class — not "works on Linux, best-effort elsewhere." TCP and UDP, IPv4 and IPv6, on all three.

## Where do I report a bug or request a feature?

See [SUPPORT.md](SUPPORT.md) for where to ask questions, and the [issue templates](.github/ISSUE_TEMPLATE) for bug reports/feature requests. For anything security-sensitive, see [SECURITY.md](SECURITY.md) instead of a public issue.
