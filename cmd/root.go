package cmd

import (
	"os"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	"github.com/DanielKirkwood/unwrap-gift/internal/app"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

// Execute builds the root command and runs it. It is called by main.main().
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// NewRootCmd builds the CLI's root command and its full subcommand tree.
// Flags are bound to local variables captured by PersistentPreRunE's
// closure rather than package-level state.
func NewRootCmd() *cobra.Command {
	var (
		devMode  bool
		envMode  bool
		logLevel string
	)

	name := moduleName()

	rootCmd := &cobra.Command{
		Use:   name,
		Short: name + " — boilerplate Go API: HTTP servers, database migrations, and operational tasks",
		Long: `
#    # #    # #    # #####    ##   #####      ####  # ###### #####
#    # ##   # #    # #    #  #  #  #    #    #    # # #        #
#    # # #  # #    # #    # #    # #    #    #      # #####    #
#    # #  # # # ## # #####  ###### #####     #  ### # #        #
#    # #   ## ##  ## #   #  #    # #         #    # # #        #
 ####  #    # #    # #    # #    # #          ####  # #        #
		` + name + ` cli

` + name + ` serves three HTTP APIs (public, protected, and hidden — see
"` + name + ` info *-api-routes"), backed by SQLite, Ory Kratos authentication,
and Ory Keto authorization.

Common subcommands:
  start   run the API servers
  db      manage database migrations and seed data
  task    run ad-hoc operational tasks (grant/revoke admin, etc.)
  info    inspect the resolved config, feature flags, and routes

Use --dev to force development mode, --env to print the resolved environment
before running, and --log to override the configured log level.`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			env, err := config.Load()
			if err != nil {
				return err
			}

			if devMode {
				env.Env = "development"
			}
			if logLevel != "" {
				env.LogLevel = logLevel
			}
			if envMode {
				return printEnv(cmd, env)
			}

			a, err := app.Bootstrap(cmd.Context(), env)
			if err != nil {
				return err
			}
			cmd.SetContext(withApp(cmd.Context(), a))

			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, _ []string) error {
			a, ok := appFromContext(cmd.Context())
			if !ok {
				return nil
			}

			return a.Shutdown(cmd.Context())
		},
	}

	rootCmd.PersistentFlags().BoolVarP(&devMode, "dev", "d", false, "Run in development mode")
	rootCmd.PersistentFlags().BoolVarP(&envMode, "env", "e", false, "Print environment before execution")
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log", "l", "", "Log level")

	rootCmd.AddCommand(NewInfoCmd())
	rootCmd.AddCommand(NewStartCmd())
	rootCmd.AddCommand(NewDBCmd())
	rootCmd.AddCommand(NewTaskCmd())

	return rootCmd
}

// moduleName derives the CLI's display name from the running binary's build
// info, falling back to a fixed name if build info isn't available.
func moduleName() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unwrap-gift"
	}

	parts := strings.Split(info.Main.Path, "/")

	return parts[len(parts)-1]
}
