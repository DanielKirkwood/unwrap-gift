// Package app is the composition root: it constructs dependencies (config,
// clients, DB store, routers) and wires them together explicitly. It is the
// only package that imports api/db/clients/tasks.
package app
