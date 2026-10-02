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

// ErrDrawNotFound is returned by DrawStore implementations when no draw
// exists for the given id within the given drawer.
var ErrDrawNotFound = errors.New("api: draw not found")

// ErrDrawAlreadyRun is returned by DrawStore.RunDraw when the draw's
// status is no longer 'draft'.
var ErrDrawAlreadyRun = errors.New("api: draw has already been run")

// ErrNoValidAssignment is returned by DrawStore.RunDraw when no
// gifter→giftee mapping satisfies the drawer's current
// members/exclusions/history, even after relaxing the history window to
// zero.
var ErrNoValidAssignment = errors.New(
	"api: no valid assignment exists for this drawer's current members/exclusions/history",
)

// ErrTooFewMembers is returned by DrawStore.RunDraw when the drawer has
// fewer than two members.
var ErrTooFewMembers = errors.New("api: drawer has fewer than two members")

// ErrNotificationFailed is returned by DrawStore.RunDraw when assignment
// succeeds and is persisted, but sending the notification SMS to one or
// more members fails. The draw's status stays 'assigned' (not
// 'notified') in this case — see internal/app's
// storeDraws.finalizeNotifications.
var ErrNotificationFailed = errors.New("api: failed to send notification sms to one or more members")

// Draw is api's own representation of a draw row, decoupled from
// internal/db/sqlc.Draw.
type Draw struct {
	ID           int64     `json:"id"`
	DrawerID     int64     `json:"drawer_id"`
	ExchangeDate time.Time `json:"exchange_date"`
	BudgetAmount int64     `json:"budget_amount"` // minor units (pence)
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Assignment is api's own representation of an assignment row, decoupled
// from internal/db/sqlc.Assignment.
type Assignment struct {
	ID             int64     `json:"id"`
	DrawID         int64     `json:"draw_id"`
	GifterMemberID int64     `json:"gifter_member_id"`
	GifteeMemberID int64     `json:"giftee_member_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// DrawStore is the subset of persistence the draw handlers need.
// internal/api never imports internal/assign (dependency-direction rule);
// internal/app's RunDraw implementation translates assign's sentinel
// errors into this package's own (ErrNoValidAssignment/ErrTooFewMembers)
// before returning.
type DrawStore interface {
	CreateDraw(ctx context.Context, drawerID int64, exchangeDate time.Time, budgetAmount int64) (Draw, error)
	GetDraw(ctx context.Context, drawerID, id int64) (Draw, error)
	ListDrawsByDrawer(ctx context.Context, drawerID int64) ([]Draw, error)
	ListAssignmentsByDraw(ctx context.Context, drawerID, drawID int64) ([]Assignment, error)
	UpdateDraw(ctx context.Context, drawerID, id int64, exchangeDate time.Time, budgetAmount int64) (Draw, error)
	RunDraw(ctx context.Context, drawerID, drawID int64) (Draw, []Assignment, error)
}

// drawCreateRequest is the request body for POST
// /drawers/{drawerID}/draws.
type drawCreateRequest struct {
	ExchangeDate time.Time `json:"exchange_date"`
	BudgetAmount int64     `json:"budget_amount"`
}

// drawUpdateRequest is the request body for PUT
// /drawers/{drawerID}/draws/{id} -- a full replace of exchange_date/
// budget_amount, matching drawerUpdateRequest's style, not a partial PATCH.
// Only valid while the draw is still 'draft'; DrawStore.UpdateDraw returns
// ErrDrawAlreadyRun otherwise.
type drawUpdateRequest struct {
	ExchangeDate time.Time `json:"exchange_date"`
	BudgetAmount int64     `json:"budget_amount"`
}

// runDrawResponse is the composite body POST .../run writes — a dedicated
// struct for this one endpoint's two-part payload, rather than widening
// Envelope (which stays Data any, unchanged).
type runDrawResponse struct {
	Draw        Draw         `json:"draw"`
	Assignments []Assignment `json:"assignments"`
}

// MountDraws adds the draw creation/listing endpoints, the assignments
// listing endpoint, and the run-draw endpoint to r, nested under
// /drawers/{drawerID}/draws.
func MountDraws(r chi.Router, draws DrawStore, adapter Adapter) {
	r.Route("/drawers/{drawerID}/draws", func(r chi.Router) {
		r.Post("/", adapter.Adapt(createDraw(draws)))
		r.Get("/", adapter.Adapt(listDraws(draws)))
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", adapter.Adapt(getDraw(draws)))
			r.Put("/", adapter.Adapt(updateDraw(draws)))
			r.Get("/assignments", adapter.Adapt(listAssignments(draws)))
			r.Post("/run", adapter.Adapt(runDraw(draws)))
		})
	})
}

func createDraw(draws DrawStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		drawerID, err := drawerIDParam(r)
		if err != nil {
			return err
		}

		var req drawCreateRequest
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			return fmt.Errorf("api: decode create draw request: %w", decodeErr)
		}

		draw, err := draws.CreateDraw(r.Context(), drawerID, req.ExchangeDate, req.BudgetAmount)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusCreated, draw)
	}
}

func listDraws(draws DrawStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		drawerID, err := drawerIDParam(r)
		if err != nil {
			return err
		}

		list, err := draws.ListDrawsByDrawer(r.Context(), drawerID)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, list)
	}
}

func getDraw(draws DrawStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		drawerID, err := drawerIDParam(r)
		if err != nil {
			return err
		}

		id, err := drawID(r)
		if err != nil {
			return err
		}

		draw, err := draws.GetDraw(r.Context(), drawerID, id)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, draw)
	}
}

func updateDraw(draws DrawStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		drawerID, err := drawerIDParam(r)
		if err != nil {
			return err
		}

		id, err := drawID(r)
		if err != nil {
			return err
		}

		var req drawUpdateRequest
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			return fmt.Errorf("api: decode update draw request: %w", decodeErr)
		}

		draw, err := draws.UpdateDraw(r.Context(), drawerID, id, req.ExchangeDate, req.BudgetAmount)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, draw)
	}
}

func listAssignments(draws DrawStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		drawerID, err := drawerIDParam(r)
		if err != nil {
			return err
		}

		id, err := drawID(r)
		if err != nil {
			return err
		}

		list, err := draws.ListAssignmentsByDraw(r.Context(), drawerID, id)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, list)
	}
}

func runDraw(draws DrawStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		drawerID, err := drawerIDParam(r)
		if err != nil {
			return err
		}

		id, err := drawID(r)
		if err != nil {
			return err
		}

		draw, assignments, err := draws.RunDraw(r.Context(), drawerID, id)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, runDrawResponse{Draw: draw, Assignments: assignments})
	}
}

func drawID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("api: parse draw id: %w", err)
	}

	return id, nil
}
