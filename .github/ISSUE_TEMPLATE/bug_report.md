---
name: Bug report
about: Something isn't working the way it should
title: ""
labels: bug
assignees: ""
---

**Describe the bug**
A clear, concise description of what's wrong.

**To reproduce**
Exact steps to reproduce the behavior, including the exact command(s) you ran.

**Expected behavior**
What you expected to happen instead.

**Environment**
- porthole version: (`porthole --version`)
- OS/architecture: (e.g. Windows 11 x64, macOS 14 arm64, Ubuntu 24.04 x64)
- Install method: (Homebrew / Scoop / install.sh / go install / built from source)
- Terminal: (e.g. Windows Terminal, iTerm2, GNOME Terminal) — only relevant for TUI rendering issues

**Output**
If applicable, paste the actual output (use `--json` or a screenshot for TUI issues). Please redact anything sensitive (paths, hostnames, etc. are usually fine to leave in — process command lines occasionally aren't).

**Additional context**
Anything else that might be relevant.

---

If this is a security issue (a process getting killed that shouldn't have been, a firewall rule leaking outside porthole's isolated group, the auto-kill daemon acting outside its safety rails), please use [SECURITY.md](../../SECURITY.md) instead of a public issue.
