// Package tasks holds self-registering, ad-hoc CLI-runnable jobs. Each task
// lives in its own file and registers itself via init(), mirroring
// internal/config's feature-registry pattern — but for one-off operational
// actions (grant/revoke admin, reseed Keto) rather than always-on
// subsystems.
package tasks
