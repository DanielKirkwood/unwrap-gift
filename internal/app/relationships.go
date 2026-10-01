package app

import (
	"context"
	"fmt"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

// storeRelationships adapts *db.Store to api.RelationshipStore.
type storeRelationships struct {
	store *db.Store
}

// CreateRelationship rejects a == b before touching the DB, then sorts the
// pair so a < b lexicographically — the sqlc query's CHECK constraint
// requires this, and this file is responsible for the sort, not the
// caller. Go's string < on E.164 numbers matches SQLite's own TEXT
// collation, so no numeric parsing is needed.
func (s storeRelationships) CreateRelationship(
	ctx context.Context,
	phoneNumberA, phoneNumberB string,
) (api.Relationship, error) {
	if phoneNumberA == phoneNumberB {
		return api.Relationship{}, api.ErrRelationshipSamePhoneNumber
	}

	if phoneNumberA > phoneNumberB {
		phoneNumberA, phoneNumberB = phoneNumberB, phoneNumberA
	}

	relationship, err := s.store.Queries.CreateRelationship(ctx, sqlc.CreateRelationshipParams{
		PhoneNumberA: phoneNumberA,
		PhoneNumberB: phoneNumberB,
	})
	if err != nil {
		if isUniqueConstraintErr(err) {
			return api.Relationship{}, api.ErrRelationshipAlreadyExists
		}
		return api.Relationship{}, fmt.Errorf("app: create relationship: %w", err)
	}

	return toAPIRelationship(relationship), nil
}

func (s storeRelationships) ListRelationshipsForPhoneNumber(
	ctx context.Context,
	phoneNumber string,
) ([]api.Relationship, error) {
	rows, err := s.store.Queries.ListRelationshipsForPhoneNumber(ctx, sqlc.ListRelationshipsForPhoneNumberParams{
		PhoneNumberA: phoneNumber,
		PhoneNumberB: phoneNumber,
	})
	if err != nil {
		return nil, fmt.Errorf("app: list relationships for phone number: %w", err)
	}

	out := make([]api.Relationship, len(rows))
	for i, row := range rows {
		out[i] = toAPIRelationship(row)
	}

	return out, nil
}

func (s storeRelationships) DeleteRelationship(ctx context.Context, id int64) error {
	if err := s.store.Queries.DeleteRelationship(ctx, id); err != nil {
		return fmt.Errorf("app: delete relationship: %w", err)
	}

	return nil
}

func toAPIRelationship(r sqlc.Relationship) api.Relationship {
	return api.Relationship{
		ID:           r.ID,
		PhoneNumberA: r.PhoneNumberA,
		PhoneNumberB: r.PhoneNumberB,
		CreatedAt:    r.CreatedAt,
	}
}
