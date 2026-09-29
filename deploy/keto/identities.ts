// OPL namespace config for the local dev Ory Keto stack. Started via
// `task keto:up` (see Taskfile.yml / docker-compose.keto.yml), referenced
// by keto.yml's `namespaces.location`.
//
// Models the one resource Phase 6 protects: the hidden router's admin
// identity CRUD endpoints. `db seed` grants a configured Kratos identity
// (KETO_SEED_ADMIN_IDENTITY_ID) the admin Role, which in turn is the sole
// manager of the Identities:admin object — see cmd/db.go's seedKeto.
//
// Phase 7's first real domain resource reuses this same Role namespace
// rather than duplicating role/permission plumbing per resource.
import { Namespace, Context } from "@ory/keto-namespace-types"

class Identity implements Namespace {}

class Role implements Namespace {
  related: {
    members: Identity[]
  }

  permits = {
    isMember: (ctx: Context): boolean => this.related.members.includes(ctx.subject),
  }
}

class Identities implements Namespace {
  related: {
    managers: Role[]
  }

  permits = {
    manage: (ctx: Context): boolean =>
      this.related.managers.traverse((role) => role.permits.isMember(ctx)),
  }
}
