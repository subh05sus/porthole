package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/subh05sus/porthole/internal/kill"
	"github.com/subh05sus/porthole/internal/scan"
)

func newKillCmd(app *App) *cobra.Command {
	var (
		force   bool
		dryRun  bool
		yes     bool
		project string
		dev     bool
	)

	cmd := &cobra.Command{
		Use:   "kill [ports...]",
		Short: "Kill the process listening on one or more ports",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runKill(app, args, force, dryRun, yes, dev, project)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "send SIGKILL immediately instead of escalating")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be killed, do nothing")
	cmd.Flags().BoolVar(&yes, "yes", false, "skip the confirmation prompt")
	cmd.Flags().StringVar(&project, "project", "", "kill everything owned by this project")
	cmd.Flags().BoolVar(&dev, "dev", false, "kill everything you own in the common dev port range (3000-9999)")

	return cmd
}

func runKill(app *App, args []string, force, dryRun, yes, dev bool, project string) error {
	if dev {
		if len(args) > 0 || project != "" {
			return exitErr(ExitNotFound, errors.New("--dev cannot be combined with explicit ports or --project"))
		}
		args = []string{"3000-9999"}
	}
	if project != "" && len(args) > 0 {
		return exitErr(ExitNotFound, errors.New("specify either ports or --project, not both"))
	}
	if project == "" && len(args) == 0 {
		return exitErr(ExitNotFound, errors.New("specify at least one port, a range, or --project"))
	}

	ctx := context.Background()
	services, err := app.Lister.List(ctx)
	if err != nil {
		return err
	}

	targets, notFound, err := resolveTargets(services, args, project)
	if err != nil {
		return exitErr(ExitNotFound, err)
	}

	if dev {
		// --dev is a broad best-effort sweep, not a precise target list:
		// most of a 3000-9999 scan will be empty ports, and most of what's
		// listening won't be owned by the current user (system services).
		// Reporting "nothing found on port 4523" ~6000 times and demanding
		// elevated-permission errors for every locked row would drown out
		// anything useful, so both are silently dropped here instead.
		notFound = nil
		owned := targets[:0]
		for _, t := range targets {
			if t.Owned {
				owned = append(owned, t)
			}
		}
		targets = owned
		if len(targets) == 0 {
			fmt.Fprintln(app.Stdout, "nothing owned to kill in 3000-9999")
			return nil
		}
	} else {
		for _, p := range notFound {
			fmt.Fprintf(app.Stderr, "nothing found on port %d\n", p)
		}
	}
	if len(targets) == 0 {
		return exitErr(ExitNotFound, errors.New("nothing found to kill"))
	}

	if dryRun {
		for _, s := range targets {
			fmt.Fprintf(app.Stdout, "would kill %s (pid %d) on :%d\n", s.Process, s.PID, s.Port)
		}
		return nil
	}

	worstCode := ExitSuccess
	if len(notFound) > 0 {
		worstCode = ExitNotFound
	}

	reader := bufio.NewReader(app.Stdin)
	for _, s := range targets {
		if protected, reason := app.Config.IsProtected(s.Port); protected {
			// Protected ports always require typed confirmation — --yes
			// does not bypass this, per v1.3's "no shortcuts for protected
			// ports" design: a script that blindly passes --yes should not
			// be able to silently take down something the user explicitly
			// marked as protected.
			prompt := fmt.Sprintf("port :%d is protected", s.Port)
			if reason != "" {
				prompt += " (" + reason + ")"
			}
			fmt.Fprintf(app.Stdout, "%s. Type %d to confirm killing %s: ", prompt, s.Port, s.Process)
			line, _ := reader.ReadString('\n')
			if strings.TrimSpace(line) != strconv.Itoa(s.Port) {
				fmt.Fprintf(app.Stdout, "skipped :%d (confirmation did not match)\n", s.Port)
				continue
			}
		} else if !yes {
			fmt.Fprintf(app.Stdout, "kill %s on :%d? (y/n) ", s.Process, s.Port)
			line, _ := reader.ReadString('\n')
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y") {
				fmt.Fprintf(app.Stdout, "skipped :%d\n", s.Port)
				continue
			}
		}

		target := kill.Target{PID: s.PID, StartTime: s.StartTime, Owned: s.Owned}
		res, killErr := app.Killer.Execute(ctx, target, kill.Options{Force: force, AutoEscalate: true})
		code, msg := classifyKillResult(s, res, killErr)
		if code == ExitSuccess {
			fmt.Fprintln(app.Stdout, msg)
		} else {
			fmt.Fprintln(app.Stderr, msg)
			if code > worstCode {
				worstCode = code
			}
		}
	}

	if worstCode != ExitSuccess {
		return exitErr(worstCode, errors.New("one or more kills did not succeed"))
	}
	return nil
}

// resolveTargets turns CLI args (or --project) into the concrete services to
// act on, matched against a fresh scan. Ports with no matching service are
// returned separately rather than silently dropped.
func resolveTargets(services []scan.Service, args []string, project string) (targets []scan.Service, notFound []int, err error) {
	if project != "" {
		for _, s := range services {
			if strings.EqualFold(s.Project, project) {
				targets = append(targets, s)
			}
		}
		return targets, nil, nil
	}

	ports, err := parsePorts(args)
	if err != nil {
		return nil, nil, err
	}

	byPort := make(map[int]scan.Service, len(services))
	for _, s := range services {
		byPort[s.Port] = s
	}
	for _, p := range ports {
		if s, ok := byPort[p]; ok {
			targets = append(targets, s)
		} else {
			notFound = append(notFound, p)
		}
	}
	return targets, notFound, nil
}

// parsePorts expands a mix of single ports ("3000") and inclusive ranges
// ("3000-3010") into a deduplicated, order-preserving port list.
func parsePorts(args []string) ([]int, error) {
	var ports []int
	seen := make(map[int]bool)
	add := func(p int) {
		if !seen[p] {
			seen[p] = true
			ports = append(ports, p)
		}
	}

	for _, arg := range args {
		if lo, hi, ok := strings.Cut(arg, "-"); ok {
			loN, errLo := strconv.Atoi(strings.TrimSpace(lo))
			hiN, errHi := strconv.Atoi(strings.TrimSpace(hi))
			if errLo == nil && errHi == nil {
				if loN > hiN {
					return nil, fmt.Errorf("invalid port range %q: start greater than end", arg)
				}
				for p := loN; p <= hiN; p++ {
					add(p)
				}
				continue
			}
		}
		p, err := strconv.Atoi(strings.TrimSpace(arg))
		if err != nil {
			return nil, fmt.Errorf("invalid port %q", arg)
		}
		add(p)
	}
	return ports, nil
}

func classifyKillResult(s scan.Service, res kill.Result, err error) (code int, msg string) {
	if err != nil {
		switch {
		case errors.Is(err, kill.ErrPIDReused):
			return ExitPIDChanged, fmt.Sprintf("port :%d: process changed since scan, refreshed", s.Port)
		case errors.Is(err, kill.ErrNotOwned):
			return ExitPermissionDenied, fmt.Sprintf("port :%d: needs elevated permissions, try: sudo porthole", s.Port)
		default:
			return ExitKillFailed, fmt.Sprintf("port :%d: %v", s.Port, err)
		}
	}
	switch res.Status {
	case kill.StatusKilled, kill.StatusAlreadyDead:
		return ExitSuccess, fmt.Sprintf("terminated %s on :%d", s.Process, s.Port)
	default:
		return ExitKillFailed, fmt.Sprintf("port :%d: process ignored kill signal", s.Port)
	}
}
