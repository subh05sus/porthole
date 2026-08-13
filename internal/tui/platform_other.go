//go:build !windows

package tui

// EnableVirtualTerminal is a no-op on Unix terminals, which support ANSI
// escape sequences natively.
func EnableVirtualTerminal() bool { return true }
