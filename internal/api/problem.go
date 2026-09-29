package api

import (
	"encoding/json"
	"net/http"
)

// contentTypeProblemJSON is the media type registered for RFC 9457 (which
// obsoletes RFC 7807) Problem Details responses.
const contentTypeProblemJSON = "application/problem+json"

// Problem is an RFC 9457 Problem Details object: the uniform shape for every
// error response this API returns.
type Problem struct {
	// Type is a URI reference identifying the problem type. "about:blank"
	// (the RFC's default) means the problem has no more specific semantics
	// than its HTTP status code.
	Type string `json:"type"`

	// Title is a short, human-readable summary of the problem type. It
	// should not change from occurrence to occurrence of the same Type.
	Title string `json:"title"`

	// Status is the HTTP status code for this occurrence of the problem.
	Status int `json:"status"`

	// Detail is a human-readable explanation specific to this occurrence.
	Detail string `json:"detail,omitempty"`

	// Instance is a URI reference identifying this specific occurrence.
	Instance string `json:"instance,omitempty"`
}

// WriteProblem writes p to w as application/problem+json, using p.Status as
// the HTTP status code. If p.Type is empty, it defaults to "about:blank" per
// RFC 9457.
func WriteProblem(w http.ResponseWriter, p Problem) error {
	if p.Type == "" {
		p.Type = "about:blank"
	}

	w.Header().Set("Content-Type", contentTypeProblemJSON)
	w.WriteHeader(p.Status)

	return json.NewEncoder(w).Encode(p)
}
