package app //nolint:testpackage // intentional: needs direct access to unexported storeDrawers, see newAppTestStore (draws_test.go)

// This file is intentionally in package app (not app_test): it needs direct
// access to the unexported storeDrawers type to exercise
// DeleteDrawer's not-found check against a real temp SQLite *db.Store —
// a delete of a nonexistent id used to return 204 No Content instead of
// surfacing api.ErrDrawerNotFound (see codebase-review-2026-10-02.md,
// "Correctness bugs" #1 / issue #25).

import (
	"errors"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

func TestStoreDrawers_DeleteNonexistentReturnsNotFound(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)
	drawers := storeDrawers{store: store}

	err := drawers.DeleteDrawer(t.Context(), 999)
	if !errors.Is(err, api.ErrDrawerNotFound) {
		t.Errorf("DeleteDrawer() (nonexistent) error = %v, want api.ErrDrawerNotFound", err)
	}
}

func TestStoreDrawers_DeleteExistingSucceeds(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)
	drawers := storeDrawers{store: store}

	created, err := drawers.CreateDrawer(t.Context(), "Kirkwood Family Christmas", "identity-1")
	if err != nil {
		t.Fatalf("CreateDrawer() error = %v, want nil", err)
	}

	if deleteErr := drawers.DeleteDrawer(t.Context(), created.ID); deleteErr != nil {
		t.Fatalf("DeleteDrawer() error = %v, want nil", deleteErr)
	}

	_, getErr := drawers.GetDrawer(t.Context(), created.ID)
	if !errors.Is(getErr, api.ErrDrawerNotFound) {
		t.Errorf("GetDrawer() (after delete) error = %v, want api.ErrDrawerNotFound", getErr)
	}
}
