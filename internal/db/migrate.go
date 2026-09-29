package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sync"

	"github.com/pressly/goose/v3"
)

// migrationsDirOnDisk is where `db create` writes new migration files — a
// real filesystem path, since migrationsFS below is compiled in and
// read-only at runtime.
const migrationsDirOnDisk = "internal/db/migrations"

// migrationsFSDir is the directory name goose looks up inside migrationsFS.
const migrationsFSDir = "migrations"

//go:embed migrations/*.sql
var migrationsFS embed.FS

// configureGooseOnce guards the one-time call into goose's package-level
// SetBaseFS/SetDialect setters. Those setters mutate goose's own unsynchronized
// global state, so calling configureGoose from concurrent tests (internal/db's
// t.Parallel() tests all call Migrate/Status/Rollback) races unless the actual
// mutation happens exactly once; the values set never change between calls,
// so doing it once is both sufficient and correct.
//
//nolint:gochecknoglobals // deliberate: memoizes an unsynchronized goose package-level setter, not application state.
var (
	configureGooseOnce sync.Once
	errConfigureGoose  error
)

// configureGoose points goose at the embedded migrations and the sqlite
// dialect. Every exported function below calls it directly; [sync.Once] makes
// repeated calls (including concurrent ones from parallel tests) cheap and
// race-free instead of re-running goose's global setters each time.
func configureGoose() error {
	configureGooseOnce.Do(func() {
		goose.SetBaseFS(migrationsFS)
		if err := goose.SetDialect("sqlite"); err != nil {
			errConfigureGoose = fmt.Errorf("db: set goose dialect: %w", err)
		}
	})

	return errConfigureGoose
}

// Migrate applies every pending migration in order, bringing sqlDB fully up
// to date. It's what `db migrate` runs directly (cmd/db.go), and what test
// setup calls before exercising sqlc queries. Safe to call when already up
// to date — goose treats an empty pending set as a no-op.
func Migrate(ctx context.Context, sqlDB *sql.DB) error {
	if err := configureGoose(); err != nil {
		return err
	}

	if err := goose.UpContext(ctx, sqlDB, migrationsFSDir); err != nil {
		return fmt.Errorf("db: migrate: %w", err)
	}

	return nil
}

// Rollback reverts the single most recently applied migration — not a full
// reset. It's what `db rollback` runs; call it repeatedly (once per
// migration) to undo more than one step.
func Rollback(ctx context.Context, sqlDB *sql.DB) error {
	if err := configureGoose(); err != nil {
		return err
	}

	if err := goose.DownContext(ctx, sqlDB, migrationsFSDir); err != nil {
		return fmt.Errorf("db: rollback: %w", err)
	}

	return nil
}

// Status prints which migrations are applied vs pending to goose's
// configured output (stdout by default).
func Status(ctx context.Context, sqlDB *sql.DB) error {
	if err := configureGoose(); err != nil {
		return err
	}

	if err := goose.StatusContext(ctx, sqlDB, migrationsFSDir); err != nil {
		return fmt.Errorf("db: status: %w", err)
	}

	return nil
}

// Create scaffolds a new empty SQL migration file named name in
// internal/db/migrations, writing to disk rather than the embedded FS.
func Create(name string) error {
	if err := goose.Create(nil, migrationsDirOnDisk, name, "sql"); err != nil {
		return fmt.Errorf("db: create migration: %w", err)
	}

	return nil
}
