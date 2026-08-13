//go:build windows

package notify

import (
	"fmt"
	"os/exec"
	"strings"
)

// NewDefaultNotifier shells out to PowerShell for a System.Windows.Forms
// balloon-tip notification — not a "real" WinRT toast (that needs an AppID
// registered via a packaged app, which a single static binary doesn't
// have), but System.Windows.Forms ships with every Windows install via
// .NET Framework, so this works with no extra dependency. Degrades to NoOp
// if powershell.exe isn't on PATH (unusual, but possible on a locked-down
// box). [tested, live] — verified the PowerShell script runs and exits 0
// against this real machine; whether the balloon is visually rendered on
// screen isn't something this environment can confirm, same limitation as
// the TUI's own unverified visual rendering.
func NewDefaultNotifier() Notifier {
	if _, err := exec.LookPath("powershell"); err != nil {
		return NoOp{}
	}
	return shellNotifier{
		bin: "powershell",
		buildArgs: func(title, body string) []string {
			script := fmt.Sprintf(
				"Add-Type -AssemblyName System.Windows.Forms; "+
					"$n = New-Object System.Windows.Forms.NotifyIcon; "+
					"$n.Icon = [System.Drawing.SystemIcons]::Information; "+
					"$n.Visible = $true; "+
					"$n.ShowBalloonTip(4000, %s, %s, [System.Windows.Forms.ToolTipIcon]::Info); "+
					"Start-Sleep -Milliseconds 4200; "+
					"$n.Dispose()",
				psQuote(title), psQuote(body),
			)
			return []string{"-NoProfile", "-NonInteractive", "-Command", script}
		},
	}
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
