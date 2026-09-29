package ketoclient_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	keto "github.com/ory/keto-client-go/v26"

	"github.com/DanielKirkwood/unwrap-gift/internal/clients/ketoclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

func TestNewDisabled(t *testing.T) {
	t.Parallel()

	client, err := ketoclient.New(config.KetoConfig{}, false)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if client != nil {
		t.Errorf("New() = %+v, want nil", client)
	}
}

func TestNewEnabled(t *testing.T) {
	t.Parallel()

	cfg := config.KetoConfig{ReadURL: "http://read.example", WriteURL: "http://write.example"}

	client, err := ketoclient.New(cfg, true)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if client == nil {
		t.Fatal("New() = nil, want non-nil")
	}
	if client.Read == nil {
		t.Error("Read = nil, want non-nil")
	}
	if client.Write == nil {
		t.Error("Write = nil, want non-nil")
	}
}

// fakeKeto serves the small slice of Keto's read/write HTTP surface
// ketoclient.Client's methods call, so its behavior can be exercised
// without a real Keto instance.
func fakeKeto(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /relation-tuples/check/openapi", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"allowed": r.URL.Query().Get("subject_id") == "allowed-subject"})
	})

	mux.HandleFunc("PUT /admin/relation-tuples", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"namespace":  "Role",
			"object":     "admin",
			"relation":   "members",
			"subject_id": "identity-1",
		})
	})

	mux.HandleFunc("DELETE /admin/relation-tuples", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func testClient(t *testing.T, server *httptest.Server) *ketoclient.Client {
	t.Helper()

	cfg := keto.NewConfiguration()
	cfg.Servers = keto.ServerConfigurations{{URL: server.URL}}
	cfg.HTTPClient = server.Client()
	apiClient := keto.NewAPIClient(cfg)

	return &ketoclient.Client{Read: apiClient, Write: apiClient}
}

func TestClient_CheckPermission(t *testing.T) {
	t.Parallel()

	client := testClient(t, fakeKeto(t))

	t.Run("allowed", func(t *testing.T) {
		t.Parallel()

		allowed, err := client.CheckPermission(t.Context(), "Identities", "admin", "manage", "allowed-subject")
		if err != nil {
			t.Fatalf("CheckPermission() error = %v, want nil", err)
		}
		if !allowed {
			t.Error("CheckPermission() = false, want true")
		}
	})

	t.Run("denied", func(t *testing.T) {
		t.Parallel()

		allowed, err := client.CheckPermission(t.Context(), "Identities", "admin", "manage", "other-subject")
		if err != nil {
			t.Fatalf("CheckPermission() error = %v, want nil", err)
		}
		if allowed {
			t.Error("CheckPermission() = true, want false")
		}
	})
}

func TestClient_CreateRelationTuple(t *testing.T) {
	t.Parallel()

	client := testClient(t, fakeKeto(t))

	subjectID := "identity-1"
	if err := client.CreateRelationTuple(t.Context(), "Role", "admin", "members", &subjectID, nil); err != nil {
		t.Fatalf("CreateRelationTuple() error = %v, want nil", err)
	}

	subjectSet := &keto.SubjectSet{Namespace: "Role", Object: "admin", Relation: "members"}
	if err := client.CreateRelationTuple(t.Context(), "Identities", "admin", "managers", nil, subjectSet); err != nil {
		t.Fatalf("CreateRelationTuple() with subject set error = %v, want nil", err)
	}
}

func TestClient_DeleteRelationTuple(t *testing.T) {
	t.Parallel()

	client := testClient(t, fakeKeto(t))

	subjectID := "identity-1"
	if err := client.DeleteRelationTuple(t.Context(), "Role", "admin", "members", &subjectID, nil); err != nil {
		t.Fatalf("DeleteRelationTuple() error = %v, want nil", err)
	}

	subjectSet := &keto.SubjectSet{Namespace: "Role", Object: "admin", Relation: "members"}
	if err := client.DeleteRelationTuple(t.Context(), "Identities", "admin", "managers", nil, subjectSet); err != nil {
		t.Fatalf("DeleteRelationTuple() with subject set error = %v, want nil", err)
	}
}
