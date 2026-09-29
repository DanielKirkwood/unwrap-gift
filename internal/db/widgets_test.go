package db_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

func TestWidgetQueries_CRUD(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if err := db.Migrate(t.Context(), store.DB); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}

	created, err := store.Queries.CreateWidget(t.Context(), "sprocket")
	if err != nil {
		t.Fatalf("CreateWidget() error = %v, want nil", err)
	}
	if created.Name != "sprocket" {
		t.Errorf("CreateWidget() name = %q, want sprocket", created.Name)
	}

	got, err := store.Queries.GetWidget(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetWidget() error = %v, want nil", err)
	}
	if got != created {
		t.Errorf("GetWidget() = %+v, want %+v", got, created)
	}

	updated, err := store.Queries.UpdateWidget(t.Context(), sqlc.UpdateWidgetParams{ID: created.ID, Name: "gizmo"})
	if err != nil {
		t.Fatalf("UpdateWidget() error = %v, want nil", err)
	}
	if updated.Name != "gizmo" {
		t.Errorf("UpdateWidget() name = %q, want gizmo", updated.Name)
	}

	list, err := store.Queries.ListWidgets(t.Context())
	if err != nil {
		t.Fatalf("ListWidgets() error = %v, want nil", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListWidgets() len = %d, want 1", len(list))
	}

	if deleteErr := store.Queries.DeleteWidget(t.Context(), created.ID); deleteErr != nil {
		t.Fatalf("DeleteWidget() error = %v, want nil", deleteErr)
	}

	if _, getErr := store.Queries.GetWidget(t.Context(), created.ID); !errors.Is(getErr, sql.ErrNoRows) {
		t.Errorf("GetWidget() after delete error = %v, want sql.ErrNoRows", getErr)
	}
}
