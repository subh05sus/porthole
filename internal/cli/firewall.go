package cli

import (
	"bufio"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/subh05sus/porthole/internal/firewall"
)

func newFirewallCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "firewall",
		Short: "Manage porthole-owned firewall rules (an isolated group porthole never mixes with your existing rules)",
	}
	root.AddCommand(newFirewallBlockCmd(app, firewall.ActionBlock))
	root.AddCommand(newFirewallBlockCmd(app, firewall.ActionAllow))
	root.AddCommand(newFirewallListCmd(app))
	root.AddCommand(newFirewallCleanCmd(app))
	return root
}

// newFirewallBlockCmd builds both "block" and "allow" — identical shape,
// opposite Action, so there is exactly one place implementing the
// preview/confirm/apply flow rather than two copies to keep in sync.
func newFirewallBlockCmd(app *App, action firewall.Action) *cobra.Command {
	var (
		proto string
		out   bool
	)

	cmd := &cobra.Command{
		Use:   string(action) + " <port>",
		Short: fmt.Sprintf("%s a port via an isolated, porthole-owned firewall rule", strings.ToUpper(string(action)[:1])+string(action)[1:]),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFirewallApply(app, args[0], action, proto, out)
		},
	}
	// Deliberately no --yes flag anywhere in this command tree: firewall
	// changes always require the typed-port confirmation below, even in
	// scripts, matching v1.3's "no shortcuts for protected ports" design
	// taken one step further — there is no bypass to design an exception
	// for in the first place.
	cmd.Flags().StringVar(&proto, "proto", "tcp", `protocol: "tcp" or "udp"`)
	cmd.Flags().BoolVar(&out, "out", false, "apply to outbound traffic instead of inbound")
	return cmd
}

func runFirewallApply(app *App, portArg string, action firewall.Action, proto string, out bool) error {
	port, err := strconv.Atoi(portArg)
	if err != nil || port < 1 || port > 65535 {
		return exitErr(ExitNotFound, fmt.Errorf("invalid port %q", portArg))
	}
	proto = strings.ToLower(proto)
	if proto != "tcp" && proto != "udp" {
		return exitErr(ExitNotFound, fmt.Errorf("invalid --proto %q: must be tcp or udp", proto))
	}

	direction := firewall.DirectionIn
	dirWord := "inbound"
	if out {
		direction = firewall.DirectionOut
		dirWord = "outbound"
	}
	rule := firewall.Rule{Port: port, Proto: proto, Action: action, Direction: direction}

	fmt.Fprintf(app.Stdout, "this will add an isolated firewall rule: %s %s %s port %d (rule name: %s)\n",
		action, dirWord, strings.ToUpper(proto), port, rule.Name())
	fmt.Fprintf(app.Stdout, "type %d to confirm: ", port)

	reader := bufio.NewReader(app.Stdin)
	line, _ := reader.ReadString('\n')
	if strings.TrimSpace(line) != strconv.Itoa(port) {
		fmt.Fprintln(app.Stdout, "cancelled (confirmation did not match)")
		return nil
	}

	if err := app.Firewall.Apply(context.Background(), rule); err != nil {
		return exitErr(1, err)
	}
	fmt.Fprintf(app.Stdout, "applied: %s\n", rule.Name())
	return nil
}

func newFirewallListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List porthole-owned firewall rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			rules, err := app.Firewall.List(context.Background())
			if err != nil {
				return exitErr(1, err)
			}
			if len(rules) == 0 {
				fmt.Fprintln(app.Stdout, "no porthole-owned firewall rules")
				return nil
			}
			for _, r := range rules {
				fmt.Fprintf(app.Stdout, "%-6s %-9s %-4s %d\n", r.Direction, r.Action, r.Proto, r.Port)
			}
			return nil
		},
	}
}

func newFirewallCleanCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Remove every porthole-owned firewall rule",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFirewallClean(app)
		},
	}
}

func runFirewallClean(app *App) error {
	ctx := context.Background()
	rules, err := app.Firewall.List(ctx)
	if err != nil {
		return exitErr(1, err)
	}
	if len(rules) == 0 {
		fmt.Fprintln(app.Stdout, "no porthole-owned firewall rules to remove")
		return nil
	}

	fmt.Fprintf(app.Stdout, "this will remove %d porthole-owned firewall rule(s):\n", len(rules))
	for _, r := range rules {
		fmt.Fprintf(app.Stdout, "  %-6s %-9s %-4s %d\n", r.Direction, r.Action, r.Proto, r.Port)
	}
	fmt.Fprint(app.Stdout, "type CLEAN to confirm: ")

	reader := bufio.NewReader(app.Stdin)
	line, _ := reader.ReadString('\n')
	if strings.TrimSpace(line) != "CLEAN" {
		fmt.Fprintln(app.Stdout, "cancelled (confirmation did not match)")
		return nil
	}

	if err := app.Firewall.RemoveAll(ctx); err != nil {
		return exitErr(1, err)
	}
	fmt.Fprintf(app.Stdout, "removed %d rule(s)\n", len(rules))
	return nil
}
