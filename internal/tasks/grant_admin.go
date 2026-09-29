package tasks

import (
	"context"
	"errors"
	"fmt"
)

// errKetoNotConfigured is returned by every Keto-related task when
// deps.Keto is nil, i.e. the keto feature is disabled.
var errKetoNotConfigured = errors.New("tasks: keto feature is not enabled")

//nolint:gochecknoinits // deliberate: self-registration pattern, see package doc.
func init() {
	Register(Task{
		Name:         "grant-admin",
		RequiredArgs: []string{ArgIdentityID},
		Run:          grantAdmin,
	})
}

// grantAdmin grants args[ArgIdentityID] the Role:admin members relation,
// the same tuple cmd/db.go's `db seed` grants at setup time.
func grantAdmin(ctx context.Context, deps Deps, args map[string]string) error {
	if deps.Keto == nil {
		return errKetoNotConfigured
	}

	identityID := args[ArgIdentityID]

	if err := deps.Keto.CreateRelationTuple(ctx, "Role", "admin", "members", &identityID, nil); err != nil {
		return fmt.Errorf("tasks: grant admin: %w", err)
	}

	return nil
}
