// Package web holds the fourth HTTP layer: the browser-facing router
// (NewWebRouter) that serves a server-rendered login page and the
// participant wishlist UI. It deliberately never imports internal/api,
// internal/clients, or internal/db: instead it declares its own narrow
// interfaces (SessionValidator, LoginFlowProvider, WishlistItemStore) that
// internal/app implements against the real clients and wires in via
// RouterDeps, per ARCHITECTURE.md's dependency-direction rule — the same
// reason internal/api never imports internal/clients.
//
// Login is not delegated to a separate UI service: it drives Kratos's
// public browser-flow API directly (GetLoginFlow, login.go), rendering
// whatever ui.nodes the flow currently has via a small, generic
// node-flattening helper rather than hardcoding one template per step.
package web
