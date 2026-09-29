package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/trace"
)

const contentTypeJSON = "application/json"

// EnforceJSON rejects requests whose Content-Type isn't application/json
// when they carry a body, and requests whose Accept header explicitly
// excludes application/json.
func EnforceJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > 0 && !acceptsMediaType(r.Header.Get("Content-Type"), contentTypeJSON) {
			_ = WriteProblem(w, Problem{
				Status: http.StatusUnsupportedMediaType,
				Title:  "Unsupported Media Type",
				Detail: "Content-Type must be application/json",
			})

			return
		}

		if accept := r.Header.Get("Accept"); accept != "" && !acceptsMediaType(accept, contentTypeJSON) &&
			!acceptsMediaType(accept, "*/*") {
			_ = WriteProblem(w, Problem{
				Status: http.StatusNotAcceptable,
				Title:  "Not Acceptable",
				Detail: "Accept must include application/json",
			})

			return
		}

		next.ServeHTTP(w, r)
	})
}

// acceptsMediaType reports whether header (a Content-Type or Accept value,
// possibly with parameters and multiple comma-separated entries) matches
// mediaType.
func acceptsMediaType(header, mediaType string) bool {
	for entry := range strings.SplitSeq(header, ",") {
		parsed, _, err := mime.ParseMediaType(strings.TrimSpace(entry))
		if err == nil && parsed == mediaType {
			return true
		}
	}

	return false
}

// RequestLogger logs each request's method, path, status, and duration via
// logger, one line per request, tagged with chi's request ID and (when
// present) the otel trace ID. Responses carrying an application/problem+json
// body (any 4xx/5xx from EnforceJSON, the NotFound/MethodNotAllowed
// handlers, or Adapter) also get that Problem's title/detail logged, and the
// line is emitted at WARN (4xx) or ERROR (5xx) instead of INFO.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			var body bytes.Buffer
			ww.Tee(&body)

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

			level := problemAttrs(&attrs, ww.Status(), ww.Header().Get("Content-Type"), body.Bytes())

			logger.Log(r.Context(), level, "http request", attrs...)
		})
	}
}

// problemAttrs appends error_title/error_detail to attrs when status is an
// error carrying an application/problem+json body, and returns the log
// level the request should be logged at.
func problemAttrs(attrs *[]any, status int, contentType string, body []byte) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		if p, ok := decodeProblem(contentType, body); ok {
			*attrs = append(*attrs, "error_title", p.Title, "error_detail", p.Detail)
		}
		return slog.LevelError
	case status >= http.StatusBadRequest:
		if p, ok := decodeProblem(contentType, body); ok {
			*attrs = append(*attrs, "error_title", p.Title, "error_detail", p.Detail)
		}
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func decodeProblem(contentType string, body []byte) (Problem, bool) {
	if contentType != contentTypeProblemJSON {
		return Problem{}, false
	}

	var p Problem
	if err := json.Unmarshal(body, &p); err != nil {
		return Problem{}, false
	}

	return p, true
}

// SecurityHeaders sets response headers that harden the API against
// content-sniffing, clickjacking, and referrer leakage.
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
