package app //nolint:testpackage // intentional: needs direct access to unexported storeRelationships, see newAppTestStore (draws_test.go)

// This file is intentionally in package app (not app_test): it needs direct
// access to the unexported storeRelationships type to exercise
// DeleteRelationship's not-found check against a real temp SQLite
// *db.Store — a delete of a nonexistent id used to return 204 No Content
// instead of surfacing api.ErrRelationshipNotFound (see
// codebase-review-2026-10-02.md, "Correctness bugs" #1 / issue #25).
// RelationshipStore deliberately has no GetRelationship query, so this
// relies on DeleteRelationship's :execrows rows-affected count rather than
// a pre-existence check, unlike storeDrawers/storeWidgets.

import (
	"errors"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
)

func TestStoreRelationships_DeleteNonexistentReturnsNotFound(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)
	relationships := storeRelationships{store: store}

	err := relationships.DeleteRelationship(t.Context(), 999)
	if !errors.Is(err, api.ErrRelationshipNotFound) {
		t.Errorf("DeleteRelationship() (nonexistent) error = %v, want api.ErrRelationshipNotFound", err)
	}
}

func TestStoreRelationships_DeleteExistingSucceeds(t *testing.T) {
	t.Parallel()

	store := newAppTestStore(t)
	relationships := storeRelationships{store: store}

	created, err := relationships.CreateRelationship(t.Context(), "+447700900000", "+447700900001")
	if err != nil {
		t.Fatalf("CreateRelationship() error = %v, want nil", err)
	}

	if deleteErr := relationships.DeleteRelationship(t.Context(), created.ID); deleteErr != nil {
		t.Fatalf("DeleteRelationship() error = %v, want nil", deleteErr)
	}

	deleteAgainErr := relationships.DeleteRelationship(t.Context(), created.ID)
	if !errors.Is(deleteAgainErr, api.ErrRelationshipNotFound) {
		t.Errorf(
			"DeleteRelationship() (already deleted) error = %v, want api.ErrRelationshipNotFound",
			deleteAgainErr,
		)
	}
}
