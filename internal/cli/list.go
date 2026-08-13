package cli

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/subh05sus/porthole/internal/output"
	"github.com/subh05sus/porthole/internal/scan"
)

func newListCmd(app *App) *cobra.Command {
	var (
		asJSON        bool
		portFilter    int
		projectFilter string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List currently listening services",
		RunE: func(cmd *cobra.Command, args []string) error {
			services, err := app.Lister.List(context.Background())
			if err != nil {
				return err
			}

			services = filterServices(services, portFilter, projectFilter)

			if asJSON {
				return output.JSON(app.Stdout, services)
			}
			return output.Table(app.Stdout, services)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	cmd.Flags().IntVar(&portFilter, "port", 0, "filter to a single port")
	cmd.Flags().StringVar(&projectFilter, "project", "", "filter by detected project name")

	return cmd
}

func filterServices(services []scan.Service, port int, project string) []scan.Service {
	if port == 0 && project == "" {
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
		out = append(out, s)
	}
	return out
}
