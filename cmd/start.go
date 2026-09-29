package cmd

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/DanielKirkwood/unwrap-gift/internal/app"
)

// NewStartCmd builds the `start` command group: the combined `start`
// (all three servers) plus one subcommand per router for independent
// deployment/scaling.
func NewStartCmd() *cobra.Command {
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the public, protected, and hidden API servers together",
		Long: `Starts the public, protected, and hidden API servers in the same
process and blocks until an interrupt/TERM signal arrives, then shuts down
gracefully. Use the publicApi/protectedApi/hiddenApi subcommands instead to
run and scale one router independently (e.g. as separate deployments).`,
		Example: "  unwrap-gift start",
		RunE:    runStart(selectAll),
	}

	startCmd.AddCommand(&cobra.Command{
		Use:   "publicApi",
		Short: "Start only the public API server",
		Long: `Starts just the public router's server and blocks until an
interrupt/TERM signal arrives, then shuts down gracefully. Use this to deploy
or scale the public API independently of the protected and hidden ones.`,
		Example: "  unwrap-gift start publicApi",
		RunE:    runStart(selectPublic),
	})
	startCmd.AddCommand(&cobra.Command{
		Use:   "protectedApi",
		Short: "Start only the protected API server",
		Long: `Starts just the protected router's server and blocks until an
interrupt/TERM signal arrives, then shuts down gracefully. Use this to deploy
or scale the protected API independently of the public and hidden ones.`,
		Example: "  unwrap-gift start protectedApi",
		RunE:    runStart(selectProtected),
	})
	startCmd.AddCommand(&cobra.Command{
		Use:   "hiddenApi",
		Short: "Start only the hidden API server",
		Long: `Starts just the hidden router's server and blocks until an
interrupt/TERM signal arrives, then shuts down gracefully. Use this to deploy
the admin-only hidden API on its own, non-public-facing interface, independent
of the public and protected servers.`,
		Example: "  unwrap-gift start hiddenApi",
		RunE:    runStart(selectHidden),
	})

	return startCmd
}

// selectFunc picks which of the three built servers a start variant should
// actually run. All three are always built (BuildServers is cheap — it
// constructs [http.Server] values, it doesn't bind sockets), so
// App.Shutdown can uniformly shut down a.Servers regardless of which subset
// was started: Shutdown on a server that was never started is a no-op.
type selectFunc func(*app.Servers) []*http.Server

func selectAll(s *app.Servers) []*http.Server       { return []*http.Server{s.Public, s.Protected, s.Hidden} }
func selectPublic(s *app.Servers) []*http.Server    { return []*http.Server{s.Public} }
func selectProtected(s *app.Servers) []*http.Server { return []*http.Server{s.Protected} }
func selectHidden(s *app.Servers) []*http.Server    { return []*http.Server{s.Hidden} }

func runStart(sel selectFunc) func(cmd *cobra.Command, _ []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		a, ok := appFromContext(cmd.Context())
		if !ok {
			return errAppNotBootstrapped
		}

		servers, err := app.BuildServers(a)
		if err != nil {
			return err
		}
		a.Servers = servers

		return serveUntilSignalOrError(cmd.Context(), a, sel(servers))
	}
}

// serveUntilSignalOrError starts each server in toStart and blocks until
// either an interrupt/TERM signal arrives or one of them fails to serve.
// It deliberately does not shut anything down itself: PersistentPostRunE
// calls a.Shutdown on the *original* (non-cancelled) command context once
// this returns, so graceful shutdown always runs exactly once, with a fresh
// context rather than the already-cancelled signal one.
func serveUntilSignalOrError(ctx context.Context, a *app.App, toStart []*http.Server) error {
	notifyCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, len(toStart))
	for _, srv := range toStart {
		go func(srv *http.Server) {
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
				return
			}
			errCh <- nil
		}(srv)
	}

	a.Logger.InfoContext(ctx, "start: serving", "servers", len(toStart))

	select {
	case <-notifyCtx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}
