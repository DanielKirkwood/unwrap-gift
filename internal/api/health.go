package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// statusKey is the JSON key health responses report their state under.
const statusKey = "status"

// ReadyCheck reports whether the service is ready to serve traffic. A nil
// ReadyCheck (the Phase 3 default) means always ready; Phase 4 injects a
// real DB-ping implementation without any router code changing.
type ReadyCheck func(context.Context) error

// MountHealth adds /health/alive and /health/ready to r. /health/alive
// always reports 200. /health/ready calls ready (if non-nil) and reports
// 503 with a Problem body if it errors.
func MountHealth(r chi.Router, ready ReadyCheck) {
	r.Get("/health/alive", func(w http.ResponseWriter, _ *http.Request) {
		_ = WriteData(w, http.StatusOK, map[string]string{statusKey: "alive"})
	})

	r.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		if ready != nil {
			if err := ready(r.Context()); err != nil {
				_ = WriteProblem(w, Problem{
					Status: http.StatusServiceUnavailable,
					Title:  "Service Unavailable",
					Detail: err.Error(),
				})

				return
			}
		}

		_ = WriteData(w, http.StatusOK, map[string]string{statusKey: "ready"})
	})
}
