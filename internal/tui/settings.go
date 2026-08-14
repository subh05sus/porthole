package tui

import (
	"fmt"
	"strings"
)

// settingsView renders every value currently loaded from ~/.porthole.yaml
// (or the built-in defaults, if the file is absent/a field was never set)
// — the one place in the TUI that shows the actual config values, as
// opposed to just their effects (the 🛡/🐳 glyphs, hidden rows, etc.).
func (m Model) settingsView() string {
	cfg := m.cfg

	var b strings.Builder
	b.WriteString(m.th.Header.Render("porthole — settings (~/.porthole.yaml)"))
	b.WriteString("\n\n")

	row := func(label, value string) {
		b.WriteString(m.th.Muted.Render(padColumns(label, 22)))
		b.WriteString(value)
		b.WriteString("\n")
	}
	section := func(title string) {
		b.WriteString("\n")
		b.WriteString(m.th.Muted.Render(title))
		b.WriteString("\n")
	}

	row("theme", cfg.Theme)
	row("animations", fmt.Sprintf("%v", cfg.Animations))

	section("display")
	row("  hide_system_processes", fmt.Sprintf("%v", cfg.Display.HideSystemProcesses))
	row("  hide_privileged_ports", fmt.Sprintf("%v", cfg.Display.HidePrivilegedPorts))

	section("kill")
	row("  dev_port_range", nonEmpty(cfg.Kill.DevPortRange, "-"))
	row("  escalation_timeout", cfg.Kill.EscalationTimeout.String())

	section("watch")
	row("  interval", cfg.Watch.Interval.String())

	section("auto-kill daemon (auto_kill)")
	row("  enabled", fmt.Sprintf("%v", cfg.AutoKill.Enabled))
	row("  interval", cfg.AutoKill.Interval.String())
	if len(cfg.AutoKill.Allow) == 0 {
		row("  allow-list", "(empty)")
	} else {
		row("  allow-list", fmt.Sprintf("%d entr(y/ies)", len(cfg.AutoKill.Allow)))
		for _, e := range cfg.AutoKill.Allow {
			b.WriteString(fmt.Sprintf("    :%d  %s\n", e.Port, e.Process))
		}
	}

	section("protected ports")
	if len(cfg.Protected) == 0 {
		b.WriteString("  (none)\n")
	} else {
		for _, p := range cfg.Protected {
			line := fmt.Sprintf("  :%d", p.Port)
			if p.Reason != "" {
				line += " — " + p.Reason
			}
			b.WriteString(line + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.th.HintBar.Render("any key to close"))
	return b.String()
}
