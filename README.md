# porthole

A port kill switch and service viewer for your terminal.

See what is listening, know which project owns it, kill it in one keystroke — from an animated TUI or a scriptable CLI, backed by the same engine.

```bash
porthole                 # launch the interactive TUI
porthole list             # table to stdout
porthole kill 3000        # SIGTERM, escalate after 2s
```

Status: pre-release, under active development.
