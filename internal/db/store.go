package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/XSAM/otelsql"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver

	"github.com/DanielKirkwood/unwrap-gift/internal/config"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

// maxOpenConns caps the pool at a single connection, matching SQLite's
// single-writer constraint: concurrent writers from separate connections
// would otherwise race on SQLITE_BUSY. WAL mode still lets readers avoid
// blocking on the OS level even though every [sql.DB] user shares this one
// connection.
const maxOpenConns = 1

// busyTimeoutMS is how long a connection waits on SQLITE_BUSY before
// returning an error, rather than failing immediately under contention.
const busyTimeoutMS = 5000

// Store wraps a single SQLite connection and its generated queries. A nil
// *Store means the database feature is disabled — every method on Store is
// nil-receiver safe, mirroring otelclient.Providers' disabled-is-a-no-op
// convention.
type Store struct {
	DB      *sql.DB
	Queries *sqlc.Queries

	metrics metric.Registration
}

// New opens cfg's SQLite database and returns a Store, instrumented with
// otelsql (using tp/mp) for query spans and connection-pool metrics. It
// returns (nil, nil) when enabled is false — callers treat a nil Store as
// "database feature off," the same way app.Servers is nil until a serving
// command runs.
//
//nolint:nilnil // deliberate: nil is the "database feature disabled" state, not an error; every Store method is nil-receiver safe.
func New(cfg config.DatabaseConfig, enabled bool, tp trace.TracerProvider, mp metric.MeterProvider) (*Store, error) {
	if !enabled {
		return nil, nil
	}

	opts := []otelsql.Option{
		otelsql.WithAttributes(semconv.DBSystemNameSQLite),
		otelsql.WithTracerProvider(tp),
		otelsql.WithMeterProvider(mp),
	}

	sqlDB, err := otelsql.Open("sqlite", dsn(cfg.Path), opts...)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	sqlDB.SetMaxOpenConns(maxOpenConns)

	reg, err := otelsql.RegisterDBStatsMetrics(sqlDB, opts...)
	if err != nil {
		return nil, fmt.Errorf("db: register stats metrics: %w", err)
	}

	return &Store{DB: sqlDB, Queries: sqlc.New(sqlDB), metrics: reg}, nil
}

// Close unregisters the connection-pool metrics callback and closes the
// underlying connection. Safe to call on a nil Store.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}

	unregErr := s.metrics.Unregister()

	return errors.Join(unregErr, s.DB.Close())
}

// Ping reports whether the database is reachable, for use as an
// api.ReadyCheck. A nil Store always reports ready, matching the Phase 3
// default (no DB configured means nothing to check).
func (s *Store) Ping(ctx context.Context) error {
	if s == nil {
		return nil
	}

	return s.DB.PingContext(ctx)
}

// WithTx runs fn against a transaction-scoped *sqlc.Queries, committing if
// fn returns nil and rolling back otherwise — including on panic, which it
// re-panics after rollback.
func (s *Store) WithTx(ctx context.Context, fn func(*sqlc.Queries) error) (err error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()

	return fn(s.Queries.WithTx(tx))
}

// dsn builds a modernc.org/sqlite connection string with WAL mode and a
// busy timeout, so concurrent readers don't block on each other and writers
// wait rather than immediately failing under contention.
func dsn(path string) string {
	return fmt.Sprintf("file:%s?_journal=WAL&_timeout=%d", path, busyTimeoutMS)
}
