package tasks

import (
	"context"
	"fmt"
)

//nolint:gochecknoinits // deliberate: self-registration pattern, see package doc.
func init() {
	Register(Task{
		Name:         "revoke-admin",
		RequiredArgs: []string{ArgIdentityID},
		Run:          revokeAdmin,
	})
}

// revokeAdmin deletes the Role:admin members relation tuple grantAdmin
// creates, revoking args[ArgIdentityID]'s admin access.
func revokeAdmin(ctx context.Context, deps Deps, args map[string]string) error {
	if deps.Keto == nil {
		return errKetoNotConfigured
	}

	identityID := args[ArgIdentityID]

	if err := deps.Keto.DeleteRelationTuple(ctx, "Role", "admin", "members", &identityID, nil); err != nil {
		return fmt.Errorf("tasks: revoke admin: %w", err)
	}

	return nil
}
