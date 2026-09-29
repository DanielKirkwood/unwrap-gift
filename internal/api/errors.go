package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/trace"
)

// HandlerFunc is like [http.HandlerFunc] but returns an error instead of
// writing an error response directly. Adapter.Adapt turns one into a real
// [http.HandlerFunc].
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// ErrorMapping maps one sentinel error to the Problem it should produce.
// Adapter checks mappings in order, using [errors.Is], so earlier entries
// take precedence over later, less specific ones.
type ErrorMapping struct {
	Match   error
	Problem Problem
}

// ErrorsMap is an ordered list of ErrorMapping. An error that matches none
// of them produces a generic 500 Problem.
type ErrorsMap []ErrorMapping

// Adapter turns HandlerFunc values into [http.HandlerFunc], translating any
// returned error into a Problem response via ErrorsMap. It is constructed
// once (with a Logger and ErrorsMap) and injected into router construction,
// not a package-level singleton.
type Adapter struct {
	Logger    *slog.Logger
	ErrorsMap ErrorsMap
}

// Adapt wraps fn, writing fn's returned error (if any) as a Problem
// response instead of letting it propagate unhandled.
func (a Adapter) Adapt(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := fn(w, r)
		if err == nil {
			return
		}

		problem, ok := a.ErrorsMap.lookup(err)
		if !ok {
			a.logUnmapped(r.Context(), err)
			problem = Problem{Status: http.StatusInternalServerError, Title: "Internal Server Error"}
		}

		if writeErr := WriteProblem(w, problem); writeErr != nil {
			a.Logger.ErrorContext(r.Context(), "api: write problem response", "error", writeErr)
		}
	}
}

func (a Adapter) logUnmapped(ctx context.Context, err error) {
	attrs := []any{"error", err}

	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		attrs = append(attrs, "trace_id", span.TraceID().String())
	}

	a.Logger.ErrorContext(ctx, "api: unmapped handler error", attrs...)
}

// lookup returns the first mapping in m whose Match matches err via
// [errors.Is], and whether one was found.
func (m ErrorsMap) lookup(err error) (Problem, bool) {
	for _, mapping := range m {
		if errors.Is(err, mapping.Match) {
			return mapping.Problem, true
		}
	}

	return Problem{}, false
}
