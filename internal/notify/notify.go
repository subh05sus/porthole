// Package notify sends best-effort desktop notifications for watch mode's
// --notify flag, by shelling out to whatever native notifier tool the OS
// already ships rather than adding a notification library dependency.
package notify

import "os/exec"

// Notifier sends a best-effort desktop notification. A failure to notify
// must never be fatal to the caller — watch mode keeps running either way
// — so implementations swallow their own errors instead of returning one,
// and must not block: Notify is called inline from the watch loop.
type Notifier interface {
	Notify(title, body string)
}

// NoOp does nothing. It's watch mode's default (no --notify flag) and the
// fallback wherever the platform's native notifier tool isn't found.
type NoOp struct{}

func (NoOp) Notify(string, string) {}

// shellNotifier shells out to a native notification binary. Generic across
// platforms — each OS's NewDefaultNotifier only supplies the binary name
// and how to build its argv from (title, body); see notify_linux.go,
// notify_darwin.go, notify_windows.go.
type shellNotifier struct {
	bin       string
	buildArgs func(title, body string) []string
}

// Notify runs the shell-out in a goroutine: some native notifiers (the
// Windows balloon-tip fallback in particular) hold the process open for a
// few seconds to keep the notification visible, and the watch loop that
// calls Notify must never block waiting for that.
func (s shellNotifier) Notify(title, body string) {
	args := s.buildArgs(title, body)
	go func() {
		cmd := exec.Command(s.bin, args...)
		_ = cmd.Run()
	}()
}
