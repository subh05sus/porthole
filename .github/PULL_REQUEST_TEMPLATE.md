## What does this change?

A short description of the change and why it's needed.

## Testing

- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] Cross-compiles for platforms you didn't test natively: `GOOS=linux go build ./...` / `GOOS=darwin go build ./...` / `GOOS=windows go build ./...` (delete lines for platforms you did test natively)
- [ ] New logic has test coverage; OS-specific code you can't run locally is at minimum cross-compiled, and this is noted below

**What's actually been run/verified, vs. only compile-checked?** (This project labels this explicitly — see CONTRIBUTING.md.)

## Safety-sensitive changes

If this touches `internal/kill` (the PID-reuse guard), `internal/firewall`, or `internal/daemon` (the auto-kill daemon): explain what safety property you verified still holds, or what you're deliberately changing and why.

## Checklist

- [ ] I've read [CONTRIBUTING.md](../CONTRIBUTING.md)
- [ ] This PR is scoped to one change (not a bundle of unrelated fixes)
