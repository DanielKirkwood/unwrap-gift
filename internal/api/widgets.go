package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// ErrWidgetNotFound is returned by WidgetStore implementations when no
// widget exists for the given id. app maps it to a 404 Problem via the
// Adapter it builds for the protected router.
var ErrWidgetNotFound = errors.New("api: widget not found")

// Widget is api's own representation of a widget row, decoupled from
// internal/db/sqlc.Widget — api never imports internal/db, per
// ARCHITECTURE.md's dependency-direction rule. internal/app's WidgetStore
// adapter converts between the two.
type Widget struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// WidgetStore is the subset of persistence the widget handlers need,
// implemented by an adapter app constructs over *db.Store and injects via
// RouterDeps — the same interface-not-concrete-type pattern IdentityAdmin
// uses for Kratos.
type WidgetStore interface {
	CreateWidget(ctx context.Context, name string) (Widget, error)
	GetWidget(ctx context.Context, id int64) (Widget, error)
	ListWidgets(ctx context.Context) ([]Widget, error)
	UpdateWidget(ctx context.Context, id int64, name string) (Widget, error)
	DeleteWidget(ctx context.Context, id int64) error
}

// widgetRequest is the request body for both POST /widgets and
// PUT /widgets/{id}.
type widgetRequest struct {
	Name string `json:"name"`
}

// MountWidgets adds the widget CRUD example endpoints to r, under
// /widgets, each routed through adapter so WidgetStore errors become
// Problem responses. This is the example resource Phase 7's plan calls
// for: it proves sqlc-backed persistence through chi on the protected
// router, the one combination identities.go (hidden router, no DB) doesn't
// already cover.
func MountWidgets(r chi.Router, widgets WidgetStore, adapter Adapter) {
	r.Post("/widgets", adapter.Adapt(createWidget(widgets)))
	r.Get("/widgets", adapter.Adapt(listWidgets(widgets)))
	r.Get("/widgets/{id}", adapter.Adapt(getWidget(widgets)))
	r.Put("/widgets/{id}", adapter.Adapt(updateWidget(widgets)))
	r.Delete("/widgets/{id}", adapter.Adapt(deleteWidget(widgets)))
}

func createWidget(widgets WidgetStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		var req widgetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return fmt.Errorf("api: decode create widget request: %w", err)
		}

		widget, err := widgets.CreateWidget(r.Context(), req.Name)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusCreated, widget)
	}
}

func listWidgets(widgets WidgetStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		list, err := widgets.ListWidgets(r.Context())
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, list)
	}
}

func getWidget(widgets WidgetStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := widgetID(r)
		if err != nil {
			return err
		}

		widget, err := widgets.GetWidget(r.Context(), id)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, widget)
	}
}

func updateWidget(widgets WidgetStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := widgetID(r)
		if err != nil {
			return err
		}

		var req widgetRequest
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			return fmt.Errorf("api: decode update widget request: %w", decodeErr)
		}

		widget, err := widgets.UpdateWidget(r.Context(), id, req.Name)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, widget)
	}
}

func deleteWidget(widgets WidgetStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := widgetID(r)
		if err != nil {
			return err
		}

		if deleteErr := widgets.DeleteWidget(r.Context(), id); deleteErr != nil {
			return deleteErr
		}

		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

// widgetID parses the {id} URL param as an int64, wrapping any parse
// failure so the adapter's unmapped-error path reports it as a 500 rather
// than panicking — a malformed id is a client error in spirit, but this
// mirrors identities.go's string-id handling: no dedicated 400 Problem for
// bad path params exists yet anywhere in this codebase.
func widgetID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("api: parse widget id: %w", err)
	}

	return id, nil
}
