package api

import (
	"encoding/json"
	"net/http"
)

// Envelope wraps every successful response body in a uniform shape.
type Envelope struct {
	Data any `json:"data"`
}

// WriteData writes a successful response: it sets the JSON content type,
// writes the status header, and encodes data inside an Envelope so every
// success response shares the same {"data": ...} shape — the counterpart to
// WriteProblem's uniform error shape.
func WriteData(w http.ResponseWriter, status int, data any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	return json.NewEncoder(w).Encode(Envelope{Data: data})
}
