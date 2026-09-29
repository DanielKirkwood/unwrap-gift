// Package api holds the HTTP layer: three independent chi routers (public,
// protected, hidden — see NewPublicRouter, NewProtectedRouter, and
// NewHiddenRouter), the shared middleware they're built from, and the
// example CRUD resources mounted on them (MountWidgets, MountIdentities).
// It deliberately never imports internal/clients or internal/db: instead it
// declares narrow interfaces (SessionValidator, PermissionChecker,
// WidgetStore, IdentityAdmin) that internal/app implements against the real
// clients and wires in via RouterDeps, per ARCHITECTURE.md's
// dependency-direction rule.
package api
