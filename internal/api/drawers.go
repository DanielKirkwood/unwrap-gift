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

// ErrDrawerNotFound is returned by DrawerStore implementations when no
// drawer exists for the given id.
var ErrDrawerNotFound = errors.New("api: drawer not found")

// Drawer is api's own representation of a drawer row, decoupled from
// internal/db/sqlc.Drawer — api never imports internal/db, per
// ARCHITECTURE.md's dependency-direction rule.
type Drawer struct {
	ID                        int64     `json:"id"`
	Name                      string    `json:"name"`
	OrganiserKratosIdentityID string    `json:"organiser_kratos_identity_id"`
	HistoryWindowDraws        int64     `json:"history_window_draws"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

// DrawerStore is the subset of persistence the drawer handlers need,
// implemented by an adapter app constructs over *db.Store and injects via
// RouterDeps.
type DrawerStore interface {
	CreateDrawer(ctx context.Context, name, organiserIdentityID string) (Drawer, error)
	GetDrawer(ctx context.Context, id int64) (Drawer, error)
	ListDrawersByOrganiser(ctx context.Context, organiserIdentityID string) ([]Drawer, error)
	UpdateDrawer(ctx context.Context, id int64, name string, historyWindowDraws int64) (Drawer, error)
	DeleteDrawer(ctx context.Context, id int64) error
}

// drawerCreateRequest is the request body for POST /drawers.
type drawerCreateRequest struct {
	Name string `json:"name"`
}

// drawerUpdateRequest is the request body for PUT /drawers/{id} — a full
// replace, matching widgetRequest's style, not a partial PATCH.
type drawerUpdateRequest struct {
	Name               string `json:"name"`
	HistoryWindowDraws int64  `json:"history_window_draws"`
}

// MountDrawers adds the organiser drawer CRUD endpoints to r, under
// /drawers, each routed through adapter so DrawerStore errors become
// Problem responses.
func MountDrawers(r chi.Router, drawers DrawerStore, adapter Adapter) {
	r.Post("/drawers", adapter.Adapt(createDrawer(drawers)))
	r.Get("/drawers", adapter.Adapt(listDrawers(drawers)))
	r.Get("/drawers/{id}", adapter.Adapt(getDrawer(drawers)))
	r.Put("/drawers/{id}", adapter.Adapt(updateDrawer(drawers)))
	r.Delete("/drawers/{id}", adapter.Adapt(deleteDrawer(drawers)))
}

// createDrawer sets OrganiserKratosIdentityID from the caller's own
// identity (never from the request body), so it can't be spoofed by a
// client-supplied value.
func createDrawer(drawers DrawerStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			return WriteProblem(w, unauthorizedProblem())
		}

		var req drawerCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return fmt.Errorf("api: decode create drawer request: %w", err)
		}

		drawer, err := drawers.CreateDrawer(r.Context(), req.Name, identity.Id)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusCreated, drawer)
	}
}

// listDrawers returns the caller's own drawers — a convenience "my
// drawers" view, not an authorization boundary (see Decisions table in the
// Phase 5 plan: any admin can still GET /drawers/{id} directly regardless
// of who created it).
func listDrawers(drawers DrawerStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			return WriteProblem(w, unauthorizedProblem())
		}

		list, err := drawers.ListDrawersByOrganiser(r.Context(), identity.Id)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, list)
	}
}

func getDrawer(drawers DrawerStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := drawerID(r)
		if err != nil {
			return err
		}

		drawer, err := drawers.GetDrawer(r.Context(), id)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, drawer)
	}
}

func updateDrawer(drawers DrawerStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := drawerID(r)
		if err != nil {
			return err
		}

		var req drawerUpdateRequest
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			return fmt.Errorf("api: decode update drawer request: %w", decodeErr)
		}

		drawer, err := drawers.UpdateDrawer(r.Context(), id, req.Name, req.HistoryWindowDraws)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, drawer)
	}
}

func deleteDrawer(drawers DrawerStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := drawerID(r)
		if err != nil {
			return err
		}

		if deleteErr := drawers.DeleteDrawer(r.Context(), id); deleteErr != nil {
			return deleteErr
		}

		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

// drawerID parses the {id} URL param as an int64.
func drawerID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("api: parse drawer id: %w", err)
	}

	return id, nil
}
