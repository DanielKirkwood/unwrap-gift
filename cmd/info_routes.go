package cmd

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

func newInfoPublicRoutesCmd() *cobra.Command {
	c := newInfoRoutesCmd("public-api-routes", "Print the public router's registered routes", api.NewPublicRouter)
	c.Long = `Builds the public router the same way it's built at server start and
prints its METHOD/ROUTE table. Useful for confirming which unauthenticated
endpoints are actually mounted without spinning up a listener.`
	c.Example = "  unwrap-gift info public-api-routes"

	return c
}

func newInfoProtectedRoutesCmd() *cobra.Command {
	c := newInfoRoutesCmd(
		"protected-api-routes", "Print the protected router's registered routes", api.NewProtectedRouter,
	)
	c.Long = `Builds the protected router the same way it's built at server start
and prints its METHOD/ROUTE table. These routes require an authenticated
Kratos session; this command only lists them, it doesn't call them.`
	c.Example = "  unwrap-gift info protected-api-routes"

	return c
}

func newInfoHiddenRoutesCmd() *cobra.Command {
	c := newInfoRoutesCmd("hidden-api-routes", "Print the hidden router's registered routes", api.NewHiddenRouter)
	c.Long = `Builds the hidden router the same way it's built at server start and
prints its METHOD/ROUTE table. The hidden router serves admin-only endpoints
meant to be bound to an interface not exposed to the public internet; this
command only lists its routes, it doesn't call them.`
	c.Example = "  unwrap-gift info hidden-api-routes"

	return c
}

func newInfoRoutesCmd(use, short string, build func(api.RouterDeps) *chi.Mux) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			a, ok := appFromContext(cmd.Context())
			if !ok {
				return errAppNotBootstrapped
			}

			router := build(api.RouterDeps{Logger: a.Logger, TracerProvider: a.Otel.TracerProvider})

			return printRoutes(cmd, router)
		},
	}
}

func printRoutes(cmd *cobra.Command, router chi.Router) error {
	w := newTabWriter(cmd)

	fmt.Fprintln(w, "METHOD\tROUTE")

	walkErr := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		fmt.Fprintf(w, "%s\t%s\n", method, route)
		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	return w.Flush()
}
