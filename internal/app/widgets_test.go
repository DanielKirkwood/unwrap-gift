package app //nolint:testpackage // intentional: needs direct access to unexported storeWidgets, see newAppTestStore (draws_test.go)

// This file is intentionally in package app (not app_test): it needs direct
// access to the unexported storeWidgets type to exercise DeleteWidget's
// not-found check against a real temp SQLite *db.Store — a delete of a
// nonexistent id used to return 204 No Content instead of surfacing
// api.ErrWidgetNotFound (see codebase-review-2026-10-02.md, "Correctness
// bugs" #1 / issue #25).

import (
	"errors"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

func TestStoreWidgets_DeleteNonexistentReturnsNotFound(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)
	widgets := storeWidgets{store: store}

	err := widgets.DeleteWidget(t.Context(), 999)
	if !errors.Is(err, api.ErrWidgetNotFound) {
		t.Errorf("DeleteWidget() (nonexistent) error = %v, want api.ErrWidgetNotFound", err)
	}
}

func TestStoreWidgets_DeleteExistingSucceeds(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)
	widgets := storeWidgets{store: store}

	created, err := widgets.CreateWidget(t.Context(), "Test Widget")
	if err != nil {
		t.Fatalf("CreateWidget() error = %v, want nil", err)
	}

	if deleteErr := widgets.DeleteWidget(t.Context(), created.ID); deleteErr != nil {
		t.Fatalf("DeleteWidget() error = %v, want nil", deleteErr)
	}

	_, getErr := widgets.GetWidget(t.Context(), created.ID)
	if !errors.Is(getErr, api.ErrWidgetNotFound) {
		t.Errorf("GetWidget() (after delete) error = %v, want api.ErrWidgetNotFound", getErr)
	}
}
