// Command porthole is a port kill switch and service viewer for your terminal.
package main

import (
	"os"

	"github.com/subh05sus/porthole/internal/cli"
	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/proc"
	"github.com/subh05sus/porthole/internal/restart"
	"github.com/subh05sus/porthole/internal/scan"
)

func main() {
	app := &cli.App{
		Lister:  scan.NewDefaultLister(),
		Killer:  kill.NewDefaultKiller(),
		Lookup:  proc.NewDefaultLookup(),
		Spawner: restart.NewDefaultSpawner(),
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	}
	os.Exit(cli.Execute(app, os.Args[1:]))
}
