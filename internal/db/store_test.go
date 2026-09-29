package db_test

import (
	"errors"
	"path/filepath"
	"testing"

	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/DanielKirkwood/unwrap-gift/internal/config"
	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

func TestNewDisabled(t *testing.T) {
	t.Parallel()

	store, err := db.New(config.DatabaseConfig{}, false, tracenoop.NewTracerProvider(), noop.NewMeterProvider())
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if store != nil {
		t.Fatalf("New() store = %v, want nil", store)
	}

	if pingErr := store.Ping(t.Context()); pingErr != nil {
		t.Errorf("nil Store.Ping() error = %v, want nil", pingErr)
	}
	if closeErr := store.Close(); closeErr != nil {
		t.Errorf("nil Store.Close() error = %v, want nil", closeErr)
	}
}

func TestNewEnabled_Ping(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	if err := store.Ping(t.Context()); err != nil {
		t.Fatalf("Ping() error = %v, want nil", err)
	}
}

func TestMigrateStatusRollback(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	if err := db.Status(t.Context(), store.DB); err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if err := db.Rollback(t.Context(), store.DB); err != nil {
		t.Fatalf("Rollback() error = %v, want nil", err)
	}
}

func TestStore_WithTx(t *testing.T) {
	t.Parallel()

	t.Run("commits on success", func(t *testing.T) {
		t.Parallel()

		store := newTestStore(t)

		var got int64
		err := store.WithTx(t.Context(), func(q *sqlc.Queries) error {
			v, err := q.Ping(t.Context())
			got = v
			return err
		})
		if err != nil {
			t.Fatalf("WithTx() error = %v, want nil", err)
		}
		if got != 1 {
			t.Fatalf("Ping() = %d, want 1", got)
		}
	})

	t.Run("rolls back and propagates fn's error", func(t *testing.T) {
		t.Parallel()

		store := newTestStore(t)
		wantErr := errors.New("boom")

		err := store.WithTx(t.Context(), func(*sqlc.Queries) error {
			return wantErr
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("WithTx() error = %v, want %v", err, wantErr)
		}
	})
}

// newTestStore opens a Store against a fresh temp-dir SQLite file, closed
// automatically at test cleanup.
func newTestStore(t *testing.T) *db.Store {
	t.Helper()

	cfg := config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "test.db")}

	store, err := db.New(cfg, true, tracenoop.NewTracerProvider(), noop.NewMeterProvider())
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	return store
}
