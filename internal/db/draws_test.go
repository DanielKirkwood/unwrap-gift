package db_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

func TestDrawQueries_CRUD(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Office Secret Santa",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	exchangeDate := time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC)

	created, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
		DrawerID:     drawer.ID,
		ExchangeDate: exchangeDate,
		BudgetAmount: 2000,
	})
	if err != nil {
		t.Fatalf("CreateDraw() error = %v, want nil", err)
	}
	if created.Status != "draft" {
		t.Errorf("CreateDraw() status = %q, want draft", created.Status)
	}

	got, err := store.Queries.GetDraw(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetDraw() error = %v, want nil", err)
	}
	if got != created {
		t.Errorf("GetDraw() = %+v, want %+v", got, created)
	}

	assigned, err := store.Queries.UpdateDrawStatus(
		t.Context(),
		sqlc.UpdateDrawStatusParams{ID: created.ID, Status: "assigned"},
	)
	if err != nil {
		t.Fatalf("UpdateDrawStatus(assigned) error = %v, want nil", err)
	}
	if assigned.Status != "assigned" {
		t.Errorf("UpdateDrawStatus(assigned) status = %q, want assigned", assigned.Status)
	}

	notified, err := store.Queries.UpdateDrawStatus(
		t.Context(),
		sqlc.UpdateDrawStatusParams{ID: created.ID, Status: "notified"},
	)
	if err != nil {
		t.Fatalf("UpdateDrawStatus(notified) error = %v, want nil", err)
	}
	if notified.Status != "notified" {
		t.Errorf("UpdateDrawStatus(notified) status = %q, want notified", notified.Status)
	}

	list, err := store.Queries.ListDrawsByDrawer(t.Context(), drawer.ID)
	if err != nil {
		t.Fatalf("ListDrawsByDrawer() error = %v, want nil", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListDrawsByDrawer() len = %d, want 1", len(list))
	}

	if deleteErr := store.Queries.DeleteDraw(t.Context(), created.ID); deleteErr != nil {
		t.Fatalf("DeleteDraw() error = %v, want nil", deleteErr)
	}

	if _, getErr := store.Queries.GetDraw(t.Context(), created.ID); !errors.Is(getErr, sql.ErrNoRows) {
		t.Errorf("GetDraw() after delete error = %v, want sql.ErrNoRows", getErr)
	}
}

func TestUpdateDrawStatus_RejectsInvalidStatus(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Office Secret Santa",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	draw, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
		DrawerID:     drawer.ID,
		ExchangeDate: time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC),
		BudgetAmount: 2000,
	})
	if err != nil {
		t.Fatalf("CreateDraw() error = %v, want nil", err)
	}

	_, err = store.Queries.UpdateDrawStatus(t.Context(), sqlc.UpdateDrawStatusParams{ID: draw.ID, Status: "bogus"})
	if err == nil {
		t.Fatal("UpdateDrawStatus() with invalid status error = nil, want non-nil")
	}
}

func TestListRecentCompletedDrawsByDrawer(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	drawer, err := store.Queries.CreateDrawer(t.Context(), sqlc.CreateDrawerParams{
		Name:                      "Office Secret Santa",
		OrganiserKratosIdentityID: "organiser-1",
	})
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	older, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
		DrawerID:     drawer.ID,
		ExchangeDate: time.Date(2024, time.December, 25, 0, 0, 0, 0, time.UTC),
		BudgetAmount: 2000,
	})
	if err != nil {
		t.Fatalf("CreateDraw(older) error = %v, want nil", err)
	}
	if _, statusErr := store.Queries.UpdateDrawStatus(
		t.Context(), sqlc.UpdateDrawStatusParams{ID: older.ID, Status: "assigned"},
	); statusErr != nil {
		t.Fatalf("UpdateDrawStatus(older, assigned) error = %v, want nil", statusErr)
	}

	mostRecent, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
		DrawerID:     drawer.ID,
		ExchangeDate: time.Date(2025, time.December, 25, 0, 0, 0, 0, time.UTC),
		BudgetAmount: 2000,
	})
	if err != nil {
		t.Fatalf("CreateDraw(mostRecent) error = %v, want nil", err)
	}
	if _, statusErr := store.Queries.UpdateDrawStatus(
		t.Context(), sqlc.UpdateDrawStatusParams{ID: mostRecent.ID, Status: "assigned"},
	); statusErr != nil {
		t.Fatalf("UpdateDrawStatus(mostRecent, assigned) error = %v, want nil", statusErr)
	}

	// draft is left in 'draft' status (CreateDraw's default) and must never
	// be treated as history.
	draft, err := store.Queries.CreateDraw(t.Context(), sqlc.CreateDrawParams{
		DrawerID:     drawer.ID,
		ExchangeDate: time.Date(2026, time.December, 25, 0, 0, 0, 0, time.UTC),
		BudgetAmount: 2000,
	})
	if err != nil {
		t.Fatalf("CreateDraw(draft) error = %v, want nil", err)
	}

	list, err := store.Queries.ListRecentCompletedDrawsByDrawer(
		t.Context(),
		sqlc.ListRecentCompletedDrawsByDrawerParams{
			DrawerID: drawer.ID,
			ID:       draft.ID,
			Limit:    1,
		},
	)
	if err != nil {
		t.Fatalf("ListRecentCompletedDrawsByDrawer() error = %v, want nil", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListRecentCompletedDrawsByDrawer() len = %d, want 1", len(list))
	}
	if list[0].ID != mostRecent.ID {
		t.Errorf(
			"ListRecentCompletedDrawsByDrawer() = draw %d, want the most recent assigned draw %d",
			list[0].ID,
			mostRecent.ID,
		)
	}
}
