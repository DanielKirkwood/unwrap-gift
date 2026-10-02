package web

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/trace"
)

const statusKey = "status"

// RequestLogger logs each request's method, path, status, and duration via
// logger, one line per request, tagged with chi's request ID and (when
// present) the otel trace ID — WARN on 4xx, ERROR on 5xx, INFO otherwise.
// Trimmed from internal/api's RequestLogger: web responses are HTML, not
// application/problem+json, so there's no Problem body to parse out.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				statusKey, ww.Status(),
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			}
			if span := trace.SpanContextFromContext(r.Context()); span.IsValid() {
				attrs = append(attrs, "trace_id", span.TraceID().String())
			}

			logger.Log(r.Context(), logLevel(ww.Status()), "web request", attrs...)
		})
	}
}

// logLevel maps an HTTP status to the slog level its request line should be
// logged at.
func logLevel(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

// SecurityHeaders sets response headers that harden the web app against
// content-sniffing, clickjacking, and referrer leakage. Verbatim copy of
// internal/api's SecurityHeaders.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-XSS-Protection", "0")

		next.ServeHTTP(w, r)
	})
}
