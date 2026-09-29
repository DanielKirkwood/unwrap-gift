package tasks_test

import (
	"context"
	"errors"
	"testing"

	keto "github.com/ory/keto-client-go/v26"

	"github.com/DanielKirkwood/unwrap-gift/internal/tasks"
)

// fakeKeto is a minimal tasks.RelationTupleWriter for testing the
// Keto-related tasks without a real Keto instance. It records the last
// call's arguments so tests can assert on what a task passed through.
type fakeKeto struct {
	err error

	lastOp         string
	lastNamespace  string
	lastObject     string
	lastRelation   string
	lastSubjectID  *string
	lastSubjectSet *keto.SubjectSet
}

func (f *fakeKeto) CreateRelationTuple(
	_ context.Context,
	namespace, object, relation string,
	subjectID *string,
	subjectSet *keto.SubjectSet,
) error {
	f.lastOp, f.lastNamespace, f.lastObject, f.lastRelation = "create", namespace, object, relation
	f.lastSubjectID, f.lastSubjectSet = subjectID, subjectSet

	return f.err
}

func (f *fakeKeto) DeleteRelationTuple(
	_ context.Context,
	namespace, object, relation string,
	subjectID *string,
	subjectSet *keto.SubjectSet,
) error {
	f.lastOp, f.lastNamespace, f.lastObject, f.lastRelation = "delete", namespace, object, relation
	f.lastSubjectID, f.lastSubjectSet = subjectID, subjectSet

	return f.err
}

func TestRegistry_AllSorted(t *testing.T) {
	t.Parallel()

	names := make(map[string]bool)
	for _, task := range tasks.All() {
		names[task.Name] = true
	}

	for _, want := range []string{"grant-admin", "revoke-admin", "reseed-keto"} {
		if !names[want] {
			t.Errorf("All() missing task %q", want)
		}
	}

	all := tasks.All()
	for i := 1; i < len(all); i++ {
		if all[i-1].Name > all[i].Name {
			t.Fatalf("All() not sorted: %q before %q", all[i-1].Name, all[i].Name)
		}
	}
}

func TestGrantAdmin(t *testing.T) {
	t.Parallel()

	task, ok := tasks.Get("grant-admin")
	if !ok {
		t.Fatal("Get(grant-admin) = false, want true")
	}
	if err := task.CheckRequiredArgs(map[string]string{}); err == nil {
		t.Error("CheckRequiredArgs({}) error = nil, want error for missing identity-id")
	}

	fake := &fakeKeto{}
	err := task.Run(t.Context(), tasks.Deps{Keto: fake}, map[string]string{"identity-id": "identity-1"})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if fake.lastOp != "create" ||
		fake.lastNamespace != "Role" || fake.lastObject != "admin" || fake.lastRelation != "members" {
		t.Errorf("Run() called %+v, want create Role:admin#members", fake)
	}
	if fake.lastSubjectID == nil || *fake.lastSubjectID != "identity-1" {
		t.Errorf("Run() subjectID = %v, want identity-1", fake.lastSubjectID)
	}
}

func TestGrantAdmin_KetoDisabled(t *testing.T) {
	t.Parallel()

	task, _ := tasks.Get("grant-admin")

	err := task.Run(t.Context(), tasks.Deps{}, map[string]string{"identity-id": "identity-1"})
	if err == nil {
		t.Fatal("Run() error = nil, want error when Keto is not configured")
	}
}

func TestRevokeAdmin(t *testing.T) {
	t.Parallel()

	task, ok := tasks.Get("revoke-admin")
	if !ok {
		t.Fatal("Get(revoke-admin) = false, want true")
	}

	fake := &fakeKeto{}
	err := task.Run(t.Context(), tasks.Deps{Keto: fake}, map[string]string{"identity-id": "identity-1"})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if fake.lastOp != "delete" || fake.lastNamespace != "Role" || fake.lastObject != "admin" ||
		fake.lastRelation != "members" {
		t.Errorf("Run() called %+v, want delete Role:admin#members", fake)
	}
}

func TestReseedKeto(t *testing.T) {
	t.Parallel()

	task, ok := tasks.Get("reseed-keto")
	if !ok {
		t.Fatal("Get(reseed-keto) = false, want true")
	}

	fake := &fakeKeto{}
	err := task.Run(t.Context(), tasks.Deps{Keto: fake}, map[string]string{"identity-id": "identity-1"})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	// SeedKetoAdmin makes two calls; fakeKeto only records the last one, so
	// just assert the final call is the Identities:admin managers grant.
	if fake.lastOp != "create" || fake.lastNamespace != "Identities" || fake.lastObject != "admin" ||
		fake.lastRelation != "managers" {
		t.Errorf("Run() last call = %+v, want create Identities:admin#managers", fake)
	}
}

func TestSeedKetoAdmin_PropagatesError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	fake := &fakeKeto{err: wantErr}

	err := tasks.SeedKetoAdmin(t.Context(), fake, "identity-1")
	if !errors.Is(err, wantErr) {
		t.Fatalf("SeedKetoAdmin() error = %v, want %v", err, wantErr)
	}
}
