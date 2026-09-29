package tasks

import (
	"context"
	"fmt"

	keto "github.com/ory/keto-client-go/v26"
)

//nolint:gochecknoinits // deliberate: self-registration pattern, see package doc.
func init() {
	Register(Task{
		Name:         "reseed-keto",
		RequiredArgs: []string{ArgIdentityID},
		Run: func(ctx context.Context, deps Deps, args map[string]string) error {
			if deps.Keto == nil {
				return errKetoNotConfigured
			}

			return SeedKetoAdmin(ctx, deps.Keto, args[ArgIdentityID])
		},
	})
}

// SeedKetoAdmin grants adminIdentityID the Role:admin members relation and
// makes Identities:admin's manage permission traverse that role, matching
// deploy/keto/identities.ts's OPL. It's the single implementation shared by
// cmd/db.go's `db seed` and this package's `reseed-keto` task, so the two
// entry points can't drift out of sync.
func SeedKetoAdmin(ctx context.Context, k RelationTupleWriter, adminIdentityID string) error {
	if err := k.CreateRelationTuple(ctx, "Role", "admin", "members", &adminIdentityID, nil); err != nil {
		return fmt.Errorf("tasks: seed keto admin role member: %w", err)
	}

	managerRole := &keto.SubjectSet{Namespace: "Role", Object: "admin", Relation: "members"}
	if err := k.CreateRelationTuple(ctx, "Identities", "admin", "managers", nil, managerRole); err != nil {
		return fmt.Errorf("tasks: seed keto identities manager: %w", err)
	}

	return nil
}
