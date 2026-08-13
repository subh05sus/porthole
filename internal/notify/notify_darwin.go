//go:build darwin

package notify

import (
	"fmt"
	"os/exec"
	"strings"
)

// NewDefaultNotifier shells out to osascript, part of every macOS install,
// degrading to NoOp if it's somehow missing. [compiles] — no macOS
// toolchain or hardware available in this dev environment to verify the
// notification actually appears.
func NewDefaultNotifier() Notifier {
	if _, err := exec.LookPath("osascript"); err != nil {
		return NoOp{}
	}
	return shellNotifier{
		bin: "osascript",
		buildArgs: func(title, body string) []string {
			script := fmt.Sprintf("display notification %s with title %s", appleScriptQuote(body), appleScriptQuote(title))
			return []string{"-e", script}
		},
	}
}

func appleScriptQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
