package cli

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/subh05sus/porthole/internal/output"
	"github.com/subh05sus/porthole/internal/scan"
)

func newListCmd(app *App) *cobra.Command {
	var (
		asJSON         bool
		oneline        bool
		portFilter     int
		projectFilter  string
		since          time.Duration
		containersOnly bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List currently listening services",
		RunE: func(cmd *cobra.Command, args []string) error {
			services, err := app.Lister.List(context.Background())
			if err != nil {
				return err
			}

			services = filterServices(services, portFilter, projectFilter, since, containersOnly)

			switch {
			case asJSON:
				return output.JSON(app.Stdout, services)
			case oneline:
				return output.OneLine(app.Stdout, services)
			default:
				return output.Table(app.Stdout, services)
			}
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	cmd.Flags().BoolVar(&oneline, "oneline", false, "compact single-line summary, for status bars (tmux/zellij)")
	cmd.Flags().IntVar(&portFilter, "port", 0, "filter to a single port")
	cmd.Flags().StringVar(&projectFilter, "project", "", "filter by detected project name")
	cmd.Flags().DurationVar(&since, "since", 0, "only show services started within this long ago (e.g. 5m)")
	cmd.Flags().BoolVar(&containersOnly, "containers", false, "only show services published by a running container")

	return cmd
}

func filterServices(services []scan.Service, port int, project string, since time.Duration, containersOnly bool) []scan.Service {
	if port == 0 && project == "" && since == 0 && !containersOnly {
		return services
	}
	out := make([]scan.Service, 0, len(services))
	for _, s := range services {
		if port != 0 && s.Port != port {
			continue
		}
		if project != "" && !strings.EqualFold(s.Project, project) {
			continue
		}
		if since != 0 && s.Uptime > since {
			continue
		}
		if containersOnly && s.ContainerID == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}
