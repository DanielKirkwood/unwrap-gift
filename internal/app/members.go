package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

// storeMembers adapts *db.Store to api.MemberStore, converting between
// sqlc's generated Member type and api.Member. GetMember/UpdateMember/
// DeleteMember all enforce the "is this member actually in this drawer"
// check here — the one place in this plan the business rule lives, rather
// than duplicated across every handler.
type storeMembers struct {
	store *db.Store
}

func (s storeMembers) CreateMember(
	ctx context.Context,
	drawerID int64,
	fullName, phoneNumber string,
) (api.Member, error) {
	member, err := s.store.Queries.CreateMember(ctx, sqlc.CreateMemberParams{
		DrawerID:    drawerID,
		FullName:    fullName,
		PhoneNumber: phoneNumber,
	})
	if err != nil {
		if isUniqueConstraintErr(err) {
			return api.Member{}, api.ErrMemberPhoneAlreadyExists
		}
		return api.Member{}, fmt.Errorf("app: create member: %w", err)
	}

	return toAPIMember(member), nil
}

func (s storeMembers) GetMember(ctx context.Context, drawerID, id int64) (api.Member, error) {
	member, err := s.store.Queries.GetMember(ctx, id)
	if err != nil {
		return api.Member{}, mapMemberErr(err)
	}
	if member.DrawerID != drawerID {
		return api.Member{}, api.ErrMemberNotFound
	}

	return toAPIMember(member), nil
}

func (s storeMembers) ListMembersByDrawer(ctx context.Context, drawerID int64) ([]api.Member, error) {
	rows, err := s.store.Queries.ListMembersByDrawer(ctx, drawerID)
	if err != nil {
		return nil, fmt.Errorf("app: list members by drawer: %w", err)
	}

	out := make([]api.Member, len(rows))
	for i, row := range rows {
		out[i] = toAPIMember(row)
	}

	return out, nil
}

func (s storeMembers) UpdateMember(
	ctx context.Context,
	drawerID, id int64,
	fullName, phoneNumber string,
) (api.Member, error) {
	existing, err := s.store.Queries.GetMember(ctx, id)
	if err != nil {
		return api.Member{}, mapMemberErr(err)
	}
	if existing.DrawerID != drawerID {
		return api.Member{}, api.ErrMemberNotFound
	}

	member, err := s.store.Queries.UpdateMember(ctx, sqlc.UpdateMemberParams{
		ID:          id,
		FullName:    fullName,
		PhoneNumber: phoneNumber,
	})
	if err != nil {
		if isUniqueConstraintErr(err) {
			return api.Member{}, api.ErrMemberPhoneAlreadyExists
		}
		return api.Member{}, fmt.Errorf("app: update member: %w", err)
	}

	return toAPIMember(member), nil
}

func (s storeMembers) DeleteMember(ctx context.Context, drawerID, id int64) error {
	existing, err := s.store.Queries.GetMember(ctx, id)
	if err != nil {
		return mapMemberErr(err)
	}
	if existing.DrawerID != drawerID {
		return api.ErrMemberNotFound
	}

	if deleteErr := s.store.Queries.DeleteMember(ctx, id); deleteErr != nil {
		return fmt.Errorf("app: delete member: %w", deleteErr)
	}

	return nil
}

func toAPIMember(m sqlc.Member) api.Member {
	return api.Member{
		ID:          m.ID,
		DrawerID:    m.DrawerID,
		FullName:    m.FullName,
		PhoneNumber: m.PhoneNumber,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// mapMemberErr translates [sql.ErrNoRows] into api.ErrMemberNotFound.
func mapMemberErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return api.ErrMemberNotFound
	}

	return fmt.Errorf("app: member store: %w", err)
}

// isUniqueConstraintErr reports whether err is a SQLite UNIQUE constraint
// violation (code 2067), confirmed against modernc.org/sqlite@v1.60.0's
// lib.SQLITE_CONSTRAINT_UNIQUE constant.
func isUniqueConstraintErr(err error) bool {
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE
}
