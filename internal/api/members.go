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

// ErrMemberNotFound is returned by MemberStore implementations when no
// member exists for the given id within the given drawer.
var ErrMemberNotFound = errors.New("api: member not found")

// ErrMemberPhoneAlreadyExists is returned by MemberStore.CreateMember when
// the drawer already has a member with that phone number.
var ErrMemberPhoneAlreadyExists = errors.New("api: member with this phone number already exists in this drawer")

// ErrMemberInvalidPhoneNumber is returned by MemberStore.CreateMember/
// UpdateMember when phoneNumber isn't a valid E.164 number (e.g.
// "+447700900000") -- catching a typo here, rather than letting it persist
// silently, is what keeps a Member's phone number matching the Kratos
// identity's "phone" trait it's joined to by string equality everywhere
// else in this codebase (wishlist lookups, SMS notification, login).
var ErrMemberInvalidPhoneNumber = errors.New("api: member phone number must be in E.164 format")

// Member is api's own representation of a member row, decoupled from
// internal/db/sqlc.Member.
type Member struct {
	ID          int64     `json:"id"`
	DrawerID    int64     `json:"drawer_id"`
	FullName    string    `json:"full_name"`
	PhoneNumber string    `json:"phone_number"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MemberStore is the subset of persistence the member handlers need. Every
// method that takes a drawerID returns ErrMemberNotFound if the member
// exists but belongs to a different drawer — not a generic 403/400 — so a
// caller can't distinguish "wrong drawer" from "doesn't exist" via
// error-shape. That check lives here, in the app-layer adapter, not in the
// handlers.
type MemberStore interface {
	CreateMember(ctx context.Context, drawerID int64, fullName, phoneNumber string) (Member, error)
	GetMember(ctx context.Context, drawerID, id int64) (Member, error)
	ListMembersByDrawer(ctx context.Context, drawerID int64) ([]Member, error)
	UpdateMember(ctx context.Context, drawerID, id int64, fullName, phoneNumber string) (Member, error)
	DeleteMember(ctx context.Context, drawerID, id int64) error
}

// memberRequest is the request body for both POST and PUT member
// endpoints.
type memberRequest struct {
	FullName    string `json:"full_name"`
	PhoneNumber string `json:"phone_number"`
}

// MountMembers adds the member CRUD endpoints to r, nested under
// /drawers/{drawerID}/members — the NESTED_RESOURCE_ROUTING pattern this
// plan introduces, chi's native URL-param nesting.
func MountMembers(r chi.Router, members MemberStore, adapter Adapter) {
	r.Route("/drawers/{drawerID}/members", func(r chi.Router) {
		r.Post("/", adapter.Adapt(createMember(members)))
		r.Get("/", adapter.Adapt(listMembers(members)))
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", adapter.Adapt(getMember(members)))
			r.Put("/", adapter.Adapt(updateMember(members)))
			r.Delete("/", adapter.Adapt(deleteMember(members)))
		})
	})
}

func createMember(members MemberStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		drawerID, err := drawerIDParam(r)
		if err != nil {
			return err
		}

		var req memberRequest
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			return fmt.Errorf("api: decode create member request: %w", decodeErr)
		}

		member, err := members.CreateMember(r.Context(), drawerID, req.FullName, req.PhoneNumber)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusCreated, member)
	}
}

func listMembers(members MemberStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		drawerID, err := drawerIDParam(r)
		if err != nil {
			return err
		}

		list, err := members.ListMembersByDrawer(r.Context(), drawerID)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, list)
	}
}

func getMember(members MemberStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		drawerID, err := drawerIDParam(r)
		if err != nil {
			return err
		}

		id, err := memberID(r)
		if err != nil {
			return err
		}

		member, err := members.GetMember(r.Context(), drawerID, id)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, member)
	}
}

func updateMember(members MemberStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		drawerID, err := drawerIDParam(r)
		if err != nil {
			return err
		}

		id, err := memberID(r)
		if err != nil {
			return err
		}

		var req memberRequest
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			return fmt.Errorf("api: decode update member request: %w", decodeErr)
		}

		member, err := members.UpdateMember(r.Context(), drawerID, id, req.FullName, req.PhoneNumber)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, member)
	}
}

func deleteMember(members MemberStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		drawerID, err := drawerIDParam(r)
		if err != nil {
			return err
		}

		id, err := memberID(r)
		if err != nil {
			return err
		}

		if deleteErr := members.DeleteMember(r.Context(), drawerID, id); deleteErr != nil {
			return deleteErr
		}

		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

func memberID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("api: parse member id: %w", err)
	}

	return id, nil
}

func drawerIDParam(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "drawerID"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("api: parse drawer id: %w", err)
	}

	return id, nil
}
