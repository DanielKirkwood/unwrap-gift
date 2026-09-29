package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/tasks"
)

var (
	errDatabaseNotConfigured           = errors.New("cmd: database feature is not enabled")
	errKetoSeedAdminIdentityIDRequired = errors.New(
		"cmd: KETO_SEED_ADMIN_IDENTITY_ID must be set to seed Keto relation tuples",
	)
)

// NewDBCmd builds the `db` command group: migration and seed management
// against the bootstrapped app's Store.
func NewDBCmd() *cobra.Command {
	dbCmd := &cobra.Command{
		Use:   "db",
		Short: "Manage the database: migrations and seed data",
		Long: `db manages the SQLite schema (via goose migrations) and seed data:

  create <name>   scaffold a new empty migration file
  migrate         apply every pending migration
  rollback        revert the most recently applied migration
  status          show which migrations are applied vs pending
  seed            seed development data (currently: Keto admin relation tuples)

migrate/rollback/status/seed all require the database feature to be enabled
and the app to bootstrap successfully; create only writes a file to disk and
needs neither.`,
	}

	dbCmd.AddCommand(newDBCreateCmd())
	dbCmd.AddCommand(newDBMigrateCmd())
	dbCmd.AddCommand(newDBRollbackCmd())
	dbCmd.AddCommand(newDBStatusCmd())
	dbCmd.AddCommand(newDBSeedCmd())

	return dbCmd
}

func newDBCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create [name]",
		Short: "Scaffold a new empty migration file",
		Long: `Writes a new, empty timestamped SQL migration file for [name] into
the migrations directory, using goose's file naming convention. This only
writes a file — it doesn't touch the database or require it to be configured.`,
		Example: "  unwrap-gift db create add_users_table",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return db.Create(args[0])
		},
	}
}

func newDBMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "migrate",
		Short:   "Apply every pending migration",
		Example: "  unwrap-gift db migrate",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := storeFromContext(cmd)
			if err != nil {
				return err
			}

			return db.Migrate(cmd.Context(), store.DB)
		},
	}
}

func newDBRollbackCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "rollback",
		Short:   "Revert the most recently applied migration",
		Long:    `Reverts exactly one migration — the most recently applied one — not all of them.`,
		Example: "  unwrap-gift db rollback",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := storeFromContext(cmd)
			if err != nil {
				return err
			}

			return db.Rollback(cmd.Context(), store.DB)
		},
	}
}

func newDBStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "status",
		Short:   "Show which migrations are applied vs pending",
		Example: "  unwrap-gift db status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := storeFromContext(cmd)
			if err != nil {
				return err
			}

			return db.Status(cmd.Context(), store.DB)
		},
	}
}

func newDBSeedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "seed",
		Short: "Seed the database with development data",
		Long: `Seeds development data. Currently this only seeds Keto relation
tuples: it grants the Role:admin relation to the identity named by the
KETO_SEED_ADMIN_IDENTITY_ID environment variable, and is a no-op if the keto
feature isn't enabled. Requires KETO_SEED_ADMIN_IDENTITY_ID to be set when
Keto is enabled, or the command errors.`,
		Example: "  unwrap-gift db seed",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := storeFromContext(cmd); err != nil {
				return err
			}

			// SQLite domain data is introduced alongside the first real
			// domain table in Phase 7 — nothing to seed there yet. Keto
			// relation-tuple seeding lands in Phase 6, so it's the only
			// seed data today.
			if err := seedKeto(cmd); err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), "db seed: seeded Keto relation tuples")

			return nil
		},
	}
}

// seedKeto grants the Role:admin relation tuple to
// EnvVars.KetoSeedAdminIdentityID and makes Identities:admin's manage
// permission traverse that role, matching deploy/keto/identities.ts's OPL.
// It's a no-op when the keto feature is disabled. The actual seeding is
// tasks.SeedKetoAdmin, shared with `task exec reseed-keto` so the two entry
// points can't drift out of sync.
func seedKeto(cmd *cobra.Command) error {
	a, ok := appFromContext(cmd.Context())
	if !ok {
		return errAppNotBootstrapped
	}
	if a.Keto == nil {
		return nil
	}
	if a.Env.KetoSeedAdminIdentityID == "" {
		return errKetoSeedAdminIdentityIDRequired
	}

	if err := tasks.SeedKetoAdmin(cmd.Context(), a.Keto, a.Env.KetoSeedAdminIdentityID); err != nil {
		return fmt.Errorf("cmd: seed keto: %w", err)
	}

	return nil
}

// storeFromContext returns the bootstrapped app's *db.Store, erroring if the
// app isn't bootstrapped or the database feature is disabled.
func storeFromContext(cmd *cobra.Command) (*db.Store, error) {
	a, ok := appFromContext(cmd.Context())
	if !ok {
		return nil, errAppNotBootstrapped
	}
	if a.Store == nil {
		return nil, errDatabaseNotConfigured
	}

	return a.Store, nil
}
