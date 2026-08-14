# Examples

Practical recipes beyond the README's quick start. All output shapes below are pulled directly from the actual command flags, not approximated.

## Basic kills

```bash
# Kill whatever's on port 3000, confirm interactively
porthole kill 3000

# Kill several ports at once
porthole kill 3000 8080 5173

# Skip the confirmation prompt (scripts, CI)
porthole kill 3000 --yes

# See what would be killed without doing it
porthole kill 3000 --dry-run

# Ignored SIGTERM? Skip straight to SIGKILL
porthole kill 3000 --force
```

## Clearing out everything you're running for local dev

```bash
# Kill everything you own listening in the configured dev port range
# (kill.dev_port_range in ~/.porthole.yaml, default 3000-9999). Locked
# (not-owned) rows and empty ports are silently skipped, since this is a
# broad sweep, not a precise target list.
porthole kill --dev
```

## Killing by project instead of by port

```bash
# If porthole's project detection resolved a name (walks up from the
# process's cwd to the nearest package.json/go.mod/Cargo.toml/.git),
# you can target it directly instead of hunting for the port number.
porthole kill --project zapmail-web
```

## Scripting with `--json`

```bash
porthole list --json
```

```json
[
  {
    "port": 3000,
    "proto": "tcp",
    "addr": "127.0.0.1",
    "pid": 48211,
    "process": "node",
    "cmdline": "node server.js",
    "user": "sub",
    "cwd": "/home/sub/zapmail-web",
    "project": "zapmail-web",
    "uptime_seconds": 8040,
    "owned": true
  }
]
```

Pipe it through `jq`:

```bash
# Every port your own postgres is listening on
porthole list --json | jq '.[] | select(.process == "postgres") | .port'

# Every service porthole couldn't fully resolve (permission gaps)
porthole list --json | jq '.[] | select(.resolve_error != null)'
```

A `resolve_error` field appears only when something couldn't be fully resolved (e.g. a permission gap on another user's process) — it's never used to silently drop a row, so a service is always represented even when partially unresolved.

## Status-bar integration (tmux/zellij)

```bash
porthole list --oneline
# 4 services · 1 locked
```

Wire it into a tmux status line:

```tmux
set -g status-right "#(porthole list --oneline)"
```

## Filtering

```bash
porthole list --port 5432
porthole list --project zapmail-web
porthole list --since 5m          # only services started in the last 5 minutes
porthole list --containers        # only services published by a running container
```

## Hiding system/privileged noise by default

If you mostly care about your own dev services, set a default in `~/.porthole.yaml` instead of squinting past dozens of locked system rows every time:

```yaml
display:
  hide_system_processes: true   # hide rows you don't own (can't kill anyway)
  hide_privileged_ports: true   # hide ports < 1024, regardless of ownership
```

```bash
porthole list          # only your own, non-privileged services now
porthole list --all    # see everything for just this one run
```

Same config applies to `watch`, and the TUI has an `a` key that does the same thing as `--all` for the current session. None of this ever changes what `porthole kill <port>` can target — it's display-only, so a hidden row is still there, just not shown by default.

## Watch mode

```bash
# Headless live tail — no TUI, just a diff printed on each change
porthole watch

# Rescan every 5s instead of the default 2s
porthole watch --interval 5s

# Same thing, but as a persistent default instead of typing the flag every
# time — ~/.porthole.yaml:
#   watch:
#     interval: 5s
# The --interval flag still overrides this for a single run when passed.

# Get a desktop notification whenever a new service starts listening
porthole watch --notify

# See everything for this run, ignoring display.hide_* settings
porthole watch --all
```

Sample output:

```
PORT  PID    PROCESS  PROJECT       UPTIME
3000  48211  node     zapmail-web   2h 14m
+ python on :8000 (pid 51022)
- node on :3000 (pid 48211)
```

## Restart a process in place

```bash
# Kills the process on :3000, then respawns it with the exact same
# command line, working directory, and environment it had before.
porthole restart 3000
```

Refuses on container-backed ports (the captured command line would belong to the host-side Docker forwarding process, not the containerized one) — use `docker restart <container>` for those instead.

## Protecting a port from accidental kills

`~/.porthole.yaml`:

```yaml
protected:
  - 5432
  - port: 3306
    reason: "prod tunnel, do not touch"
```

```bash
$ porthole kill 5432
port :5432 is protected. Type 5432 to confirm killing postgres:
```

`--yes` does **not** bypass this, in either the CLI or the TUI — protected ports always need the typed confirmation.

## Auto-kill daemon walkthrough

Start conservative — allow-list one thing, dry-run first:

```yaml
# ~/.porthole.yaml
auto_kill:
  enabled: true
  interval: 5s      # how often the daemon polls — separate from the cooldown below
  allow:
    - port: 3000
      process: node
```

```bash
# Dry-run: logs matches, kills nothing
porthole daemon
# porthole daemon starting (dry-run mode, 1 allow-list entry)
# [dry-run] would kill node on :3000 (pid 48211)

# Poll more often than the configured default for this one run
porthole daemon --interval 2s

# Once you trust it, actually enable real kills
porthole daemon --live
```

Every match is re-verified against a fresh scan immediately before acting, and each port has a 30s cooldown after any kill attempt — a crash-looping process can't be killed in a tight loop. That cooldown isn't the same thing as `auto_kill.interval` above: interval is how often the daemon *looks*, cooldown is how long it waits before acting on the *same port* again after a kill — and the cooldown itself isn't configurable.

## Firewall rules

```bash
# Block inbound TCP traffic to a port you don't want reachable
porthole firewall block 8080
# this will add an isolated firewall rule: block inbound TCP port 8080 (rule name: porthole-in-block-8080-tcp)
# type 8080 to confirm:

# Allow instead of block, on UDP, for outbound
porthole firewall allow 53 --proto udp --out

# See everything porthole has added
porthole firewall list

# Remove every porthole-owned rule (typed CLEAN confirmation, not y/n)
porthole firewall clean
```

## Debugging a container's forwarded port

```bash
$ porthole list --containers
PORT  PID    PROCESS             PROJECT   CONTAINER          UPTIME
5432  22368  com.docker.backend  -         my-postgres-dev    3h 2m
```

Killing that row runs `docker stop my-postgres-dev` instead of signaling the host-side forwarding process (which usually wouldn't stop the container anyway).

## Web dashboard

```bash
porthole serve
# porthole dashboard: http://127.0.0.1:9191 (Ctrl+C to stop)

porthole serve --port 8888   # different local port
```

Binds `127.0.0.1` only, always — there's no flag to expose it to the network.

## Diagnosing environment issues

```bash
porthole doctor
```

Checks OS-specific prerequisites (e.g. `lsof` present on macOS, `/proc` readable on Linux), current permission level, `NO_COLOR`/terminal color support, and whether a container runtime is reachable.

## Kill history / audit log

```bash
porthole history
```

Shows recent kills — target, timestamp, and outcome — from every entry point (CLI, TUI, web dashboard, daemon), since they all funnel through the same logging layer.
