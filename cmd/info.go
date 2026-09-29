package cmd

import (
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

const redacted = "<redacted>"

var errAppNotBootstrapped = errors.New("cmd: app not bootstrapped on context")

const (
	tabWriterMinWidth = 0
	tabWriterTabWidth = 4
	tabWriterPadding  = 2
	tabWriterPadChar  = ' '
)

// NewInfoCmd builds the `info` command group: read-only diagnostics over the
// resolved configuration.
func NewInfoCmd() *cobra.Command {
	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "Print diagnostic information about the resolved configuration",
		Long: `info groups read-only introspection commands for answering "what
does this build do right now?" without reading code:

  env                    print resolved environment variables
  features               print each feature flag's enabled/disabled state
  public-api-routes      print the public router's registered routes
  protected-api-routes   print the protected router's registered routes
  hidden-api-routes      print the hidden router's registered routes

All of these bootstrap the app the same way any other command does, so a
broken configuration will surface here too.`,
	}

	infoCmd.AddCommand(newInfoEnvCmd())
	infoCmd.AddCommand(newInfoFeaturesCmd())
	infoCmd.AddCommand(newInfoPublicRoutesCmd())
	infoCmd.AddCommand(newInfoProtectedRoutesCmd())
	infoCmd.AddCommand(newInfoHiddenRoutesCmd())

	return infoCmd
}

func newInfoEnvCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "env",
		Short: "Print resolved environment variables",
		Long: `Prints every resolved environment variable as a NAME/VALUE table.
Fields marked secret (e.g. API keys, credentials) are printed as <redacted>
rather than their real value.`,
		Example: "  unwrap-gift info env",
		RunE: func(cmd *cobra.Command, _ []string) error {
			a, ok := appFromContext(cmd.Context())
			if !ok {
				return errAppNotBootstrapped
			}

			return printEnv(cmd, a.Env)
		},
	}
}

func newInfoFeaturesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "features",
		Short: "Print each feature's enabled/disabled state",
		Long: `Prints a NAME/ENABLED/REASON table for every registered feature
flag, then validates that every enabled feature's resolved configuration is
actually usable (e.g. required env vars are set), returning an error if any
enabled feature fails that check.`,
		Example: "  unwrap-gift info features",
		RunE: func(cmd *cobra.Command, _ []string) error {
			a, ok := appFromContext(cmd.Context())
			if !ok {
				return errAppNotBootstrapped
			}

			return printFeatures(cmd, a.Registry)
		},
	}
}

func printEnv(cmd *cobra.Command, env config.EnvVars) error {
	w := newTabWriter(cmd)

	fmt.Fprintln(w, "ENV VAR\tVALUE")
	for _, f := range env.Fields() {
		value := f.Value
		if f.Secret && value != "" {
			value = redacted
		}
		fmt.Fprintf(w, "%s\t%s\n", f.Env, value)
	}

	return w.Flush()
}

func printFeatures(cmd *cobra.Command, registry *config.Registry) error {
	w := newTabWriter(cmd)

	fmt.Fprintln(w, "FEATURE\tENABLED\tREASON")
	for _, f := range registry.All() {
		fmt.Fprintf(w, "%s\t%t\t%s\n", f.Name, f.Enabled, f.Reason)
	}

	if err := w.Flush(); err != nil {
		return err
	}

	return registry.ValidateReadiness()
}

func newTabWriter(cmd *cobra.Command) *tabwriter.Writer {
	return tabwriter.NewWriter(
		cmd.OutOrStdout(), tabWriterMinWidth, tabWriterTabWidth, tabWriterPadding, tabWriterPadChar, 0,
	)
}
