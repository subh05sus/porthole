// Command porthole is a port kill switch and service viewer for your terminal.
package main

import (
	"fmt"
	"os"

	"github.com/subh05sus/porthole/internal/cli"
	"github.com/subh05sus/porthole/internal/config"
	"github.com/subh05sus/porthole/internal/history"
	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/proc"
	"github.com/subh05sus/porthole/internal/restart"
	"github.com/subh05sus/porthole/internal/scan"
)

func main() {
	// A present-but-invalid ~/.porthole.yaml fails loudly before anything
	// else runs, rather than silently falling back to defaults — a broken
	// protected-ports config could otherwise give false confidence that a
	// port is protected when porthole actually never parsed it.
	cfg, err := config.LoadDefault()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// A failure to resolve the history path just disables history logging
	// (HistoryPath stays "") rather than blocking startup — it's a
	// best-effort convenience feature, not a critical one.
	historyPath, _ := history.DefaultPath()
	killer := &history.LoggingKiller{Inner: kill.NewDefaultKiller(), Path: historyPath}

	app := &cli.App{
		Lister:      scan.NewDefaultLister(),
		Killer:      killer,
		Lookup:      proc.NewDefaultLookup(),
		Spawner:     restart.NewDefaultSpawner(),
		Config:      cfg,
		HistoryPath: historyPath,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	}
	os.Exit(cli.Execute(app, os.Args[1:]))
}
