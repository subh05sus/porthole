//go:build linux

package notify

import "os/exec"

// NewDefaultNotifier shells out to notify-send (part of libnotify, present
// on virtually every desktop Linux distro), degrading to NoOp on a headless
// box or a minimal distro that doesn't have it. [compiles] — no Linux
// desktop session available in this dev environment to verify the balloon
// actually appears.
func NewDefaultNotifier() Notifier {
	if _, err := exec.LookPath("notify-send"); err != nil {
		return NoOp{}
	}
	return shellNotifier{
		bin: "notify-send",
		buildArgs: func(title, body string) []string {
			return []string{title, body}
		},
	}
}
