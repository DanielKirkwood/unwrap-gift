package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

// storeDrawers adapts *db.Store to api.DrawerStore, converting between
// sqlc's generated Drawer type and api.Drawer.
type storeDrawers struct {
	store *db.Store
}

func (s storeDrawers) CreateDrawer(ctx context.Context, name, organiserIdentityID string) (api.Drawer, error) {
	drawer, err := s.store.Queries.CreateDrawer(ctx, sqlc.CreateDrawerParams{
		Name:                      name,
		OrganiserKratosIdentityID: organiserIdentityID,
	})
	if err != nil {
		return api.Drawer{}, fmt.Errorf("app: create drawer: %w", err)
	}

	return toAPIDrawer(drawer), nil
}

func (s storeDrawers) GetDrawer(ctx context.Context, id int64) (api.Drawer, error) {
	drawer, err := s.store.Queries.GetDrawer(ctx, id)
	if err != nil {
		return api.Drawer{}, mapDrawerErr(err)
	}

	return toAPIDrawer(drawer), nil
}

func (s storeDrawers) ListDrawersByOrganiser(ctx context.Context, organiserIdentityID string) ([]api.Drawer, error) {
	rows, err := s.store.Queries.ListDrawersByOrganiser(ctx, organiserIdentityID)
	if err != nil {
		return nil, fmt.Errorf("app: list drawers by organiser: %w", err)
	}

	out := make([]api.Drawer, len(rows))
	for i, row := range rows {
		out[i] = toAPIDrawer(row)
	}

	return out, nil
}

func (s storeDrawers) UpdateDrawer(
	ctx context.Context,
	id int64,
	name string,
	historyWindowDraws int64,
) (api.Drawer, error) {
	drawer, err := s.store.Queries.UpdateDrawer(ctx, sqlc.UpdateDrawerParams{
		ID:                 id,
		Name:               name,
		HistoryWindowDraws: historyWindowDraws,
	})
	if err != nil {
		return api.Drawer{}, mapDrawerErr(err)
	}

	return toAPIDrawer(drawer), nil
}

func (s storeDrawers) DeleteDrawer(ctx context.Context, id int64) error {
	if _, err := s.store.Queries.GetDrawer(ctx, id); err != nil {
		return mapDrawerErr(err)
	}

	if err := s.store.Queries.DeleteDrawer(ctx, id); err != nil {
		return fmt.Errorf("app: delete drawer: %w", err)
	}

	return nil
}

func toAPIDrawer(d sqlc.Drawer) api.Drawer {
	return api.Drawer{
		ID:                        d.ID,
		Name:                      d.Name,
		OrganiserKratosIdentityID: d.OrganiserKratosIdentityID,
		HistoryWindowDraws:        d.HistoryWindowDraws,
		CreatedAt:                 d.CreatedAt,
		UpdatedAt:                 d.UpdatedAt,
	}
}

// mapDrawerErr translates [sql.ErrNoRows] into api.ErrDrawerNotFound,
// mirroring mapWidgetErr's 404 translation.
func mapDrawerErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return api.ErrDrawerNotFound
	}

	return fmt.Errorf("app: drawer store: %w", err)
}
