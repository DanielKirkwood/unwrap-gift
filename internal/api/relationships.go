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

// ErrRelationshipNotFound is returned by RelationshipStore implementations
// when no relationship exists for the given id.
var ErrRelationshipNotFound = errors.New("api: relationship not found")

// ErrRelationshipSamePhoneNumber is returned by
// RelationshipStore.CreateRelationship when both phone numbers are the
// same.
var ErrRelationshipSamePhoneNumber = errors.New("api: relationship phone numbers must differ")

// ErrRelationshipAlreadyExists is returned by
// RelationshipStore.CreateRelationship when the (unordered) pair already
// has a relationship row.
var ErrRelationshipAlreadyExists = errors.New("api: relationship already exists for this pair")

// Relationship is api's own representation of a relationship row,
// decoupled from internal/db/sqlc.Relationship. Relationships are global —
// independent of any drawer or Member row — so a couple can be excluded
// before either person is a Member anywhere (see the Phase 5 plan's
// Decisions table).
type Relationship struct {
	ID           int64     `json:"id"`
	PhoneNumberA string    `json:"phone_number_a"`
	PhoneNumberB string    `json:"phone_number_b"`
	CreatedAt    time.Time `json:"created_at"`
}

// RelationshipStore is the subset of persistence the relationship handlers
// need. There is deliberately no GetRelationship (not in Phase 5's scope)
// and no UpdateRelationship (an exclusion pair is either present or not;
// "updating" one is delete-and-recreate via the two methods below).
type RelationshipStore interface {
	CreateRelationship(ctx context.Context, phoneNumberA, phoneNumberB string) (Relationship, error)
	ListRelationshipsForPhoneNumber(ctx context.Context, phoneNumber string) ([]Relationship, error)
	DeleteRelationship(ctx context.Context, id int64) error
}

// relationshipCreateRequest is the request body for POST /relationships.
type relationshipCreateRequest struct {
	PhoneNumberA string `json:"phone_number_a"`
	PhoneNumberB string `json:"phone_number_b"`
}

// missingPhoneNumberProblem is written directly (not via ErrorsMap) when
// GET /relationships is missing its required ?phone_number= query param —
// this is request-shape validation, not a store error.
func missingPhoneNumberProblem() Problem {
	return Problem{
		Status: http.StatusBadRequest,
		Title:  "Bad Request",
		Detail: "phone_number query parameter is required",
	}
}

// MountRelationships adds the global exclusion-pair endpoints to r, under
// /relationships — intentionally flat, not nested under /drawers/{id},
// matching the schema (no drawer_id column on relationships).
func MountRelationships(r chi.Router, relationships RelationshipStore, adapter Adapter) {
	r.Post("/relationships", adapter.Adapt(createRelationship(relationships)))
	r.Get("/relationships", adapter.Adapt(listRelationships(relationships)))
	r.Delete("/relationships/{id}", adapter.Adapt(deleteRelationship(relationships)))
}

func createRelationship(relationships RelationshipStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		var req relationshipCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return fmt.Errorf("api: decode create relationship request: %w", err)
		}

		relationship, err := relationships.CreateRelationship(r.Context(), req.PhoneNumberA, req.PhoneNumberB)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusCreated, relationship)
	}
}

func listRelationships(relationships RelationshipStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		phoneNumber := r.URL.Query().Get("phone_number")
		if phoneNumber == "" {
			return WriteProblem(w, missingPhoneNumberProblem())
		}

		list, err := relationships.ListRelationshipsForPhoneNumber(r.Context(), phoneNumber)
		if err != nil {
			return err
		}

		return WriteData(w, http.StatusOK, list)
	}
}

func deleteRelationship(relationships RelationshipStore) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := relationshipID(r)
		if err != nil {
			return err
		}

		if deleteErr := relationships.DeleteRelationship(r.Context(), id); deleteErr != nil {
			return deleteErr
		}

		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

func relationshipID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("api: parse relationship id: %w", err)
	}

	return id, nil
}
