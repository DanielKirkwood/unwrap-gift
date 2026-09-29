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

// storeWidgets adapts *db.Store to api.WidgetStore, converting between
// sqlc's generated Widget type and api.Widget. api never imports
// internal/db, per ARCHITECTURE.md's dependency-direction rule, so this
// conversion has to happen here in app — the one package allowed to import
// both.
type storeWidgets struct {
	store *db.Store
}

func (s storeWidgets) CreateWidget(ctx context.Context, name string) (api.Widget, error) {
	widget, err := s.store.Queries.CreateWidget(ctx, name)
	if err != nil {
		return api.Widget{}, fmt.Errorf("app: create widget: %w", err)
	}

	return toAPIWidget(widget), nil
}

func (s storeWidgets) GetWidget(ctx context.Context, id int64) (api.Widget, error) {
	widget, err := s.store.Queries.GetWidget(ctx, id)
	if err != nil {
		return api.Widget{}, mapWidgetErr(err)
	}

	return toAPIWidget(widget), nil
}

func (s storeWidgets) ListWidgets(ctx context.Context) ([]api.Widget, error) {
	rows, err := s.store.Queries.ListWidgets(ctx)
	if err != nil {
		return nil, fmt.Errorf("app: list widgets: %w", err)
	}

	out := make([]api.Widget, len(rows))
	for i, row := range rows {
		out[i] = toAPIWidget(row)
	}

	return out, nil
}

func (s storeWidgets) UpdateWidget(ctx context.Context, id int64, name string) (api.Widget, error) {
	widget, err := s.store.Queries.UpdateWidget(ctx, sqlc.UpdateWidgetParams{ID: id, Name: name})
	if err != nil {
		return api.Widget{}, mapWidgetErr(err)
	}

	return toAPIWidget(widget), nil
}

func (s storeWidgets) DeleteWidget(ctx context.Context, id int64) error {
	if err := s.store.Queries.DeleteWidget(ctx, id); err != nil {
		return fmt.Errorf("app: delete widget: %w", err)
	}

	return nil
}

func toAPIWidget(w sqlc.Widget) api.Widget {
	return api.Widget{ID: w.ID, Name: w.Name, CreatedAt: w.CreatedAt}
}

// mapWidgetErr translates [sql.ErrNoRows] (sqlc's :one queries return it
// when no row matches) into api.ErrWidgetNotFound, mirroring
// kratosclient.mapIdentityErr's 404 translation for Kratos.
func mapWidgetErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return api.ErrWidgetNotFound
	}

	return fmt.Errorf("app: widget store: %w", err)
}
