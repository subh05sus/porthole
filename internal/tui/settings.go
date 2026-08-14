package tui

import (
	"fmt"
	"strings"

	"github.com/subh05sus/porthole/internal/config"
)

// settingsRowKind identifies both what a settings row displays and how
// 'enter' should behave on it — a toggle, an enum cycle, a text-edit
// sub-mode, a list entry (deletable via 'd', otherwise inert), or an
// "add a new entry" action row.
type settingsRowKind int

const (
	rowTheme settingsRowKind = iota
	rowAnimations
	rowHideSystemProcesses
	rowHidePrivilegedPorts
	rowDevPortRange
	rowEscalationTimeout
	rowWatchInterval
	rowAutoKillEnabled
	rowAutoKillInterval
	rowAutoKillEntry
	rowAddAutoKillEntry
	rowProtectedEntry
	rowAddProtectedEntry
)

type settingsRow struct {
	kind  settingsRowKind
	label string
	value string
	// index is only meaningful for rowAutoKillEntry/rowProtectedEntry —
	// the entry's position in cfg.AutoKill.Allow/cfg.Protected.
	index int
}

// settingsRows builds the full addressable row list from cfg, rebuilt
// fresh on every render/key event rather than cached — cfg.AutoKill.Allow
// and cfg.Protected can grow/shrink as the user adds/deletes entries, so
// there's no stable row count to memoize against.
func settingsRows(cfg config.Config) []settingsRow {
	rows := []settingsRow{
		{kind: rowTheme, label: "theme", value: cfg.Theme},
		{kind: rowAnimations, label: "animations", value: boolStr(cfg.Animations)},
		{kind: rowHideSystemProcesses, label: "display.hide_system_processes", value: boolStr(cfg.Display.HideSystemProcesses)},
		{kind: rowHidePrivilegedPorts, label: "display.hide_privileged_ports", value: boolStr(cfg.Display.HidePrivilegedPorts)},
		{kind: rowDevPortRange, label: "kill.dev_port_range", value: nonEmpty(cfg.Kill.DevPortRange, "-")},
		{kind: rowEscalationTimeout, label: "kill.escalation_timeout", value: cfg.Kill.EscalationTimeout.String()},
		{kind: rowWatchInterval, label: "watch.interval", value: cfg.Watch.Interval.String()},
		{kind: rowAutoKillEnabled, label: "auto_kill.enabled", value: boolStr(cfg.AutoKill.Enabled)},
		{kind: rowAutoKillInterval, label: "auto_kill.interval", value: cfg.AutoKill.Interval.String()},
	}

	for i, e := range cfg.AutoKill.Allow {
		rows = append(rows, settingsRow{kind: rowAutoKillEntry, label: fmt.Sprintf("  auto_kill.allow[%d]", i), value: fmt.Sprintf(":%d %s", e.Port, e.Process), index: i})
	}
	rows = append(rows, settingsRow{kind: rowAddAutoKillEntry, label: "  + add allow-list entry"})

	for i, p := range cfg.Protected {
		value := fmt.Sprintf(":%d", p.Port)
		if p.Reason != "" {
			value += " — " + p.Reason
		}
		rows = append(rows, settingsRow{kind: rowProtectedEntry, label: fmt.Sprintf("  protected[%d]", i), value: value, index: i})
	}
	rows = append(rows, settingsRow{kind: rowAddProtectedEntry, label: "  + add protected port"})

	return rows
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// nextTheme cycles auto -> color -> plain -> auto.
func nextTheme(cur string) string {
	switch cur {
	case "auto":
		return "color"
	case "color":
		return "plain"
	default:
		return "auto"
	}
}

// settingsView renders the editable settings screen: every row from
// settingsRows, the cursor, a dirty (unsaved changes) indicator, and any
// pending validation error.
func (m Model) settingsView() string {
	rows := settingsRows(m.settingsCfg)

	var b strings.Builder
	title := "porthole — settings (~/.porthole.yaml)"
	if m.settingsDirty {
		title += " *"
	}
	b.WriteString(m.th.Header.Render(title))
	b.WriteString("\n\n")

	for i, r := range rows {
		cursor := "  "
		if i == m.settingsCursor {
			cursor = "▸ "
		}
		line := cursor + padColumns(r.label, 34) + r.value
		if i == m.settingsCursor {
			b.WriteString(m.th.Selected.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	if m.settingsErr != "" {
		b.WriteString("\n")
		b.WriteString(m.th.Danger.Render("error: " + m.settingsErr))
		b.WriteString("\n")
	}

	if m.mode == modeSettingsEdit {
		b.WriteString("\n")
		b.WriteString(m.confirmInput.View())
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if m.mode == modeSettingsEdit {
		b.WriteString(m.th.HintBar.Render("enter confirm · esc cancel"))
	} else {
		b.WriteString(m.th.HintBar.Render("↑↓ nav · enter toggle/edit · d delete entry · s save · esc/q close (discards unsaved changes)"))
	}
	return b.String()
}
