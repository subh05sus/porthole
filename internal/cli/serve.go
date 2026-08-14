package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/subh05sus/porthole/internal/webui"
)

const shutdownTimeout = 2 * time.Second

func newServeCmd(app *App) *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start a local web dashboard (127.0.0.1 only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(app, port)
		},
	}
	cmd.Flags().IntVar(&port, "port", 9191, "port to listen on (127.0.0.1 only — never exposed to the network)")
	return cmd
}

// runServe wires the real OS-signal-derived shutdown context and hands off
// to serveUntilDone, which does the actual bind/serve/shutdown and is
// unit-testable with an injectable context instead of real OS signals.
func runServe(app *App, port int) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return serveUntilDone(ctx, app, port, nil)
}

// serveUntilDone binds 127.0.0.1 explicitly, never 0.0.0.0/all-interfaces
// — FUTURE_PLANS.md's own parked-item note is blunt about why: a kill-
// switch-capable dashboard exposed to a LAN is the wrong default even
// though the feature itself was asked for. There is no flag to change
// this; opening it up would need a deliberate code change, not a
// typo-able flag.
//
// ready, if non-nil, receives the bound address once listening starts —
// used by tests to learn the real ephemeral port when port is 0, and to
// know serving has actually begun before firing requests at it.
func serveUntilDone(ctx context.Context, app *App, port int, ready chan<- string) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return exitErr(1, fmt.Errorf("serve: %w", err))
	}

	handler := webui.NewHandler(app.Lister, app.Killer)
	srv := &http.Server{Handler: handler}

	fmt.Fprintf(app.Stdout, "porthole dashboard: http://%s (Ctrl+C to stop)\n", ln.Addr())
	if ready != nil {
		ready <- ln.Addr().String()
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return exitErr(1, fmt.Errorf("serve: %w", err))
		}
		return nil
	}
}
