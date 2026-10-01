package db_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

func TestDrawerQueries_CRUD(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	created, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Office Secret Santa",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}
	if created.Name != "Office Secret Santa" {
		t.Errorf("CreateDrawer() name = %q, want Office Secret Santa", created.Name)
	}

	got, err := store.Queries.GetDrawer(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetDrawer() error = %v, want nil", err)
	}
	if got != created {
		t.Errorf("GetDrawer() = %+v, want %+v", got, created)
	}

	updated, err := store.Queries.UpdateDrawer(
		t.Context(),
		sqlc.UpdateDrawerParams{ID: created.ID, Name: "Family Secret Santa", HistoryWindowDraws: 5},
	)
	if err != nil {
		t.Fatalf("UpdateDrawer() error = %v, want nil", err)
	}
	if updated.Name != "Family Secret Santa" {
		t.Errorf("UpdateDrawer() name = %q, want Family Secret Santa", updated.Name)
	}
	if updated.HistoryWindowDraws != 5 {
		t.Errorf("UpdateDrawer() history_window_draws = %d, want 5", updated.HistoryWindowDraws)
	}

	got, err = store.Queries.GetDrawer(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetDrawer() after update error = %v, want nil", err)
	}
	if got.HistoryWindowDraws != 5 {
		t.Errorf("GetDrawer() after update history_window_draws = %d, want 5", got.HistoryWindowDraws)
	}

	list, err := store.Queries.ListDrawersByOrganiser(t.Context(), "organiser-1")
	if err != nil {
		t.Fatalf("ListDrawersByOrganiser() error = %v, want nil", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListDrawersByOrganiser() len = %d, want 1", len(list))
	}

	if deleteErr := store.Queries.DeleteDrawer(t.Context(), created.ID); deleteErr != nil {
		t.Fatalf("DeleteDrawer() error = %v, want nil", deleteErr)
	}

	if _, getErr := store.Queries.GetDrawer(t.Context(), created.ID); !errors.Is(getErr, sql.ErrNoRows) {
		t.Errorf("GetDrawer() after delete error = %v, want sql.ErrNoRows", getErr)
	}
}
