//go:build windows

package tui

import (
	"os"

	"golang.org/x/sys/windows"
)

// EnableVirtualTerminal attempts to turn on ANSI/VT100 escape sequence
// processing for stdout (edge case B: cmd.exe and older PowerShell don't
// support it by default, so bubbletea's rendering would show raw escape
// codes instead of the TUI). Returns false on failure, in which case the
// caller must force the plain, zero-animation theme rather than risk a
// garbled screen.
func EnableVirtualTerminal() bool {
	h := windows.Handle(os.Stdout.Fd())

	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true // already on (e.g. Windows Terminal)
	}
	if err := windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING); err != nil {
		return false
	}
	return true
}
