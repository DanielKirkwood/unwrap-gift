package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	kratos "github.com/ory/kratos-client-go/v26"
)

// IdentityAdmin is the subset of Kratos's admin Identity API the example
// CRUD endpoints in MountIdentities need. Implemented by
// internal/clients/kratosclient.Client; api depends on this interface
// instead of that concrete type so it never imports internal/clients, per
// ARCHITECTURE.md's dependency-direction rule. Errors it returns are mapped
// to Problems by whatever Adapter/ErrorsMap app builds for the router these
// handlers are mounted on (see kratosclient.ErrIdentityNotFound).
type IdentityAdmin interface {
	CreateIdentity(ctx context.Context, traits map[string]any, password string) (*kratos.Identity, error)
	ListIdentities(ctx context.Context) ([]kratos.Identity, error)
	GetIdentity(ctx context.Context, id string) (*kratos.Identity, error)
	UpdateIdentity(ctx context.Context, id, state string, traits map[string]any) (*kratos.Identity, error)
	DeleteIdentity(ctx context.Context, id string) error
}

// createIdentityRequest is the request body for POST /admin/identities.
// Password may be omitted to create an identity with no password
// credential.
type createIdentityRequest struct {
	Traits   map[string]any `json:"traits"`
	Password string         `json:"password"`
}

// updateIdentityRequest is the request body for PUT /admin/identities/{id}.
// State defaults to "active" when omitted.
type updateIdentityRequest struct {
	Traits map[string]any `json:"traits"`
	State  string         `json:"state"`
}

// MountIdentities adds the admin identity CRUD example endpoints to r,
// under /admin/identities, each routed through adapter so IdentityAdmin
// errors become Problem responses. It's the hidden-router,
// Kratos-admin-backed counterpart to MountWidgets' protected-router,
// sqlc-backed example — copy whichever one matches the new resource: an
// admin-only Kratos-fronted one, or an authenticated DB-backed one.
func MountIdentities(r chi.Router, identities IdentityAdmin, adapter Adapter) {
	r.Post("/admin/identities", adapter.Adapt(createIdentity(identities)))
	r.Get("/admin/identities", adapter.Adapt(listIdentities(identities)))
	r.Get("/admin/identities/{id}", adapter.Adapt(getIdentity(identities)))
	r.Put("/admin/identities/{id}", adapter.Adapt(updateIdentity(identities)))
	r.Delete("/admin/identities/{id}", adapter.Adapt(deleteIdentity(identities)))
}

func createIdentity(identities IdentityAdmin) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		var req createIdentityRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return fmt.Errorf("api: decode create identity request: %w", err)
		}

		identity, err := identities.CreateIdentity(r.Context(), req.Traits, req.Password)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusCreated, identity)
	}
}

func listIdentities(identities IdentityAdmin) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		list, err := identities.ListIdentities(r.Context())
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, list)
	}
}

func getIdentity(identities IdentityAdmin) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		identity, err := identities.GetIdentity(r.Context(), chi.URLParam(r, "id"))
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, identity)
	}
}

func updateIdentity(identities IdentityAdmin) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		id := chi.URLParam(r, "id")

		var req updateIdentityRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return fmt.Errorf("api: decode update identity request: %w", err)
		}
		if req.State == "" {
			req.State = "active"
		}

		identity, err := identities.UpdateIdentity(r.Context(), id, req.State, req.Traits)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, identity)
	}
}

func deleteIdentity(identities IdentityAdmin) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		if err := identities.DeleteIdentity(r.Context(), chi.URLParam(r, "id")); err != nil {
			return err
		}

		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}
